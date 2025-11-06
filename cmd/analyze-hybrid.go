package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/bombsimon/logrusr/v3"
	"github.com/go-logr/logr"
	kantraProvider "github.com/konveyor-ecosystem/kantra/pkg/provider"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	java "github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/parser"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"github.com/konveyor/analyzer-lsp/tracing"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/yaml.v2"
)

// setupJavaProviderHybrid creates a network-based Java provider client for hybrid mode.
// The provider runs in a container and this client connects via network (localhost:PORT).
func (a *analyzeCommand) setupJavaProviderHybrid(ctx context.Context, analysisLog logr.Logger) (provider.InternalProviderClient, []string, []provider.InitConfig, error) {
	provInit, ok := a.providersMap[util.JavaProvider]
	if !ok {
		return nil, nil, nil, fmt.Errorf("java provider not initialized in providersMap")
	}

	// Create network-based provider config
	// Key difference from containerless: Address is set, BinaryPath is empty
	javaConfig := provider.Config{
		Name:       util.JavaProvider,
		Address:    fmt.Sprintf("localhost:%d", provInit.port), // Connect to containerized provider
		BinaryPath: "",                                          // Empty = network mode
		InitConfig: []provider.InitConfig{
			{
				Location:     a.input,
				AnalysisMode: provider.AnalysisMode(a.mode),
				ProviderSpecificConfig: map[string]interface{}{
					"lspServerName": util.JavaProvider,
				},
			},
		},
	}

	// Set proxy if configured
	if a.httpProxy != "" || a.httpsProxy != "" {
		proxy := provider.Proxy{
			HTTPProxy:  a.httpProxy,
			HTTPSProxy: a.httpsProxy,
			NoProxy:    a.noProxy,
		}
		javaConfig.Proxy = &proxy
	}
	javaConfig.ContextLines = a.contextLines

	providerLocations := []string{}
	for _, ind := range javaConfig.InitConfig {
		providerLocations = append(providerLocations, ind.Location)
	}

	// Create network-based Java provider (connects to localhost:PORT)
	javaProvider := java.NewJavaProvider(analysisLog, "java", a.contextLines, javaConfig)

	a.log.V(1).Info("starting network-based provider", "provider", util.JavaProvider, "address", javaConfig.Address)
	initCtx, _ := tracing.StartNewSpan(ctx, "init")
	additionalBuiltinConfs, err := javaProvider.ProviderInit(initCtx, nil)
	if err != nil {
		a.log.Error(err, "unable to init the providers", "provider", util.JavaProvider)
		return nil, nil, nil, err
	}

	return javaProvider, providerLocations, additionalBuiltinConfs, nil
}

// setupBuiltinProviderHybrid creates a builtin provider for hybrid mode.
// This is the same as containerless mode since builtin always runs in-process.
func (a *analyzeCommand) setupBuiltinProviderHybrid(ctx context.Context, excludedTargetPaths []interface{}, additionalConfigs []provider.InitConfig, analysisLog logr.Logger) (provider.InternalProviderClient, []string, error) {
	a.log.V(1).Info("setting up builtin provider for hybrid mode")

	// Get Java target paths to exclude from builtin
	javaTargetPaths, err := kantraProvider.WalkJavaPathForTarget(a.log, a.isFileInput, a.input)
	if err != nil {
		a.log.Error(err, "error getting target subdir in Java project - some duplicate incidents may occur")
	}

	builtinConfig := provider.Config{
		Name: "builtin",
		InitConfig: []provider.InitConfig{
			{
				Location:     a.input,
				AnalysisMode: provider.AnalysisMode(a.mode),
				ProviderSpecificConfig: map[string]interface{}{
					"excludedDirs": javaTargetPaths,
				},
			},
		},
	}

	// Set proxy if configured
	if a.httpProxy != "" || a.httpsProxy != "" {
		proxy := provider.Proxy{
			HTTPProxy:  a.httpProxy,
			HTTPSProxy: a.httpsProxy,
			NoProxy:    a.noProxy,
		}
		builtinConfig.Proxy = &proxy
	}
	builtinConfig.ContextLines = a.contextLines

	providerLocations := []string{}
	for _, ind := range builtinConfig.InitConfig {
		providerLocations = append(providerLocations, ind.Location)
	}

	// Use lib.GetProviderClient to create builtin provider (public API)
	builtinProvider, err := lib.GetProviderClient(builtinConfig, analysisLog)
	if err != nil {
		return nil, nil, err
	}

	a.log.V(1).Info("starting provider", "provider", "builtin")
	if _, err := builtinProvider.ProviderInit(ctx, additionalConfigs); err != nil {
		a.log.Error(err, "unable to init the builtin provider")
		return nil, nil, err
	}

	return builtinProvider, providerLocations, nil
}

// RunAnalysisHybridInProcess runs analysis in hybrid mode with the analyzer running in-process
// and providers running in containers. This provides clean output like containerless mode while
// maintaining the isolation benefits of containerized providers.
//
// Architecture:
//   - Providers: Run in containers with port publishing (localhost:PORT)
//   - Analyzer: Runs as in-process Go library with direct logging control
//   - Communication: Network-based provider clients connect to localhost:PORT
//
// This approach combines the best of both worlds:
//   - Clean output and direct control from in-process execution
//   - Provider isolation and consistency from containers
func (a *analyzeCommand) RunAnalysisHybridInProcess(ctx context.Context) error {
	a.log.Info("running analysis in hybrid mode (analyzer in-process, providers in containers)")

	// Create analysis log file
	analysisLogFilePath := filepath.Join(a.output, "analysis.log")
	analysisLog, err := os.Create(analysisLogFilePath)
	if err != nil {
		return fmt.Errorf("failed creating analysis log file at %s", analysisLogFilePath)
	}
	defer analysisLog.Close()

	// Setup logging - analyzer logs to file, clean output to console
	logrusAnalyzerLog := logrus.New()
	logrusAnalyzerLog.SetOutput(analysisLog)
	logrusAnalyzerLog.SetFormatter(&logrus.TextFormatter{})
	logrusAnalyzerLog.SetLevel(logrus.Level(logLevel))

	// Add console hook for rule processing messages
	consoleHook := &ConsoleHook{Level: logrus.InfoLevel, Log: a.log}
	logrusAnalyzerLog.AddHook(consoleHook)

	analyzeLog := logrusr.New(logrusAnalyzerLog)

	// Error logging to stderr
	logrusErrLog := logrus.New()
	logrusErrLog.SetOutput(os.Stderr)
	errLog := logrusr.New(logrusErrLog)

	// Setup label selectors
	a.log.Info("running source analysis")
	labelSelectors := a.getLabelSelector()

	selectors := []engine.RuleSelector{}
	if labelSelectors != "" {
		selector, err := labels.NewLabelSelector[*engine.RuleMeta](labelSelectors, nil)
		if err != nil {
			errLog.Error(err, "failed to create label selector from expression", "selector", labelSelectors)
			os.Exit(1)
		}
		selectors = append(selectors, selector)
	}

	var dependencyLabelSelector *labels.LabelSelector[*konveyor.Dep]
	depLabel := fmt.Sprintf("!%v=open-source", provider.DepSourceLabel)
	if !a.analyzeKnownLibraries {
		dependencyLabelSelector, err = labels.NewLabelSelector[*konveyor.Dep](depLabel, nil)
		if err != nil {
			errLog.Error(err, "failed to create label selector from expression", "selector", depLabel)
			os.Exit(1)
		}
	}

	// Start containerized providers if any
	if len(a.providersMap) > 0 {
		// Create volume for provider containers
		volName, err := a.createContainerVolume(a.input)
		if err != nil {
			return fmt.Errorf("failed to create container volume: %w", err)
		}

		// Start providers with port publishing
		err = a.RunProvidersHostNetwork(ctx, volName, 5)
		if err != nil {
			return fmt.Errorf("failed to start providers: %w", err)
		}

		// Wait for providers to initialize
		// TODO: Replace with proper health checks
		a.log.Info("waiting for providers to initialize...")
		time.Sleep(4 * time.Second)
	}

	// Setup provider clients
	providers := map[string]provider.InternalProviderClient{}
	providerLocations := []string{}

	// Get Java target paths to exclude from builtin
	javaTargetPaths, err := kantraProvider.WalkJavaPathForTarget(a.log, a.isFileInput, a.input)
	if err != nil {
		a.log.Error(err, "error getting target subdir in Java project - some duplicate incidents may occur")
	}

	var additionalBuiltinConfigs []provider.InitConfig

	// Setup Java provider (network-based) if configured
	if _, hasJava := a.providersMap[util.JavaProvider]; hasJava {
		javaProvider, javaLocations, javaBuiltinConfigs, err := a.setupJavaProviderHybrid(ctx, analyzeLog)
		if err != nil {
			errLog.Error(err, "unable to start Java provider")
			os.Exit(1)
		}
		providers[util.JavaProvider] = javaProvider
		providerLocations = append(providerLocations, javaLocations...)
		additionalBuiltinConfigs = append(additionalBuiltinConfigs, javaBuiltinConfigs...)
	}

	// Setup builtin provider (always in-process)
	builtinProvider, builtinLocations, err := a.setupBuiltinProviderHybrid(ctx, javaTargetPaths, additionalBuiltinConfigs, analyzeLog)
	if err != nil {
		errLog.Error(err, "unable to start builtin provider")
		os.Exit(1)
	}
	providers["builtin"] = builtinProvider
	providerLocations = append(providerLocations, builtinLocations...)

	// Create rule engine
	engineCtx, engineSpan := tracing.StartNewSpan(ctx, "rule-engine")
	eng := engine.CreateRuleEngine(engineCtx,
		10,
		analyzeLog,
		engine.WithContextLines(a.contextLines),
		engine.WithIncidentSelector(a.incidentSelector),
		engine.WithLocationPrefixes(providerLocations),
	)

	// Setup rule parser
	ruleParser := parser.RuleParser{
		ProviderNameToClient: providers,
		Log:                  analyzeLog.WithName("parser"),
		NoDependencyRules:    a.noDepRules,
		DepLabelSelector:     dependencyLabelSelector,
	}

	// Load rules
	ruleSets := []engine.RuleSet{}
	needProviders := map[string]provider.InternalProviderClient{}

	// Extract default rulesets from container if enabled
	if a.enableDefaultRulesets {
		rulesetsDir, err := a.extractDefaultRulesets(ctx)
		if err != nil {
			return fmt.Errorf("failed to extract default rulesets: %w", err)
		}
		if rulesetsDir != "" {
			a.rules = append(a.rules, rulesetsDir)
		}
	}

	for _, f := range a.rules {
		a.log.Info("parsing rules for analysis", "rules", f)

		internRuleSet, internNeedProviders, err := ruleParser.LoadRules(f)
		if err != nil {
			a.log.Error(err, "unable to parse all the rules for ruleset", "file", f)
		}
		ruleSets = append(ruleSets, internRuleSet...)
		for k, v := range internNeedProviders {
			needProviders[k] = v
		}
	}

	// Start dependency analysis for full analysis mode
	wg := &sync.WaitGroup{}
	var depSpan trace.Span
	if a.mode == string(provider.FullAnalysisMode) {
		_, hasJava := a.providersMap[util.JavaProvider]
		if hasJava {
			var depCtx context.Context
			depCtx, depSpan = tracing.StartNewSpan(ctx, "dep")
			wg.Add(1)

			a.log.Info("running dependency analysis")
			go a.DependencyOutputContainerless(depCtx, providers, "dependencies.yaml", wg)
		}
	}

	// Run rules
	a.log.Info("evaluating rules for violations. see analysis.log for more info")
	rulesets := eng.RunRules(ctx, ruleSets, selectors...)
	engineSpan.End()
	wg.Wait()
	if depSpan != nil {
		depSpan.End()
	}
	eng.Stop()

	// Stop providers
	for _, provider := range needProviders {
		provider.Stop()
	}

	// Sort rulesets
	sort.SliceStable(rulesets, func(i, j int) bool {
		return rulesets[i].Name < rulesets[j].Name
	})

	// Write results
	a.log.Info("writing analysis results to output", "output", a.output)
	b, err := yaml.Marshal(rulesets)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(a.output, "output.yaml"), b, 0644)
	if err != nil {
		return fmt.Errorf("failed to write output.yaml: %w", err)
	}

	// Create JSON output if requested
	err = a.CreateJSONOutput()
	if err != nil {
		a.log.Error(err, "failed to create json output file")
		return err
	}

	// Close analysis log before generating static report
	analysisLog.Close()

	// Generate static report
	err = a.GenerateStaticReport(ctx)
	if err != nil {
		a.log.Error(err, "failed to generate static report")
		return err
	}

	a.log.Info("hybrid analysis completed successfully")
	return nil
}
