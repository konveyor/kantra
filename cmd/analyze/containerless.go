package analyze

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/konveyor-ecosystem/kantra/cmd/internal/settings"
	kantraprovider "github.com/konveyor-ecosystem/kantra/pkg/provider"
	"github.com/konveyor-ecosystem/kantra/pkg/util"

	"github.com/bombsimon/logrusr/v3"
	"github.com/go-logr/logr"
	konveyorAnalyzer "github.com/konveyor/analyzer-lsp/core"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/tracing"
	"github.com/sirupsen/logrus"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

// engineStopper allows testing cleanup; engine.RuleEngine implements it.
type engineStopper interface {
	Stop()
}

// stopEngineAndProviders stops the rule engine and all providers.
// It is called from a defer in RunAnalysisContainerless so cleanup runs on every exit path.
func stopEngineAndProviders(eng engineStopper, providers map[string]provider.InternalProviderClient) {
	if eng != nil {
		eng.Stop()
	}
	for _, p := range providers {
		p.Stop()
	}
}

type ConsoleHook struct {
	Level logrus.Level
	Log   logr.Logger
}

func (hook *ConsoleHook) Fire(entry *logrus.Entry) error {
	_, err := entry.String()
	if err != nil {
		return nil // Ignore the error
	}

	if entry.Data["logger"] == "process-rule" {
		hook.Log.Info("processing rule", "ruleID", entry.Data["ruleID"])
	}
	return nil
}

func (hook *ConsoleHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// renderProgressBar renders a visual progress bar to stderr
func renderProgressBar(percent int, current, total int, message string) {
	const barWidth = 25
	filled := (percent * barWidth) / 100
	if filled > barWidth {
		filled = barWidth
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	// Truncate message if too long
	maxMessageLen := 40
	if len(message) > maxMessageLen {
		message = message[:maxMessageLen-3] + "..."
	}

	// Use \r to return to start of line and \033[K to clear to end of line
	fmt.Fprintf(os.Stderr, "\r\033[K  ✓ Processing rules %3d%% |%s| %d/%d  %s",
		percent, bar, current, total, message)
}

func (a *analyzeCommand) RunAnalysisContainerless(ctx context.Context) error {
	startTotal := time.Now()

	// Create progress mode to encapsulate progress reporting behavior
	progressMode := NewProgressMode(a.noProgress)

	// Create a conditional logger that only outputs in --no-progress mode
	// In progress mode, operational messages are suppressed to avoid interfering with the progress bar
	operationalLog := progressMode.OperationalLogger(a.log)

	operationalLog.Info("[TIMING] Containerless analysis starting")

	// Initialize Jaeger tracing if endpoint is provided
	if a.jaegerEndpoint != "" {
		operationalLog.Info("initializing Jaeger tracing", "endpoint", a.jaegerEndpoint)
		tracerOptions := tracing.Options{
			EnableJaeger:   true,
			JaegerEndpoint: a.jaegerEndpoint,
		}
		tp, err := tracing.InitTracerProvider(a.log, tracerOptions)
		if err != nil {
			a.log.Error(err, "failed to initialize tracing")
			return fmt.Errorf("failed to initialize tracing: %w", err)
		}
		defer tracing.Shutdown(ctx, a.log, tp)
		operationalLog.Info("Jaeger tracing initialized successfully")
	}

	// Hide cursor at the very start if progress is enabled
	progressMode.HideCursor()
	// Ensure cursor is shown at the end
	defer progressMode.ShowCursor()

	err := a.ValidateContainerless(ctx)
	if err != nil {
		a.log.Error(err, "failed to validate flags")
		return err
	}

	analysisLogFilePath := filepath.Join(a.output, "analysis.log")
	analysisLogFile, err := os.Create(analysisLogFilePath)
	if err != nil {
		return fmt.Errorf("failed creating provider log file at %s", analysisLogFilePath)
	}
	defer analysisLogFile.Close()

	// clean jdtls dirs after analysis
	defer func() {
		if err := a.cleanlsDirs(); err != nil {
			a.log.Error(err, "failed to clean language server directories")
		}
	}()

	// log output from analyzer to file
	logrusAnalyzerLog := logrus.New()
	logrusAnalyzerLog.SetOutput(analysisLogFile)
	logrusAnalyzerLog.SetFormatter(&logrus.TextFormatter{})
	if a.logLevel != nil {
		logrusAnalyzerLog.SetLevel(logrus.Level(*a.logLevel))
	}

	// add log hook, print the rule processing to the console
	// but only if progress is disabled (to avoid interfering with progress bar)
	if progressMode.ShouldAddConsoleHook() {
		consoleHook := &ConsoleHook{Level: logrus.InfoLevel, Log: a.log}
		logrusAnalyzerLog.AddHook(consoleHook)
	}

	analyzeLog := logrusr.New(logrusAnalyzerLog)

	// Detect if this is binary analysis based on file extension
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
	operationalLog.Info("running analysis")

	// --- Generate provider configs using shared defaults ---
	providerConfigs := kantraprovider.DefaultProviderConfig(kantraprovider.ModeLocal, kantraprovider.DefaultOptions{
		KantraDir:          a.kantraDir,
		Location:           a.input,
		AnalysisMode:       a.mode,
		DisableMavenSearch: a.disableMavenSearch,
		MavenSettingsFile:  a.mavenSettingsFile,
		JvmMaxMem:          settings.Settings.JvmMaxMem,
		ContextLines:       a.contextLines,
		HTTPProxy:          a.httpProxy,
		HTTPSProxy:         a.httpsProxy,
		NoProxy:            a.noProxy,
	})

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

	// Apply override provider settings if specified
	overrideConfigs, err := a.loadOverrideProviderSettings()
	if err != nil {
		return fmt.Errorf("failed to load override provider settings: %w", err)
	}
	if overrideConfigs != nil {
		operationalLog.Info("loaded override provider settings", "file", a.overrideProviderSettings, "providers", len(overrideConfigs))
		for i := range providerConfigs {
			providerConfigs[i] = applyProviderOverrides(providerConfigs[i], overrideConfigs)
		}
	}

	// Write provider settings to disk for konveyor.NewAnalyzer
	settingsPath := filepath.Join(a.output, "settings.json")
	jsonData, err := json.MarshalIndent(&providerConfigs, "", "\t")
	if err != nil {
		a.log.V(1).Error(err, "failed to marshal provider config")
		return err
	}
	err = os.WriteFile(settingsPath, jsonData, os.ModePerm)
	if err != nil {
		a.log.V(1).Error(err, "failed to write provider config", "dir", a.output, "file", "settings.json")
		return err
	}
	defer os.Remove(settingsPath)

	// --- Build rules list ---
	rules := make([]string, len(a.rules))
	copy(rules, a.rules)
	if a.enableDefaultRulesets {
		rules = append(rules, filepath.Join(a.kantraDir, settings.RulesetsLocation))
	}

	// --- Build label selectors ---
	labelSelector := a.getLabelSelector()
	depLabelSelector := ""
	if !a.analyzeKnownLibraries {
		depLabelSelector = fmt.Sprintf("!%v=open-source", provider.DepSourceLabel)
	}

	// --- Create progress reporter ---
	reporter, progressDone, progressCancel := setupProgressReporter(ctx, a.noProgress)
	if progressCancel != nil {
		defer progressCancel()
	}

	// Show decompiling message for binary analysis
	if isBinaryAnalysis {
		progressMode.Printf("  Decompiling binary...\n")
	}

	// --- Build analyzer options ---
	analyzerOpts := []konveyorAnalyzer.AnalyzerOption{
		konveyorAnalyzer.WithProviderConfigFilePath(settingsPath),
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

	// --- Create and run the analyzer ---
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

	sort.SliceStable(rulesets, func(i, j int) bool {
		return rulesets[i].Name < rulesets[j].Name
	})

	// Write results out to CLI
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

	// Ensure analysis log is closed before creating static-report (needed for bulk on Windows)
	analysisLogFile.Close()

	startStaticReport := time.Now()
	operationalLog.Info("[TIMING] Starting static report generation")
	err = a.GenerateStaticReport(ctx, operationalLog)
	if err != nil {
		a.log.Error(err, "failed to generate static report")
		return err
	}
	operationalLog.Info("[TIMING] Static report generation complete", "duration_ms", time.Since(startStaticReport).Milliseconds())

	// Print results summary (only in progress mode, not in --no-progress mode)
	progressMode.Println("\nResults:")
	reportPath := filepath.Join(a.output, "static-report", "index.html")
	progressMode.Printf("  Report: file://%s\n", reportPath)
	analysisLogPath := filepath.Join(a.output, "analysis.log")
	progressMode.Printf("  Analysis logs: %s\n", analysisLogPath)

	operationalLog.Info("[TIMING] Containerless analysis complete", "total_duration_ms", time.Since(startTotal).Milliseconds())
	return nil
}

func (a *analyzeCommand) buildStaticReportFile(ctx context.Context, staticReportPath string, depsErr bool) error {
	if a.skipStaticReport {
		return nil
	}
	// Prepare report args list with single input analysis
	applicationNames := []string{filepath.Base(a.input)}
	outputAnalyses := []string{filepath.Join(a.output, "output.yaml")}
	outputDeps := []string{filepath.Join(a.output, "dependencies.yaml")}
	outputJSPath := filepath.Join(staticReportPath, "output.js")

	if a.bulk {
		// Scan all available analysis output files to be reported
		applicationNames = nil
		outputAnalyses = nil
		outputDeps = nil
		outputFiles, err := filepath.Glob(filepath.Join(a.output, "output.yaml.*"))
		if err != nil {
			return err
		}
		for i := range outputFiles {
			outputName := filepath.Base(outputFiles[i])
			applicationName := strings.SplitN(outputName, "output.yaml.", 2)[1]
			applicationNames = append(applicationNames, applicationName)
			outputAnalyses = append(outputAnalyses, outputFiles[i])
			deps := fmt.Sprintf("%s.%s", filepath.Join(a.output, "dependencies.yaml"), applicationName)
			// If deps for given application are missing, empty the deps path allowing skip it in static-report
			if _, err := os.Stat(deps); errors.Is(err, os.ErrNotExist) {
				deps = ""
			}
			outputDeps = append(outputDeps, deps)
		}

	}

	if depsErr {
		outputDeps = []string{}
	}
	// create output.js file from analysis output.yaml
	apps, err := validateFlags(outputAnalyses, applicationNames, outputDeps, a.log)
	if err != nil {
		return fmt.Errorf("failed to validate flags: %w", err)
	}

	err = loadApplications(apps)
	if err != nil {
		return fmt.Errorf("failed to load report data from analysis output: %w", err)
	}

	err = generateJSBundle(apps, outputJSPath, a.log)
	if err != nil {
		return fmt.Errorf("failed to generate output.js file from template: %w", err)
	}

	return nil
}

func (a *analyzeCommand) buildStaticReportOutput(ctx context.Context, log *os.File) error {
	outputFolderSrcPath := filepath.Join(a.kantraDir, "static-report")
	outputFolderDestPath := filepath.Join(a.output, "static-report")

	//copy static report files to output folder
	err := util.CopyFolderContents(outputFolderSrcPath, outputFolderDestPath)
	if err != nil {
		return err
	}
	return nil
}

// GenerateStaticReport generates a static HTML report from analysis output.
// This function is used by both containerless and hybrid execution modes.
func (a *analyzeCommand) GenerateStaticReport(ctx context.Context, operationalLog logr.Logger) error {
	if a.skipStaticReport {
		return nil
	}
	operationalLog.Info("generating static report")
	staticReportLogFilePath := filepath.Join(a.output, "static-report.log")
	staticReportLog, err := os.Create(staticReportLogFilePath)
	if err != nil {
		return fmt.Errorf("failed creating provider log file at %s", staticReportLogFilePath)
	}
	defer staticReportLog.Close()

	// it's possible for dependency analysis to fail
	// in this case we still want to generate a static report for successful source analysis
	_, noDepFileErr := os.Stat(filepath.Join(a.output, "dependencies.yaml"))
	if errors.Is(noDepFileErr, os.ErrNotExist) {
		operationalLog.Info("unable to get dependency output in static report. generating static report from source analysis only")

		// some other err
	} else if noDepFileErr != nil && !errors.Is(noDepFileErr, os.ErrNotExist) {
		return noDepFileErr
	}

	if a.bulk {
		a.moveResults()
	}

	staticReportAnalyzePath := filepath.Join(a.kantraDir, "static-report")
	err = a.buildStaticReportOutput(ctx, staticReportLog)
	if err != nil {
		return err
	}
	err = a.buildStaticReportFile(ctx, staticReportAnalyzePath, errors.Is(noDepFileErr, os.ErrNotExist))
	if err != nil {
		return err
	}
	uri := uri.File(filepath.Join(a.output, "static-report", "index.html"))
	operationalLog.Info("Static report created. Access it at this URL:", "URL", string(uri))

	return nil
}
