package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bombsimon/logrusr/v3"
	"github.com/go-logr/logr"
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

// validateProviderConfig validates hybrid-mode-specific configuration before starting provider containers.
// This catches configuration errors early and provides helpful error messages.
// Note: Maven settings and input path are already validated in the PreRunE Validate() function.
func (a *analyzeCommand) validateProviderConfig() error {
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

// loadOverrideProviderSettings loads provider configuration overrides from a JSON file.
// Returns a slice of provider.Config objects that will be merged with default configs.
func (a *analyzeCommand) loadOverrideProviderSettings() ([]provider.Config, error) {
	if a.overrideProviderSettings == "" {
		return nil, nil
	}

	a.log.V(1).Info("loading override provider settings", "file", a.overrideProviderSettings)

	data, err := os.ReadFile(a.overrideProviderSettings)
	if err != nil {
		return nil, fmt.Errorf("failed to read override provider settings file: %w", err)
	}

	var overrideConfigs []provider.Config
	if err := json.Unmarshal(data, &overrideConfigs); err != nil {
		return nil, fmt.Errorf("failed to parse override provider settings JSON: %w", err)
	}

	a.log.V(1).Info("loaded override provider settings", "providers", len(overrideConfigs))
	return overrideConfigs, nil
}

// mergeProviderSpecificConfig merges override config into base config.
// Override values take precedence over base values.
func mergeProviderSpecificConfig(base, override map[string]interface{}) map[string]interface{} {
	if override == nil {
		return base
	}

	result := make(map[string]interface{})
	// Copy base config
	for k, v := range base {
		result[k] = v
	}
	// Apply overrides
	for k, v := range override {
		result[k] = v
	}
	return result
}

// applyProviderOverrides applies override settings to a provider config.
// Returns the modified config with overrides applied.
func applyProviderOverrides(baseConfig provider.Config, overrideConfigs []provider.Config) provider.Config {
	if overrideConfigs == nil {
		return baseConfig
	}

	// Find matching override config by provider name
	for _, override := range overrideConfigs {
		if override.Name != baseConfig.Name {
			continue
		}

		// Apply top-level config overrides
		if override.ContextLines != 0 {
			baseConfig.ContextLines = override.ContextLines
		}
		if override.Proxy != nil {
			baseConfig.Proxy = override.Proxy
		}

		// Merge InitConfig settings
		if len(override.InitConfig) > 0 && len(baseConfig.InitConfig) > 0 {
			// Merge ProviderSpecificConfig from first InitConfig
			if override.InitConfig[0].ProviderSpecificConfig != nil {
				baseConfig.InitConfig[0].ProviderSpecificConfig = mergeProviderSpecificConfig(
					baseConfig.InitConfig[0].ProviderSpecificConfig,
					override.InitConfig[0].ProviderSpecificConfig,
				)
			}
			// Apply other InitConfig fields
			if override.InitConfig[0].AnalysisMode != "" {
				baseConfig.InitConfig[0].AnalysisMode = override.InitConfig[0].AnalysisMode
			}
		}
		break
	}

	return baseConfig
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
// This function works for all provider types (Java, Go, Python, NodeJS, C#).
func (a *analyzeCommand) setupNetworkProvider(ctx context.Context, providerName string, analysisLog logr.Logger, overrideConfigs []provider.Config, progressReporter progress.ProgressReporter) (provider.InternalProviderClient, []string, []provider.InitConfig, error) {
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
		providerSpecificConfig["lspServerPath"] = JDTLSBinLocation
		providerSpecificConfig["bundles"] = JavaBundlesLocation
		providerSpecificConfig["mavenIndexPath"] = MavenIndexPath
		providerSpecificConfig["depOpenSourceLabelsFile"] = DepOpenSourceLabels
		if a.mavenSettingsFile != "" {
			// Use container path where settings.xml is mounted (copied by getConfigVolumes)
			providerSpecificConfig["mavenSettingsFile"] = path.Join(util.ConfigMountPath, "settings.xml")
		}

	case util.GoProvider:
		providerSpecificConfig["lspServerName"] = "generic"
		providerSpecificConfig[provider.LspServerPathConfigKey] = "/usr/local/bin/gopls"
		// Note: Don't set workspaceFolders - the generic Init method will use Location
		// Setting both would cause duplicate file counting in GetDocumentUris
		providerSpecificConfig["dependencyProviderPath"] = "/usr/local/bin/golang-dependency-provider"

	case util.PythonProvider:
		providerSpecificConfig["lspServerName"] = "generic"
		providerSpecificConfig[provider.LspServerPathConfigKey] = "/usr/local/bin/pylsp"
		// Note: Don't set workspaceFolders - the generic Init method will use Location
		// Setting both would cause duplicate file counting in GetDocumentUris

	case util.NodeJSProvider:
		providerSpecificConfig["lspServerName"] = "nodejs"
		providerSpecificConfig[provider.LspServerPathConfigKey] = "/usr/local/bin/typescript-language-server"
		providerSpecificConfig["lspServerArgs"] = []interface{}{"--stdio"}
		// Set workspaceFolders for nodejs provider (used alongside Location)
		// Fix merged in analyzer-lsp#1036 prevents duplicate file counting
		providerSpecificConfig["workspaceFolders"] = []interface{}{fmt.Sprintf("file://%s", util.SourceMountPath)}

	case util.CsharpProvider:
		providerSpecificConfig["ilspy_cmd"] = "/usr/local/bin/ilspycmd"
		providerSpecificConfig["paket_cmd"] = "/usr/local/bin/paket"
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
		BinaryPath: "",                                         // Empty = network mode
		InitConfig: []provider.InitConfig{
			{
				Location:               util.SourceMountPath,
				AnalysisMode:           provider.AnalysisMode(a.mode),
				ProviderSpecificConfig: providerSpecificConfig,
				Proxy:                  proxyConfig, // Keep as pointer - InitConfig.Proxy is *Proxy!
			},
		},
	}
	providerConfig.ContextLines = a.contextLines

	// Add prepare progress reporter if available
	// Note: Only set on InitConfig level to avoid duplicate progress events
	if progressReporter != nil {
		for i := range providerConfig.InitConfig {
			providerConfig.InitConfig[i].PrepareProgressReporter = provider.NewPrepareProgressAdapter(progressReporter)
		}
	}

	// Apply override provider settings if provided
	providerConfig = applyProviderOverrides(providerConfig, overrideConfigs)

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

// runParallelStartupTasks executes independent startup tasks concurrently for better performance.
// Returns the volume name and rulesets directory on success.
func (a *analyzeCommand) runParallelStartupTasks(ctx context.Context, containerLogWriter io.Writer) (volName string, rulesetsDir string, err error) {
	type startupResult struct {
		name string
		err  error
	}

	var wg sync.WaitGroup
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
			dir, err := a.extractDefaultRulesets(ctx, containerLogWriter)
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
			// Clean up any successfully created resources before returning
			if volName != "" {
				cmd := exec.CommandContext(ctx, Settings.ContainerBinary, "volume", "rm", volName)
				if cleanupErr := cmd.Run(); cleanupErr != nil {
					a.log.Error(cleanupErr, "failed to cleanup volume after startup failure")
				}
			}
			return "", "", fmt.Errorf("%s failed: %w", result.name, result.err)
		}
	}

	return volName, rulesetsDir, nil
}

// setupBuiltinProviderHybrid creates a builtin provider for hybrid mode.
// This is the same as containerless mode since builtin always runs in-process.
func (a *analyzeCommand) setupBuiltinProviderHybrid(ctx context.Context, additionalConfigs []provider.InitConfig, analysisLog logr.Logger, overrideConfigs []provider.Config, progressReporter progress.ProgressReporter) (provider.InternalProviderClient, []string, error) {
	a.log.V(1).Info("setting up builtin provider for hybrid mode")

	providerSpecificConfig := map[string]interface{}{
		// Don't set excludedDirs - let analyzer-lsp use default exclusions
		// (node_modules, vendor, dist, build, target, .git, .venv, venv)
	}

	// Check if profiles directory exists in input and add to excludedDirs
	if excludedDir := util.GetProfilesExcludedDir(a.input, false); excludedDir != "" {
		providerSpecificConfig["excludedDirs"] = []interface{}{excludedDir}
	}

	builtinConfig := provider.Config{
		Name:       "builtin",
		InitConfig: []provider.InitConfig{},
	}
	if !a.isFileInput {
		builtinConfig.InitConfig = append(builtinConfig.InitConfig, provider.InitConfig{
			Location:               a.input,
			AnalysisMode:           provider.AnalysisMode(a.mode),
			ProviderSpecificConfig: providerSpecificConfig,
		})
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

	// Apply override settings (same as containerized providers)
	builtinConfig = applyProviderOverrides(builtinConfig, overrideConfigs)

	// Add prepare progress reporter if available
	// Note: Only set on InitConfig level to avoid duplicate progress events
	if progressReporter != nil {
		for i := range builtinConfig.InitConfig {
			builtinConfig.InitConfig[i].PrepareProgressReporter = provider.NewPrepareProgressAdapter(progressReporter)
		}
	}

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
	// Create progress mode to encapsulate progress reporting behavior
	progressMode := NewProgressMode(a.noProgress)

	// Override a.log to use the conditional logger that only outputs in --no-progress mode
	// In progress mode, operational messages are suppressed to avoid interfering with the progress bar
	a.log = progressMode.OperationalLogger(a.log)

	a.log.Info("[TIMING] Hybrid analysis starting")
	a.log.Info("running analysis in hybrid mode (analyzer in-process, providers in containers)")

	// Initialize Jaeger tracing if endpoint is provided
	if a.jaegerEndpoint != "" {
		a.log.Info("initializing Jaeger tracing", "endpoint", a.jaegerEndpoint)
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
		a.log.Info("Jaeger tracing initialized successfully")
	}

	// Hide cursor at the very start if progress is enabled
	progressMode.HideCursor()
	// Ensure cursor is shown at the end
	defer progressMode.ShowCursor()

	// Show simplified message in progress mode
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
	if progressMode.ShouldAddConsoleHook() {
		consoleHook := &ConsoleHook{Level: logrus.InfoLevel, Log: a.log}
		logrusAnalyzerLog.AddHook(consoleHook)
	}

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

	// Load override provider settings if specified
	overrideConfigs, err := a.loadOverrideProviderSettings()
	if err != nil {
		errLog.Error(err, "failed to load override provider settings")
		return fmt.Errorf("failed to load override provider settings: %w", err)
	}
	if overrideConfigs != nil {
		a.log.Info("loaded override provider settings", "file", a.overrideProviderSettings, "providers", len(overrideConfigs))
	}

	providerToInputVolName := map[string]string{}
	// Start containerized providers if any
	if len(a.providersMap) > 0 {
		startProviderSetup := time.Now()
		a.log.Info("[TIMING] Starting provider container setup")

		// Run independent startup tasks in parallel for better performance
		volName, rulesetsDir, err := a.runParallelStartupTasks(ctx, analysisLog)
		if err != nil {
			return err
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

		progressMode.Printf("  ✓ Created volume\n")

		// Start providers with port publishing
		err = a.RunProvidersHostNetwork(ctx, volName, 5, analysisLog)

		// Restore original mount path for provider configuration
		if a.isFileInput {
			util.SourceMountPath = originalMountPath
			a.log.V(1).Info("restored mount path", "path", util.SourceMountPath)
		}

		if err != nil {
			return fmt.Errorf("failed to start providers: %w", err)
		}

		progressMode.Printf("  ✓ Started provider containers\n")

		// Wait for providers to become ready with health checks (in parallel)
		a.log.Info("waiting for providers to become ready...")

		// Parallel health checks with proper error handling
		type providerHealthResult struct {
			providerName string
			err          error
			volName      string
		}
		healthChan := make(chan providerHealthResult, len(a.providersMap))

		// Start health checks in parallel
		for provName, provInit := range a.providersMap {
			provName := provName // capture loop variable
			provInit := provInit
			go func() {
				err := waitForProvider(ctx, provName, provInit.port, 30*time.Second, a.log)
				healthChan <- providerHealthResult{providerName: provName, err: err, volName: volName}
			}()
		}

		// Collect results
		for i := 0; i < len(a.providersMap); i++ {
			result := <-healthChan
			if result.err != nil {
				return fmt.Errorf("provider %s health check failed: %w", result.providerName, result.err)
			}
			providerToInputVolName[result.providerName] = result.volName
		}

		a.log.Info("all providers are ready")
		a.log.Info("[TIMING] Provider container setup complete", "duration_ms", time.Since(startProviderSetup).Milliseconds())
	}

	// Create progress reporter early (before provider preparation)
	reporter, progressDone, progressCancel := setupProgressReporter(ctx, a.noProgress)
	if progressCancel != nil {
		defer progressCancel()
	}

	// Setup provider clients
	providers := map[string]provider.InternalProviderClient{}
	providerLocations := []string{}

	var additionalBuiltinConfigs []provider.InitConfig

	hostRoot := a.input
	containerRoot := util.SourceMountPath
	if a.isFileInput {
		// For binary files, use parent directory as hostRoot
		hostRoot = filepath.Dir(a.input)
		containerRoot = path.Dir(util.SourceMountPath)
	}
	// Setup network-based provider clients for all configured providers
	for provName := range a.providersMap {
		a.log.Info("setting up network provider", "provider", provName)
		provClient, locs, configs, err := a.setupNetworkProvider(ctx, provName, analyzeLog, overrideConfigs, reporter)
		if err != nil {
			errLog.Error(err, "unable to start provider", "provider", provName)
			// Clean up any providers that were started before this failure
			// to prevent resource leaks (containers left running)
			for _, prov := range providers {
				prov.Stop()
			}
			// Remove provider containers
			if cleanupErr := a.RmProviderContainers(ctx); cleanupErr != nil {
				errLog.Error(cleanupErr, "failed to cleanup providers after setup failure")
			}
			return fmt.Errorf("unable to start provider %s: %w", provName, err)
		}
		providers[provName] = provClient
		providerLocations = append(providerLocations, locs...)
		// CRITICAL FIX: Transform container paths to host paths
		// The Java provider runs in a container and returns configs with container paths (/opt/input/source).
		// The builtin provider runs on the host and needs host paths (a.input).
		// We must transform these paths or builtin provider won't find any files!
		providerHostRoot := hostRoot
		if isBinaryAnalysis {
			cmd := exec.CommandContext(ctx, Settings.ContainerBinary, "volume", "inspect", providerToInputVolName[provName])
			o, err := cmd.Output()
			if err == nil {
				j := []map[string]any{}
				err = json.Unmarshal(o, &j)
				if len(j) == 1 {
					found := false
					if opt, ok := j[0]["Options"]; ok {
						op, ok := opt.(map[string]any)
						if ok {
							if volPath, ok := op["device"]; ok {
								volPathString := volPath.(string)
								if strings.Contains(volPathString, "/mnt/c") && runtime.GOOS == "windows" {
									volPathString = filepath.FromSlash(strings.TrimPrefix(volPathString, "/mnt/c"))
								}

								if _, err := os.Lstat(volPathString); err == nil {
									providerHostRoot = volPathString
									found = true
								}
							}
						}
					}
					if volPath, ok := j[0]["Mountpoint"]; !found && ok {
						if _, err := os.Lstat(volPath.(string)); err == nil {
							providerHostRoot = volPath.(string)
							found = true
						}
					}
				}
			}
		}

		for _, c := range configs {
			if rel, err := filepath.Rel(containerRoot, c.Location); err == nil {
				if rel == "." {
					c.Location = providerHostRoot
				} else {
					c.Location = filepath.Join(providerHostRoot, filepath.FromSlash(rel))
				}
			}
			additionalBuiltinConfigs = append(additionalBuiltinConfigs, c)
		}
	}

	// Setup builtin provider (always in-process)
	builtinProvider, builtinLocations, err := a.setupBuiltinProviderHybrid(ctx, additionalBuiltinConfigs, analyzeLog, overrideConfigs, reporter)
	if err != nil {
		errLog.Error(err, "unable to start builtin provider")
		return fmt.Errorf("unable to start builtin provider: %w", err)
	}
	providers["builtin"] = builtinProvider
	providerLocations = append(providerLocations, builtinLocations...)

	// Show provider initialization completion in progress mode
	// Build provider names dynamically from the providers map
	providerNames := make([]string, 0, len(providers))
	for name := range providers {
		providerNames = append(providerNames, name)
	}
	sort.Strings(providerNames) // Sort for consistent output
	progressMode.Printf("  ✓ Initialized providers (%s)\n", strings.Join(providerNames, ", "))

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

	progressMode.Printf("  ✓ Started rules engine\n")

	// Load rules in parallel for better performance
	ruleSets := []engine.RuleSet{}
	needProviders := map[string]provider.InternalProviderClient{}

	// Note: Default rulesets extraction now happens earlier in parallel with
	// volume creation and config validation for better performance

	startRuleLoading := time.Now()
	a.log.Info("[TIMING] Starting rule loading")

	// Parallelize rule loading across multiple rulesets
	type ruleLoadResult struct {
		rulePath       string
		ruleSets       []engine.RuleSet
		providers      map[string]provider.InternalProviderClient
		provConditions map[string][]provider.ConditionsByCap
		err            error
	}

	var ruleWg sync.WaitGroup
	resultChan := make(chan ruleLoadResult, len(a.rules))
	providerConditions := map[string][]provider.ConditionsByCap{}
	// Load each ruleset in parallel
	for _, f := range a.rules {
		ruleWg.Add(1)
		go func(rulePath string) {
			defer ruleWg.Done()
			a.log.Info("parsing rules for analysis", "rules", rulePath)

			internRuleSet, internNeedProviders, provConditions, err := ruleParser.LoadRules(rulePath)
			if err != nil {
				a.log.Error(err, "unable to parse all the rules for ruleset", "file", rulePath)
			}

			resultChan <- ruleLoadResult{
				rulePath:       rulePath,
				ruleSets:       internRuleSet,
				providers:      internNeedProviders,
				provConditions: provConditions,
				err:            err,
			}
		}(f)
	}

	// Wait for all rule loading to complete
	ruleWg.Wait()
	close(resultChan)

	// Collect and merge results
	var ruleLoadErrors []error
	for result := range resultChan {
		if result.err != nil {
			ruleLoadErrors = append(ruleLoadErrors, fmt.Errorf("failed to load ruleset %s: %w", result.rulePath, result.err))
			continue
		}
		ruleSets = append(ruleSets, result.ruleSets...)
		for k, v := range result.providers {
			needProviders[k] = v
		}
		for k, v := range result.provConditions {
			if _, ok := providerConditions[k]; !ok {
				providerConditions[k] = []provider.ConditionsByCap{}
			}
			providerConditions[k] = append(providerConditions[k], v...)
		}
	}

	// Check if we have at least one ruleset loaded successfully
	if len(ruleSets) == 0 {
		if len(ruleLoadErrors) > 0 {
			return fmt.Errorf("failed to load any rulesets: %v", ruleLoadErrors)
		}
		return fmt.Errorf("no rulesets loaded")
	}

	// Log warnings for any failed rulesets (if we have at least one successful load)
	if len(ruleLoadErrors) > 0 {
		for _, err := range ruleLoadErrors {
			a.log.Error(err, "ruleset load failed but continuing with other rulesets")
		}
	}

	a.log.Info("[TIMING] Rule loading complete", "duration_ms", time.Since(startRuleLoading).Milliseconds())

	// prepare the providers
	for name, conditions := range providerConditions {
		if provider, ok := needProviders[name]; ok {
			if err := provider.Prepare(ctx, conditions); err != nil {
				errLog.Error(err, "unable to prepare provider", "provider", name)
			}
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

	// Run rules with progress reporting
	startRuleExecution := time.Now()
	a.log.Info("[TIMING] Starting rule execution")
	a.log.Info("evaluating rules for violations. see analysis.log for more info")

	// Run analysis with progress reporter (already created earlier)
	rulesets := eng.RunRulesWithOptions(ctx, ruleSets, []engine.RunOption{
		engine.WithProgressReporter(reporter),
	}, selectors...)

	// Cancel progress context and wait for goroutine to finish
	if progressMode.IsEnabled() {
		progressCancel() // This closes the Events() channel
		<-progressDone   // Wait for goroutine to finish
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
	a.log.Info("[TIMING] Rule execution complete", "duration_ms", time.Since(startRuleExecution).Milliseconds())

	// Sort rulesets
	sort.SliceStable(rulesets, func(i, j int) bool {
		return rulesets[i].Name < rulesets[j].Name
	})

	// Write results
	startWriting := time.Now()
	a.log.Info("[TIMING] Starting output writing")
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
	a.log.Info("[TIMING] Output writing complete", "duration_ms", time.Since(startWriting).Milliseconds())

	// Close analysis log before generating static report
	analysisLog.Close()

	// Generate static report
	startStaticReport := time.Now()
	a.log.Info("[TIMING] Starting static report generation")

	err = a.GenerateStaticReport(ctx, a.log)
	if err != nil {
		a.log.Error(err, "failed to generate static report")
		return err
	}
	a.log.Info("[TIMING] Static report generation complete", "duration_ms", time.Since(startStaticReport).Milliseconds())

	if err := a.getProviderLogs(ctx); err != nil {
		a.log.Error(err, "failed to get provider logs")
	}

	// Print results summary (only in progress mode, not in --no-progress mode)
	progressMode.Println("\nResults:")
	reportPath := filepath.Join(a.output, "static-report", "index.html")
	progressMode.Printf("  Report: file://%s\n", reportPath)
	analysisLogPath := filepath.Join(a.output, "analysis.log")
	progressMode.Printf("  Analysis logs: %s\n", analysisLogPath)

	a.log.Info("[TIMING] Hybrid analysis complete", "total_duration_ms", time.Since(startTotal).Milliseconds())
	a.log.Info("hybrid analysis completed successfully")
	return nil
}
