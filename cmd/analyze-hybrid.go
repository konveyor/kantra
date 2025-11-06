package cmd

import (
	"context"
	"fmt"
	"net"
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
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/parser"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"github.com/konveyor/analyzer-lsp/tracing"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/yaml.v2"
)

// validateProviderConfig validates configuration before starting provider containers.
// This catches configuration errors early and provides helpful error messages.
func (a *analyzeCommand) validateProviderConfig() error {
	// Validate Maven settings file if specified
	if a.mavenSettingsFile != "" {
		if _, err := os.Stat(a.mavenSettingsFile); err != nil {
			return fmt.Errorf(
				"Maven settings file not found: %s\n"+
					"Specified with --maven-settings flag but file does not exist",
				a.mavenSettingsFile)
		}
		a.log.V(1).Info("Maven settings file validated", "path", a.mavenSettingsFile)
	}

	// Validate input path exists
	if a.input != "" {
		if _, err := os.Stat(a.input); err != nil {
			return fmt.Errorf(
				"Input path not found: %s\n"+
					"Specified with --input flag but path does not exist",
				a.input)
		}
	}

	// Check if any provider ports are already in use
	for provName, provInit := range a.providersMap {
		address := fmt.Sprintf("localhost:%d", provInit.port)
		listener, err := net.Listen("tcp", address)
		if err != nil {
			// Port is already in use
			return fmt.Errorf(
				"port %d required for %s provider is already in use\n"+
					"Troubleshooting:\n"+
					"  1. Check what's using the port: lsof -i :%d\n"+
					"  2. Stop old provider containers: podman stop $(podman ps -a | grep provider | awk '{print $1}')\n"+
					"  3. Kill the process using the port, or restart your system",
				provInit.port, provName, provInit.port)
		}
		listener.Close()
		a.log.V(2).Info("port is available", "provider", provName, "port", provInit.port)
	}

	a.log.V(1).Info("provider configuration validated successfully")
	return nil
}

// waitForProvider polls a provider's port until it's ready or timeout is reached.
// This replaces the hardcoded 4-second sleep with proper health checking.
func waitForProvider(ctx context.Context, providerName string, port int, timeout time.Duration, log logr.Logger) error {
	deadline := time.Now().Add(timeout)
	address := fmt.Sprintf("localhost:%d", port)
	backoff := 100 * time.Millisecond
	maxBackoff := 2 * time.Second

	log.V(1).Info("waiting for provider to become ready", "provider", providerName, "address", address, "timeout", timeout)

	for time.Now().Before(deadline) {
		// Check if context was cancelled
		select {
		case <-ctx.Done():
			return fmt.Errorf("provider health check cancelled: %w", ctx.Err())
		default:
		}

		// Attempt TCP connection to check if port is open
		conn, err := net.DialTimeout("tcp", address, 1*time.Second)
		if err == nil {
			conn.Close()
			log.V(1).Info("provider is ready", "provider", providerName, "address", address)
			return nil
		}

		// Port not ready yet, wait with exponential backoff
		log.V(2).Info("provider not ready yet, retrying", "provider", providerName, "error", err, "backoff", backoff)

		select {
		case <-ctx.Done():
			return fmt.Errorf("provider health check cancelled: %w", ctx.Err())
		case <-time.After(backoff):
			// Exponential backoff with max cap
			backoff = backoff * 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}

	return fmt.Errorf(
		"provider %s failed to become ready at %s within %v\n"+
			"Troubleshooting:\n"+
			"  1. Check container is running: podman ps | grep provider\n"+
			"  2. Check container logs: podman logs <container-name>\n"+
			"  3. Check port availability: lsof -i :%d\n"+
			"  4. Verify provider image: podman images | grep provider",
		providerName, address, timeout, port)
}

// setupNetworkProvider creates a network-based provider client for hybrid mode.
// The provider runs in a container and this client connects via network (localhost:PORT).
// This function works for all provider types (Java, Go, Python, NodeJS, Dotnet).
func (a *analyzeCommand) setupNetworkProvider(ctx context.Context, providerName string, analysisLog logr.Logger) (provider.InternalProviderClient, []string, []provider.InitConfig, error) {
	provInit, ok := a.providersMap[providerName]
	if !ok {
		return nil, nil, nil, fmt.Errorf(
			"%s provider not found in providersMap\n"+
				"This indicates a programming error - provider should have been initialized before calling setupNetworkProvider",
			providerName)
	}

	// Build provider-specific configuration
	// Based on working hybrid-provider-settings.json example
	providerSpecificConfig := map[string]interface{}{}

	// Add provider-specific LSP configuration
	switch providerName {
	case util.JavaProvider:
		// Java provider configuration for network mode
		// Paths are inside the provider container
		providerSpecificConfig["lspServerName"] = providerName
		providerSpecificConfig["lspServerPath"] = "/jdtls/bin/jdtls"
		providerSpecificConfig["bundles"] = "/jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar"
		providerSpecificConfig["depOpenSourceLabelsFile"] = "/usr/local/etc/maven.default.index"
		if a.mavenSettingsFile != "" {
			providerSpecificConfig["mavenSettingsFile"] = a.mavenSettingsFile
		}

	case util.GoProvider:
		providerSpecificConfig["lspServerName"] = "generic"
		providerSpecificConfig["lspServerName"] = "generic"
		providerSpecificConfig[provider.LspServerPathConfigKey] = "/usr/local/bin/gopls"
		providerSpecificConfig["workspaceFolders"] = []string{fmt.Sprintf("file://%s", util.SourceMountPath)}
		providerSpecificConfig["dependencyProviderPath"] = "/usr/local/bin/golang-dependency-provider"

	case util.PythonProvider:
		providerSpecificConfig["lspServerName"] = "generic"
		providerSpecificConfig[provider.LspServerPathConfigKey] = "/usr/local/bin/pylsp"
		providerSpecificConfig["workspaceFolders"] = []string{fmt.Sprintf("file://%s", util.SourceMountPath)}

	case util.NodeJSProvider:
		providerSpecificConfig["lspServerName"] = "nodejs"
		providerSpecificConfig[provider.LspServerPathConfigKey] = "/usr/local/bin/typescript-language-server"
		providerSpecificConfig["lspServerArgs"] = []string{"--stdio"}
		providerSpecificConfig["workspaceFolders"] = []string{fmt.Sprintf("file://%s", util.SourceMountPath)}

	case util.DotnetProvider:
		providerSpecificConfig[provider.LspServerPathConfigKey] = "C:/Users/ContainerAdministrator/.dotnet/tools/csharp-ls.exe"
	}

	// Create network-based provider config
	// Key difference from containerless: Address is set, BinaryPath is empty
	// Location must be the path INSIDE the container where source is mounted

	// Create proxy config - must be a pointer, default to empty proxy if none configured
	proxyConfig := &provider.Proxy{}
	if a.httpProxy != "" || a.httpsProxy != "" {
		proxyConfig = &provider.Proxy{
			HTTPProxy:  a.httpProxy,
			HTTPSProxy: a.httpsProxy,
			NoProxy:    a.noProxy,
		}
	}

	providerConfig := provider.Config{
		Name:       providerName,
		Address:    fmt.Sprintf("localhost:%d", provInit.port), // Connect to containerized provider
		BinaryPath: "",                                          // Empty = network mode
		InitConfig: []provider.InitConfig{
			{
				Location:               util.SourceMountPath, // Path inside provider container
				AnalysisMode:           provider.AnalysisMode(a.mode),
				ProviderSpecificConfig: providerSpecificConfig,
				Proxy:                  proxyConfig, // Keep as pointer - InitConfig.Proxy is *Proxy!
			},
		},
	}
	providerConfig.ContextLines = a.contextLines

	providerLocations := []string{}
	for _, ind := range providerConfig.InitConfig {
		providerLocations = append(providerLocations, ind.Location)
	}

	// Create network-based provider client (connects to localhost:PORT)
	// Use lib.GetProviderClient for all providers in network mode
	// This creates a gRPC client that connects to the containerized provider
	var providerClient provider.InternalProviderClient
	var err error
	providerClient, err = lib.GetProviderClient(providerConfig, analysisLog)
	if err != nil {
		return nil, nil, nil, fmt.Errorf(
			"failed to create %s provider client: %w\n"+
				"This usually indicates a configuration error or invalid provider type",
			providerName, err)
	}

	a.log.V(1).Info("starting network-based provider", "provider", providerName, "address", providerConfig.Address)
	initCtx, _ := tracing.StartNewSpan(ctx, "init")
	// Pass empty slice instead of nil - gRPC might not handle nil properly
	additionalBuiltinConfs, err := providerClient.ProviderInit(initCtx, []provider.InitConfig{})
	if err != nil {
		a.log.Error(err, "unable to init the provider", "provider", providerName)
		return nil, nil, nil, fmt.Errorf(
			"failed to initialize %s provider at %s: %w\n"+
				"Troubleshooting:\n"+
				"  1. Check provider container is running: podman ps | grep %s\n"+
				"  2. Check provider logs: podman logs <container-name>\n"+
				"  3. Verify network connectivity: curl localhost:%d\n"+
				"  4. Check provider health: podman inspect <container-name>\n"+
				"  5. Try restarting: podman stop <container-name>",
			providerName, providerConfig.Address, err, providerName, provInit.port)
	}

	return providerClient, providerLocations, additionalBuiltinConfs, nil
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
		// Validate configuration before starting containers
		if err := a.validateProviderConfig(); err != nil {
			return fmt.Errorf("provider configuration validation failed: %w", err)
		}

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

		// Wait for providers to become ready with health checks
		a.log.Info("waiting for providers to become ready...")
		for provName, provInit := range a.providersMap {
			if err := waitForProvider(ctx, provName, provInit.port, 30*time.Second, a.log); err != nil {
				return fmt.Errorf("provider health check failed: %w", err)
			}
		}
		a.log.Info("all providers are ready")
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

	// Setup network-based provider clients for all configured providers
	for provName := range a.providersMap {
		a.log.Info("setting up network provider", "provider", provName)
		provClient, locs, configs, err := a.setupNetworkProvider(ctx, provName, analyzeLog)
		if err != nil {
			errLog.Error(err, "unable to start provider", "provider", provName)
			os.Exit(1)
		}
		providers[provName] = provClient
		providerLocations = append(providerLocations, locs...)
		additionalBuiltinConfigs = append(additionalBuiltinConfigs, configs...)
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
