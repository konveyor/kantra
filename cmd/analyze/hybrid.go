package analyze

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/bombsimon/logrusr/v3"
	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/cmd/internal/settings"
	kantraprovider "github.com/konveyor-ecosystem/kantra/pkg/provider"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	konveyorAnalyzer "github.com/konveyor/analyzer-lsp/core"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/tracing"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

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
				cmd := exec.CommandContext(ctx, settings.Settings.ContainerBinary, "volume", "rm", volName)
				if cleanupErr := cmd.Run(); cleanupErr != nil {
					a.log.Error(cleanupErr, "failed to cleanup volume after startup failure")
				}
			}
			return "", "", fmt.Errorf("%s failed: %w", result.name, result.err)
		}
	}

	return volName, rulesetsDir, nil
}

// RunAnalysisHybridInProcess runs analysis in hybrid mode with the analyzer running in-process
// and providers running in containers. This provides clean output like containerless mode while
// maintaining the isolation benefits of containerized providers.
//
// Architecture:
//   - Providers: Run in containers with port publishing (localhost:PORT)
//   - Analyzer: Runs as in-process Go library via konveyor.NewAnalyzer()
//   - Communication: Provider configs specify Address for gRPC connections
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
	analysisLogFile, err := os.Create(analysisLogFilePath)
	if err != nil {
		return fmt.Errorf("failed creating analysis log file at %s", analysisLogFilePath)
	}
	defer analysisLogFile.Close()

	// Setup logging - analyzer logs to file, clean output to console
	logrusAnalyzerLog := logrus.New()
	logrusAnalyzerLog.SetOutput(analysisLogFile)
	logrusAnalyzerLog.SetFormatter(&logrus.TextFormatter{})
	if a.logLevel != nil {
		logrusAnalyzerLog.SetLevel(logrus.Level(*a.logLevel))
	}

	// Add console hook for rule processing messages
	// but only if progress is disabled (to avoid interfering with progress bar)
	if progressMode.ShouldAddConsoleHook() {
		consoleHook := &ConsoleHook{Level: logrus.InfoLevel, Log: a.log}
		logrusAnalyzerLog.AddHook(consoleHook)
	}

	analyzeLog := logrusr.New(logrusAnalyzerLog)

	// Load override provider settings if specified
	overrideConfigs, err := a.loadOverrideProviderSettings()
	if err != nil {
		return fmt.Errorf("failed to load override provider settings: %w", err)
	}
	if overrideConfigs != nil {
		a.log.Info("loaded override provider settings", "file", a.overrideProviderSettings, "providers", len(overrideConfigs))
	}

	// --- Container infrastructure: start provider containers ---
	providerAddresses := map[string]string{}

	if len(a.providersMap) > 0 {
		startProviderSetup := time.Now()
		a.log.Info("[TIMING] Starting provider container setup")

		// Run independent startup tasks in parallel for better performance
		volName, rulesetsDir, err := a.runParallelStartupTasks(ctx, analysisLogFile)
		if err != nil {
			return err
		}

		// Add extracted rulesets to rules list if we got any
		if rulesetsDir != "" {
			a.rules = append(a.rules, rulesetsDir)
		}

		progressMode.Printf("  ✓ Created volume\n")

		// Start providers with port publishing
		err = a.RunProvidersHostNetwork(ctx, volName, 5, analysisLogFile)
		if err != nil {
			return fmt.Errorf("failed to start providers: %w", err)
		}

		progressMode.Printf("  ✓ Started provider containers\n")

		// Wait for providers to become ready with health checks (in parallel)
		a.log.Info("waiting for providers to become ready...")

		type providerHealthResult struct {
			providerName string
			err          error
		}
		healthChan := make(chan providerHealthResult, len(a.providersMap))

		for provName, provInit := range a.providersMap {
			go func() {
				err := waitForProvider(ctx, provName, provInit.port, 30*time.Second, a.log)
				healthChan <- providerHealthResult{providerName: provName, err: err}
			}()
		}

		for i := 0; i < len(a.providersMap); i++ {
			result := <-healthChan
			if result.err != nil {
				return fmt.Errorf("provider %s health check failed: %w", result.providerName, result.err)
			}
		}

		// Build provider addresses map for config generation
		for provName, provInit := range a.providersMap {
			providerAddresses[provName] = fmt.Sprintf("localhost:%d", provInit.port)
		}

		a.log.Info("all providers are ready")
		a.log.Info("[TIMING] Provider container setup complete", "duration_ms", time.Since(startProviderSetup).Milliseconds())
	}

	// --- Generate provider configs using shared defaults ---

	providerNames := make([]string, 0, len(providerAddresses)+1)
	for name := range providerAddresses {
		providerNames = append(providerNames, name)
	}
	providerNames = append(providerNames, "builtin")

	// Maven settings path: use container path since the Java provider runs in a container
	mavenSettingsPath := ""
	if a.mavenSettingsFile != "" {
		mavenSettingsPath = path.Join(util.ConfigMountPath, "settings.xml")
	}

	providerConfigs := kantraprovider.DefaultProviderConfig(kantraprovider.ModeNetwork, kantraprovider.DefaultOptions{
		Providers:         providerNames,
		Location:          a.sourceLocationPath,
		LocalLocation:     a.input,
		AnalysisMode:      a.mode,
		ProviderAddresses: providerAddresses,
		ContextLines:      a.contextLines,
		HTTPProxy:         a.httpProxy,
		HTTPSProxy:        a.httpsProxy,
		NoProxy:           a.noProxy,
		MavenSettingsFile: mavenSettingsPath,
		JvmMaxMem:         settings.Settings.JvmMaxMem,
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

	// Apply override provider settings
	if overrideConfigs != nil {
		for i := range providerConfigs {
			providerConfigs[i] = applyProviderOverrides(providerConfigs[i], overrideConfigs)
		}
	}

	// --- Build label selectors ---
	depLabelSelector := ""
	if !a.analyzeKnownLibraries {
		depLabelSelector = fmt.Sprintf("!%v=open-source", provider.DepSourceLabel)
	}

	// --- Create progress reporter ---
	reporter, progressDone, progressCancel := setupProgressReporter(ctx, a.noProgress)
	if progressCancel != nil {
		defer progressCancel()
	}

	// --- Build analyzer options ---
	analyzerOpts := []konveyorAnalyzer.AnalyzerOption{
		konveyorAnalyzer.WithRuleFilepaths(a.rules),
		konveyorAnalyzer.WithLabelSelector(a.getLabelSelector()),
		konveyorAnalyzer.WithContextLinesLimit(a.contextLines),
		konveyorAnalyzer.WithLogger(analyzeLog),
		konveyorAnalyzer.WithContext(ctx),
		konveyorAnalyzer.WithReporters(reporter),
		konveyorAnalyzer.WithProviderConfigs(providerConfigs),
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
	if isBinaryAnalysis {
		// Binary analysis: providers run in containers and return paths relative
		// to the container filesystem. The builtin provider runs on the host and
		// needs host paths. Resolve the volume's host path and set up path mappings
		// so the analyzer translates container paths to host paths automatically.
		//
		// The volume maps hostRoot <-> /opt/input/source (SourceMountPath) inside
		// the container. The Java provider decompiles the WAR and creates files
		// under /opt/input/source/..., which appear at hostRoot/... on the host.
		// So the mapping is: /opt/input/source -> hostRoot.
		containerRoot := util.SourceMountPath
		hostRoot := filepath.Dir(a.input)
		if a.volumeName != "" {
			hostRoot = kantraprovider.ResolveVolumeHostPath(
				ctx, a.log, settings.Settings.ContainerBinary, a.volumeName, hostRoot)
		}
		mappings := kantraprovider.BuildPathMappings(containerRoot, hostRoot)
		analyzerOpts = append(analyzerOpts, konveyorAnalyzer.WithPathMappings(mappings))
	} else {
		analyzerOpts = append(analyzerOpts, konveyorAnalyzer.WithIgnoreAdditionalBuiltinConfigs(true))
	}
	startAnalyzer := time.Now()
	a.log.Info("[TIMING] Creating analyzer")
	anlzr, err := konveyorAnalyzer.NewAnalyzer(analyzerOpts...)
	if err != nil {
		a.log.Error(err, "failed to create analyzer")
		return fmt.Errorf("failed to create analyzer: %w", err)
	}
	defer anlzr.Stop()
	a.log.Info("[TIMING] Analyzer created", "duration_ms", time.Since(startAnalyzer).Milliseconds())

	progressMode.Printf("  ✓ Initialized providers\n")

	// Parse rules
	startRuleLoading := time.Now()
	a.log.Info("[TIMING] Starting rule loading")
	_, err = anlzr.ParseRules()
	if err != nil {
		a.log.Error(err, "failed to parse rules")
		return fmt.Errorf("failed to parse rules: %w", err)
	}
	a.log.Info("[TIMING] Rule loading complete", "duration_ms", time.Since(startRuleLoading).Milliseconds())

	// Start providers (ProviderInit + Prepare)
	startProviders := time.Now()
	a.log.Info("[TIMING] Starting provider init")
	err = anlzr.ProviderStart()
	if err != nil {
		a.log.Error(err, "failed to start providers")
		return fmt.Errorf("failed to start providers: %w", err)
	}
	a.log.Info("[TIMING] Provider init complete", "duration_ms", time.Since(startProviders).Milliseconds())

	progressMode.Printf("  ✓ Started rules engine\n")

	// Run analysis
	startRuleExecution := time.Now()
	a.log.Info("[TIMING] Starting rule execution")
	a.log.Info("evaluating rules for violations. see analysis.log for more info")
	rulesets := anlzr.Run()
	a.log.Info("[TIMING] Rule execution complete", "duration_ms", time.Since(startRuleExecution).Milliseconds())

	// Get dependencies
	a.log.Info("resolving dependencies")
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

	if err := a.getProviderLogs(ctx); err != nil {
		a.log.Error(err, "failed to get provider logs")
	}

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

	// Generate static report
	startStaticReport := time.Now()
	a.log.Info("[TIMING] Starting static report generation")

	err = a.GenerateStaticReport(ctx, a.log)
	if err != nil {
		a.log.Error(err, "failed to generate static report")
		return err
	}
	a.log.Info("[TIMING] Static report generation complete", "duration_ms", time.Since(startStaticReport).Milliseconds())

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
