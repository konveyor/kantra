package analyze

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/bombsimon/logrusr/v3"
	"github.com/konveyor-ecosystem/kantra/cmd/internal/settings"
	kantraprovider "github.com/konveyor-ecosystem/kantra/pkg/provider"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	konveyorAnalyzer "github.com/konveyor/analyzer-lsp/core"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/tracing"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

// runAnalysis is the unified analysis entry point. It replaces both
// RunAnalysisContainerless and RunAnalysisHybridInProcess by delegating
// mode-specific concerns to a provider.Environment.
//
// The caller determines the mode and which providers to use; this method
// handles the 15-step analysis pipeline that is identical across modes:
//
//  1. Progress/tracing/logging setup
//  2. Environment Start (mode-specific infrastructure)
//  3. Provider config generation + overrides
//  4. Rule collection
//  5. Label selector construction
//  6. Progress reporter setup
//  7. Analyzer option assembly
//  8. Analyzer creation
//  9. Rule parsing
//  10. Provider start
//  11. Rule execution
//  12. Dependency resolution
//  13. Post-analysis (e.g., provider log collection)
//  14. Output writing (YAML, JSON, static report)
//  15. Results summary
func (a *analyzeCommand) runAnalysis(ctx context.Context, mode kantraprovider.ExecutionMode, foundProviders []string) error {
	startTotal := time.Now()

	// --- Progress, tracing, logging setup ---
	progressMode := NewProgressMode(a.noProgress)
	operationalLog := progressMode.OperationalLogger(a.log)

	operationalLog.Info("[TIMING] Analysis starting", "mode", mode)

	if a.jaegerEndpoint != "" {
		operationalLog.Info("initializing Jaeger tracing", "endpoint", a.jaegerEndpoint)
		tp, err := tracing.InitTracerProvider(a.log, tracing.Options{
			EnableJaeger:   true,
			JaegerEndpoint: a.jaegerEndpoint,
		})
		if err != nil {
			a.log.Error(err, "failed to initialize tracing")
			return fmt.Errorf("failed to initialize tracing: %w", err)
		}
		defer tracing.Shutdown(ctx, a.log, tp)
		operationalLog.Info("Jaeger tracing initialized successfully")
	}

	progressMode.HideCursor()
	defer progressMode.ShowCursor()

	// Detect binary analysis from file extension
	isBinaryAnalysis := false
	if a.isFileInput {
		ext := filepath.Ext(a.input)
		isBinaryAnalysis = (ext == util.JavaArchive || ext == util.WebArchive ||
			ext == util.EnterpriseArchive || ext == util.ClassFile)
	}

	if isBinaryAnalysis {
		progressMode.Printf("Running binary analysis...\n")
	} else {
		progressMode.Printf("Running source analysis...\n")
	}
	operationalLog.Info("running analysis", "mode", mode)

	// Create analysis log file
	analysisLogFilePath := filepath.Join(a.output, "analysis.log")
	analysisLog, err := os.Create(analysisLogFilePath)
	if err != nil {
		return fmt.Errorf("failed creating analysis log file at %s", analysisLogFilePath)
	}
	defer func() {
		if analysisLog != nil {
			_ = analysisLog.Close()
		}
	}()

	// Setup logrus for analyzer (writes to analysis.log)
	logrusAnalyzerLog := logrus.New()
	logrusAnalyzerLog.SetOutput(analysisLog)
	logrusAnalyzerLog.SetFormatter(&logrus.TextFormatter{})
	if a.logLevel != nil {
		logrusAnalyzerLog.SetLevel(logrus.Level(*a.logLevel))
	}
	if progressMode.ShouldAddConsoleHook() {
		consoleHook := &ConsoleHook{Level: logrus.InfoLevel, Log: a.log}
		logrusAnalyzerLog.AddHook(consoleHook)
	}
	analyzeLog := logrusr.New(logrusAnalyzerLog)

	// --- Create environment ---
	env := kantraprovider.NewEnvironment(kantraprovider.EnvironmentConfig{
		Mode:                  mode,
		Input:                 a.input,
		IsFileInput:           a.isFileInput,
		AnalysisMode:          a.mode,
		ContextLines:          a.contextLines,
		MavenSettingsFile:     a.mavenSettingsFile,
		JvmMaxMem:             settings.Settings.JvmMaxMem,
		HTTPProxy:             a.httpProxy,
		HTTPSProxy:            a.httpsProxy,
		NoProxy:               a.noProxy,
		Log:                   a.log,
		KantraDir:             a.kantraDir,
		DisableMavenSearch:    a.disableMavenSearch,
		Providers:             buildProviderInfos(foundProviders),
		ContainerBinary:       settings.Settings.ContainerBinary,
		RunnerImage:           settings.Settings.RunnerImage,
		OutputDir:             a.output,
		EnableDefaultRulesets: a.enableDefaultRulesets,
		LogLevel:              a.logLevel,
		Cleanup:               a.cleanup,
		DepFolders:            a.depFolders,
		Version:               settings.Version,
	})

	if err := env.Start(ctx); err != nil {
		return err
	}
	defer env.Stop(ctx)
	progressMode.Printf("  ✓ Started providers\n")

	// --- Provider configs + overrides ---
	providerConfigs := env.ProviderConfigs()

	// Apply excludedDirs from profile settings
	if excludedDir := util.GetProfilesExcludedDir(a.input, "", false); excludedDir != "" {
		for i := range providerConfigs {
			if len(providerConfigs[i].InitConfig) > 0 {
				if providerConfigs[i].InitConfig[0].ProviderSpecificConfig == nil {
					providerConfigs[i].InitConfig[0].ProviderSpecificConfig = map[string]interface{}{}
				}
				providerConfigs[i].InitConfig[0].ProviderSpecificConfig["excludedDirs"] = []interface{}{excludedDir}
			}
		}
	}

	overrideConfigs, err := a.loadOverrideProviderSettings()
	if err != nil {
		return fmt.Errorf("failed to load override provider settings: %w", err)
	}
	if overrideConfigs != nil {
		operationalLog.Info("loaded override provider settings", "file", a.overrideProviderSettings, "providers", len(overrideConfigs))
		providerConfigs = applyAllProviderOverrides(providerConfigs, overrideConfigs)
	}

	// --- Rules ---
	rules, err := env.Rules(a.rules, a.enableDefaultRulesets)
	if err != nil {
		return fmt.Errorf("failed to get rules: %w", err)
	}

	// --- Label selectors ---
	labelSelector := a.getLabelSelector()
	depLabelSelector := ""
	if !a.analyzeKnownLibraries {
		depLabelSelector = fmt.Sprintf("!%v=open-source", provider.DepSourceLabel)
	}

	// --- Progress reporter ---
	reporter, progressDone, progressCancel := setupProgressReporter(ctx, a.noProgress)
	if progressCancel != nil {
		defer progressCancel()
	}

	if isBinaryAnalysis {
		progressMode.Printf("  Decompiling binary...\n")
	}

	// --- Build analyzer options ---
	analyzerOpts := []konveyorAnalyzer.AnalyzerOption{
		konveyorAnalyzer.WithProviderConfigs(providerConfigs),
		konveyorAnalyzer.WithRuleFilepaths(rules),
		konveyorAnalyzer.WithLabelSelector(labelSelector),
		konveyorAnalyzer.WithContextLinesLimit(a.contextLines),
		konveyorAnalyzer.WithLogger(analyzeLog),
		konveyorAnalyzer.WithContext(ctx),
		konveyorAnalyzer.WithReporters(reporter),
	}
	if a.incidentSelector != "" {
		analyzerOpts = append(analyzerOpts, konveyorAnalyzer.WithIncidentSelector(a.incidentSelector))
	}
	if a.noDepRules {
		analyzerOpts = append(analyzerOpts, konveyorAnalyzer.WithDependencyRulesDisabled())
	}
	if depLabelSelector != "" {
		analyzerOpts = append(analyzerOpts, konveyorAnalyzer.WithDepLabelSelector(depLabelSelector))
	}

	// Apply mode-specific extra options (path mappings for binary, ignore builtin configs for source)
	extra := env.ExtraOptions(ctx, isBinaryAnalysis)
	if len(extra.PathMappings) > 0 {
		analyzerOpts = append(analyzerOpts, konveyorAnalyzer.WithPathMappings(extra.PathMappings))
	}
	if extra.IgnoreAdditionalBuiltinConfigs {
		analyzerOpts = append(analyzerOpts, konveyorAnalyzer.WithIgnoreAdditionalBuiltinConfigs(true))
	}

	// --- Analyzer lifecycle ---
	startAnalyzer := time.Now()
	operationalLog.Info("[TIMING] Creating analyzer")
	anlzr, err := konveyorAnalyzer.NewAnalyzer(analyzerOpts...)
	if err != nil {
		a.log.Error(err, "failed to create analyzer")
		return fmt.Errorf("failed to create analyzer: %w", err)
	}
	defer anlzr.Stop()
	operationalLog.Info("[TIMING] Analyzer created", "duration_ms", time.Since(startAnalyzer).Milliseconds())

	if isBinaryAnalysis {
		progressMode.Printf("  ✓ Decompiling complete\n")
	}
	progressMode.Printf("  ✓ Initialized providers\n")

	// Parse rules
	startRuleLoading := time.Now()
	operationalLog.Info("[TIMING] Starting rule loading")
	_, err = anlzr.ParseRules()
	if err != nil {
		a.log.Error(err, "failed to parse rules")
		return fmt.Errorf("failed to parse rules: %w", err)
	}
	operationalLog.Info("[TIMING] Rule loading complete", "duration_ms", time.Since(startRuleLoading).Milliseconds())

	// Start providers (ProviderInit + Prepare)
	startProviders := time.Now()
	operationalLog.Info("[TIMING] Starting provider init")
	err = anlzr.ProviderStart()
	if err != nil {
		a.log.Error(err, "failed to start providers")
		return fmt.Errorf("failed to start providers: %w", err)
	}
	operationalLog.Info("[TIMING] Provider init complete", "duration_ms", time.Since(startProviders).Milliseconds())

	progressMode.Printf("  ✓ Started rules engine\n")

	// Run analysis
	startRuleExecution := time.Now()
	operationalLog.Info("[TIMING] Starting rule execution")
	operationalLog.Info("evaluating rules for violations. see analysis.log for more info")
	rulesets := anlzr.Run()
	operationalLog.Info("[TIMING] Rule execution complete", "duration_ms", time.Since(startRuleExecution).Milliseconds())

	// Get dependencies
	operationalLog.Info("resolving dependencies")
	depFile := filepath.Join(a.output, "dependencies.yaml")
	if err := anlzr.GetDependencies(depFile, false); err != nil {
		a.log.Error(err, "failed to get dependencies")
		// Don't return -- dependency analysis failure shouldn't block output
	}

	// Cancel progress context and wait for goroutine to finish
	if progressMode.IsEnabled() && progressCancel != nil {
		progressCancel()
		<-progressDone
	}

	// Post-analysis (e.g., collect provider container logs)
	if err := env.PostAnalysis(ctx); err != nil {
		a.log.Error(err, "failed post-analysis tasks")
	}

	// --- Write output ---
	sort.SliceStable(rulesets, func(i, j int) bool {
		return rulesets[i].Name < rulesets[j].Name
	})

	startWriting := time.Now()
	operationalLog.Info("[TIMING] Starting output writing")
	operationalLog.Info("writing analysis results to output", "output", a.output)
	b, err := yaml.Marshal(rulesets)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(a.output, "output.yaml"), b, 0644)
	if err != nil {
		return fmt.Errorf("failed to write output.yaml: %w", err)
	}

	err = a.CreateJSONOutput()
	if err != nil {
		a.log.Error(err, "failed to create json output file")
		return err
	}
	operationalLog.Info("[TIMING] Output writing complete", "duration_ms", time.Since(startWriting).Milliseconds())

	err = analysisLog.Sync()
	if err != nil {
		a.log.Error(err, "failed to sync analysis log")
	}
	if a.bulk {
		// moveResults deletes analysis.log; Windows cannot remove an open file.
		if cerr := analysisLog.Close(); cerr != nil {
			a.log.Error(cerr, "failed to close analysis log before static report")
		}
		logrusAnalyzerLog.SetOutput(io.Discard)
		analysisLog = nil
	}

	startStaticReport := time.Now()
	operationalLog.Info("[TIMING] Starting static report generation")
	err = a.GenerateStaticReport(ctx, operationalLog)
	if err != nil {
		a.log.Error(err, "failed to generate static report")
		return err
	}
	operationalLog.Info("[TIMING] Static report generation complete", "duration_ms", time.Since(startStaticReport).Milliseconds())

	// Print results summary
	progressMode.Println("\nResults:")
	reportPath := filepath.Join(a.output, "static-report", "index.html")
	progressMode.Printf("  Report: file://%s\n", reportPath)
	analysisLogPath := filepath.Join(a.output, "analysis.log")
	progressMode.Printf("  Analysis logs: %s\n", analysisLogPath)

	operationalLog.Info("[TIMING] Analysis complete", "mode", mode, "total_duration_ms", time.Since(startTotal).Milliseconds())
	return nil
}

// buildProviderInfos converts a list of provider names to ProviderInfo structs
// using the provider images from settings.
func buildProviderInfos(foundProviders []string) []kantraprovider.ProviderInfo {
	infos := make([]kantraprovider.ProviderInfo, 0, len(foundProviders))
	for _, name := range foundProviders {
		image := providerImage(name)
		if image == "" {
			continue
		}
		infos = append(infos, kantraprovider.ProviderInfo{
			Name:  name,
			Image: image,
		})
	}
	return infos
}

// providerImage returns the container image for a provider name.
func providerImage(name string) string {
	switch name {
	case util.JavaProvider:
		return settings.Settings.JavaProviderImage
	case util.GoProvider, util.PythonProvider, util.NodeJSProvider:
		return settings.Settings.GenericProviderImage
	case util.CsharpProvider:
		return settings.Settings.CsharpProviderImage
	default:
		return ""
	}
}
