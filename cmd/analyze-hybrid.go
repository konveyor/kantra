package cmd

import (
	"context"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
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
	"github.com/konveyor/analyzer-lsp/progress"
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

	// Validate override provider settings file if specified
	if a.overrideProviderSettings != "" {
		if _, err := os.Stat(a.overrideProviderSettings); err != nil {
			return fmt.Errorf(
				"Override provider settings file not found: %s\n"+
					"Specified with --override-provider-settings flag but file does not exist",
				a.overrideProviderSettings)
		}
		a.log.V(1).Info("Override provider settings file validated", "path", a.overrideProviderSettings)
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
	backoff := 50 * time.Millisecond
	maxBackoff := 500 * time.Millisecond

	log.V(1).Info("waiting for provider to become ready", "provider", providerName, "address", address, "timeout", timeout)

	for time.Now().Before(deadline) {
		// Check if context was cancelled
		select {
		case <-ctx.Done():
			return fmt.Errorf("provider health check cancelled: %w", ctx.Err())
		default:
		}

		// Attempt TCP connection to check if port is open
		conn, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
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
	initCtx, initSpan := tracing.StartNewSpan(ctx, "init")
	defer initSpan.End()
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
	// Note: For binary files in hybrid mode, skip this as decompilation happens in container
	var javaTargetPaths []interface{}
	if !a.isFileInput {
		// Only walk target paths for source code analysis
		var err error
		javaTargetPaths, err = kantraProvider.WalkJavaPathForTarget(a.log, a.isFileInput, a.input)
		if err != nil {
			a.log.Error(err, "error getting target subdir in Java project - some duplicate incidents may occur")
		}
	} else {
		a.log.V(1).Info("skipping target directory walk for binary input (decompilation happens in container)")
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
	startTotal := time.Now()

	// Create a conditional logger that only outputs in --no-progress mode
	// In progress mode, operational messages are suppressed to avoid interfering with the progress bar
	var operationalLog logr.Logger
	if a.noProgress {
		operationalLog = a.log
	} else {
		operationalLog = logr.Discard()
	}

	operationalLog.Info("[TIMING] Hybrid analysis starting")
	operationalLog.Info("running analysis in hybrid mode (analyzer in-process, providers in containers)")

	// Hide cursor at the very start if progress is enabled
	if !a.noProgress {
		fmt.Fprintf(os.Stderr, "\033[?25l")
		// Ensure cursor is shown at the end
		defer fmt.Fprintf(os.Stderr, "\033[?25h")
	}

	// Show simplified message in progress mode
	if !a.noProgress {
		// Detect if this is binary analysis based on file extension
		isBinaryAnalysis := false
		if a.isFileInput {
			ext := filepath.Ext(a.input)
			isBinaryAnalysis = (ext == util.JavaArchive || ext == util.WebArchive ||
				ext == util.EnterpriseArchive || ext == util.ClassFile)
		}

		if isBinaryAnalysis {
			fmt.Fprintf(os.Stderr, "Running binary analysis...\n")
		} else {
			fmt.Fprintf(os.Stderr, "Running source analysis...\n")
		}
	}

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
	// but only if progress is disabled (to avoid interfering with progress bar)
	if a.noProgress {
		consoleHook := &ConsoleHook{Level: logrus.InfoLevel, Log: a.log}
		logrusAnalyzerLog.AddHook(consoleHook)
	}

	analyzeLog := logrusr.New(logrusAnalyzerLog)

	// Error logging to stderr
	logrusErrLog := logrus.New()
	logrusErrLog.SetOutput(os.Stderr)
	errLog := logrusr.New(logrusErrLog)

	// Setup label selectors
	operationalLog.Info("running source analysis")
	labelSelectors := a.getLabelSelector()

	selectors := []engine.RuleSelector{}
	if labelSelectors != "" {
		selector, err := labels.NewLabelSelector[*engine.RuleMeta](labelSelectors, nil)
		if err != nil {
			errLog.Error(err, "failed to create label selector from expression", "selector", labelSelectors)
			return fmt.Errorf("failed to create label selector from expression %q: %w", labelSelectors, err)
		}
		selectors = append(selectors, selector)
	}

	var dependencyLabelSelector *labels.LabelSelector[*konveyor.Dep]
	depLabel := fmt.Sprintf("!%v=open-source", provider.DepSourceLabel)
	if !a.analyzeKnownLibraries {
		dependencyLabelSelector, err = labels.NewLabelSelector[*konveyor.Dep](depLabel, nil)
		if err != nil {
			errLog.Error(err, "failed to create label selector from expression", "selector", depLabel)
			return fmt.Errorf("failed to create label selector from expression %q: %w", depLabel, err)
		}
	}

	// Start containerized providers if any
	if len(a.providersMap) > 0 {
		startProviderSetup := time.Now()
		operationalLog.Info("[TIMING] Starting provider container setup")

		// Parallelize independent startup tasks for better performance
		type startupResult struct {
			name string
			err  error
		}

		var wg sync.WaitGroup
		var volName string
		var rulesetsDir string
		resultChan := make(chan startupResult, 3)

		// Task 1: Validate provider configuration
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := a.validateProviderConfig()
			resultChan <- startupResult{name: "config validation", err: err}
		}()

		// Task 2: Create container volume
		wg.Add(1)
		go func() {
			defer wg.Done()
			vol, err := a.createContainerVolume(a.input)
			if err == nil {
				volName = vol
			}
			resultChan <- startupResult{name: "volume creation", err: err}
		}()

		// Task 3: Extract default rulesets (if enabled)
		if a.enableDefaultRulesets {
			wg.Add(1)
			go func() {
				defer wg.Done()
				dir, err := a.extractDefaultRulesets(ctx, operationalLog)
				if err == nil {
					rulesetsDir = dir
				}
				resultChan <- startupResult{name: "ruleset extraction", err: err}
			}()
		}

		// Wait for all startup tasks to complete
		wg.Wait()
		close(resultChan)

		// Check for errors from any task
		for result := range resultChan {
			if result.err != nil {
				return fmt.Errorf("%s failed: %w", result.name, result.err)
			}
		}

		// Add extracted rulesets to rules list if we got any
		if rulesetsDir != "" {
			a.rules = append(a.rules, rulesetsDir)
		}

		// For binary files, util.SourceMountPath includes the filename (e.g., /opt/input/source/app.war)
		// But volume mounts need the parent directory. Save and restore it.
		originalMountPath := util.SourceMountPath
		if a.isFileInput {
			// Temporarily set to parent directory for volume mounting
			util.SourceMountPath = path.Dir(util.SourceMountPath)
			a.log.V(1).Info("adjusted mount path for binary file",
				"original", originalMountPath,
				"adjusted", util.SourceMountPath)
		}

		if !a.noProgress {
			fmt.Fprintf(os.Stderr, "  ✓ Created volume\n")
		}

		// Start providers with port publishing
		err = a.RunProvidersHostNetwork(ctx, volName, 5, operationalLog)

		// Restore original mount path for provider configuration
		if a.isFileInput {
			util.SourceMountPath = originalMountPath
			a.log.V(1).Info("restored mount path", "path", util.SourceMountPath)
		}

		if err != nil {
			return fmt.Errorf("failed to start providers: %w", err)
		}

		if !a.noProgress {
			fmt.Fprintf(os.Stderr, "  ✓ Started provider containers\n")
		}

		// Wait for providers to become ready with health checks (in parallel)
		operationalLog.Info("waiting for providers to become ready...")

		// Parallel health checks with proper error handling
		type providerHealthResult struct {
			providerName string
			err          error
		}
		healthChan := make(chan providerHealthResult, len(a.providersMap))

		// Start health checks in parallel
		for provName, provInit := range a.providersMap {
			provName := provName // capture loop variable
			provInit := provInit
			go func() {
				err := waitForProvider(ctx, provName, provInit.port, 30*time.Second, operationalLog)
				healthChan <- providerHealthResult{providerName: provName, err: err}
			}()
		}

		// Collect results
		for i := 0; i < len(a.providersMap); i++ {
			result := <-healthChan
			if result.err != nil {
				return fmt.Errorf("provider %s health check failed: %w", result.providerName, result.err)
			}
		}

		operationalLog.Info("all providers are ready")
		operationalLog.Info("[TIMING] Provider container setup complete", "duration_ms", time.Since(startProviderSetup).Milliseconds())
	}

	// Setup provider clients
	providers := map[string]provider.InternalProviderClient{}
	providerLocations := []string{}

	// Get Java target paths to exclude from builtin
	// Note: For binary files in hybrid mode, skip this as decompilation happens in container
	var javaTargetPaths []interface{}
	if !a.isFileInput {
		// Only walk target paths for source code analysis
		var err error
		javaTargetPaths, err = kantraProvider.WalkJavaPathForTarget(a.log, a.isFileInput, a.input)
		if err != nil {
			a.log.Error(err, "error getting target subdir in Java project - some duplicate incidents may occur")
		}
	} else {
		a.log.V(1).Info("skipping target directory walk for binary input (decompilation happens in container)")
	}

	var additionalBuiltinConfigs []provider.InitConfig

	// Setup network-based provider clients for all configured providers
	for provName := range a.providersMap {
		operationalLog.Info("setting up network provider", "provider", provName)
		provClient, locs, configs, err := a.setupNetworkProvider(ctx, provName, analyzeLog)
		if err != nil {
			errLog.Error(err, "unable to start provider", "provider", provName)
			return fmt.Errorf("unable to start provider %s: %w", provName, err)
		}
		providers[provName] = provClient
		providerLocations = append(providerLocations, locs...)
		additionalBuiltinConfigs = append(additionalBuiltinConfigs, configs...)
	}

	// CRITICAL FIX: Transform container paths to host paths
	// The Java provider runs in a container and returns configs with container paths (/opt/input/source).
	// The builtin provider runs on the host and needs host paths (a.input).
	// We must transform these paths or builtin provider won't find any files!
	hostRoot := a.input
	containerRoot := util.SourceMountPath
	if a.isFileInput {
		// For binary files, use parent directory as hostRoot
		hostRoot = filepath.Dir(a.input)
		containerRoot = path.Dir(util.SourceMountPath)
	}

	transformedConfigs := make([]provider.InitConfig, len(additionalBuiltinConfigs))
	for i, conf := range additionalBuiltinConfigs {
		transformedConfigs[i] = conf
		// Replace container path prefix with host path
		if strings.HasPrefix(conf.Location, containerRoot) {
			rel := strings.TrimPrefix(conf.Location, containerRoot)
			rel = strings.TrimPrefix(rel, "/")
			if rel == "" {
				transformedConfigs[i].Location = hostRoot
			} else {
				transformedConfigs[i].Location = filepath.Join(hostRoot, rel)
			}
		}
	}

	// Setup builtin provider (always in-process)
	builtinProvider, builtinLocations, err := a.setupBuiltinProviderHybrid(ctx, javaTargetPaths, transformedConfigs, analyzeLog)
	if err != nil {
		errLog.Error(err, "unable to start builtin provider")
		return fmt.Errorf("unable to start builtin provider: %w", err)
	}
	providers["builtin"] = builtinProvider
	providerLocations = append(providerLocations, builtinLocations...)

	// Show provider initialization completion in progress mode
	if !a.noProgress {
		// Build provider names dynamically from the providers map
		providerNames := make([]string, 0, len(providers))
		for name := range providers {
			providerNames = append(providerNames, name)
		}
		sort.Strings(providerNames) // Sort for consistent output
		fmt.Fprintf(os.Stderr, "  ✓ Initialized providers (%s)\n", strings.Join(providerNames, ", "))
	}

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

	if !a.noProgress {
		fmt.Fprintf(os.Stderr, "  ✓ Started rules engine\n")
	}

	// Load rules in parallel for better performance
	ruleSets := []engine.RuleSet{}
	needProviders := map[string]provider.InternalProviderClient{}

	// Note: Default rulesets extraction now happens earlier in parallel with
	// volume creation and config validation for better performance

	startRuleLoading := time.Now()
	operationalLog.Info("[TIMING] Starting rule loading")

	// Parallelize rule loading across multiple rulesets
	type ruleLoadResult struct {
		rulePath     string
		ruleSets     []engine.RuleSet
		providers    map[string]provider.InternalProviderClient
		err          error
	}

	var ruleWg sync.WaitGroup
	resultChan := make(chan ruleLoadResult, len(a.rules))

	// Load each ruleset in parallel
	for _, f := range a.rules {
		ruleWg.Add(1)
		go func(rulePath string) {
			defer ruleWg.Done()
			operationalLog.Info("parsing rules for analysis", "rules", rulePath)

			internRuleSet, internNeedProviders, err := ruleParser.LoadRules(rulePath)
			if err != nil {
				a.log.Error(err, "unable to parse all the rules for ruleset", "file", rulePath)
			}

			resultChan <- ruleLoadResult{
				rulePath:  rulePath,
				ruleSets:  internRuleSet,
				providers: internNeedProviders,
				err:       err,
			}
		}(f)
	}

	// Wait for all rule loading to complete
	ruleWg.Wait()
	close(resultChan)

	// Collect and merge results
	for result := range resultChan {
		ruleSets = append(ruleSets, result.ruleSets...)
		for k, v := range result.providers {
			needProviders[k] = v
		}
	}

	operationalLog.Info("[TIMING] Rule loading complete", "duration_ms", time.Since(startRuleLoading).Milliseconds())

	// Start dependency analysis for full analysis mode
	wg := &sync.WaitGroup{}
	var depSpan trace.Span
	if a.mode == string(provider.FullAnalysisMode) {
		_, hasJava := a.providersMap[util.JavaProvider]
		if hasJava {
			var depCtx context.Context
			depCtx, depSpan = tracing.StartNewSpan(ctx, "dep")
			wg.Add(1)

			operationalLog.Info("running dependency analysis")
			go a.DependencyOutputContainerless(depCtx, providers, "dependencies.yaml", wg)
		}
	}

	// Run rules with progress reporting
	startRuleExecution := time.Now()
	operationalLog.Info("[TIMING] Starting rule execution")
	operationalLog.Info("evaluating rules for violations. see analysis.log for more info")

	// Create progress reporter (or noop if disabled)
	var reporter progress.ProgressReporter
	var progressDone chan struct{}
	var progressCancel context.CancelFunc

	if !a.noProgress {
		// Create channel-based progress reporter
		var progressCtx context.Context
		progressCtx, progressCancel = context.WithCancel(ctx)
		defer progressCancel()
		channelReporter := progress.NewChannelReporter(progressCtx)
		reporter = channelReporter

		// Start goroutine to consume progress events and render progress bar
		progressDone = make(chan struct{})
		go func() {
			defer close(progressDone)

			// Track cumulative progress across all rulesets
			var cumulativeTotal int
			var completedFromPreviousRulesets int
			var lastRulesetTotal int
			var justPrintedLoadedRules bool

			for event := range channelReporter.Events() {
				switch event.Stage {
				case progress.StageProviderInit:
					// Skip provider init messages - we show them earlier
				case progress.StageRuleParsing:
					if event.Total > 0 {
						cumulativeTotal += event.Total
						fmt.Fprintf(os.Stderr, "  ✓ Loaded %d rules\n\n", cumulativeTotal)
						justPrintedLoadedRules = true
					}
				case progress.StageRuleExecution:
					if event.Total > 0 {
						// Initialize cumulativeTotal from first event if not set by rule parsing
						if cumulativeTotal == 0 {
							cumulativeTotal = event.Total
							fmt.Fprintf(os.Stderr, "  ✓ Loaded %d rules\n\n", cumulativeTotal)
							justPrintedLoadedRules = true
						}

						// Skip first progress bar render right after printing "Loaded rules"
						if justPrintedLoadedRules {
							justPrintedLoadedRules = false
							continue
						}

						// Detect if we've moved to a new ruleset
						// This happens when event.Total changes
						if lastRulesetTotal > 0 && event.Total != lastRulesetTotal {
							// We've moved to a new ruleset
							completedFromPreviousRulesets += lastRulesetTotal
						}
						lastRulesetTotal = event.Total

						// Calculate overall progress
						totalCompleted := completedFromPreviousRulesets + event.Current

						overallPercent := (totalCompleted * 100) / cumulativeTotal
						renderProgressBar(overallPercent, totalCompleted, cumulativeTotal, event.Message)
					}
				case progress.StageComplete:
					// Move to next line and print completion
					fmt.Fprintf(os.Stderr, "\n\n") // Move past progress bar, blank line
					fmt.Fprintf(os.Stderr, "Analysis complete!\n") // Single line after
				}
			}
		}()
	} else {
		// Use noop reporter when progress is disabled
		reporter = progress.NewNoopReporter()
	}

	// Run analysis with progress reporter
	rulesets := eng.RunRulesWithOptions(ctx, ruleSets, []engine.RunOption{
		engine.WithProgressReporter(reporter),
	}, selectors...)

	// Cancel progress context and wait for goroutine to finish
	if !a.noProgress {
		progressCancel()  // This closes the Events() channel
		<-progressDone    // Wait for goroutine to finish
	}

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
	operationalLog.Info("[TIMING] Rule execution complete", "duration_ms", time.Since(startRuleExecution).Milliseconds())

	// Sort rulesets
	sort.SliceStable(rulesets, func(i, j int) bool {
		return rulesets[i].Name < rulesets[j].Name
	})

	// Write results
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

	// Create JSON output if requested
	err = a.CreateJSONOutput()
	if err != nil {
		a.log.Error(err, "failed to create json output file")
		return err
	}
	operationalLog.Info("[TIMING] Output writing complete", "duration_ms", time.Since(startWriting).Milliseconds())

	// Close analysis log before generating static report
	analysisLog.Close()

	// Generate static report
	startStaticReport := time.Now()
	operationalLog.Info("[TIMING] Starting static report generation")
	err = a.GenerateStaticReport(ctx, operationalLog)
	if err != nil {
		a.log.Error(err, "failed to generate static report")
		return err
	}
	operationalLog.Info("[TIMING] Static report generation complete", "duration_ms", time.Since(startStaticReport).Milliseconds())

	// Print results summary (only in progress mode, not in --no-progress mode)
	if !a.noProgress {
		fmt.Fprintf(os.Stderr, "\nResults:\n")
		reportPath := filepath.Join(a.output, "static-report", "index.html")
		fmt.Fprintf(os.Stderr, "  Report: file://%s\n", reportPath)
		analysisLogPath := filepath.Join(a.output, "analysis.log")
		fmt.Fprintf(os.Stderr, "  Logs:   %s\n", analysisLogPath)
	}

	operationalLog.Info("[TIMING] Hybrid analysis complete", "total_duration_ms", time.Since(startTotal).Milliseconds())
	operationalLog.Info("hybrid analysis completed successfully")
	return nil
}
