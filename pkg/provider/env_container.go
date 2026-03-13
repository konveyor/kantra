package provider

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/container"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	analyzerprovider "github.com/konveyor/analyzer-lsp/provider"
	"github.com/phayes/freeport"
)

// providerInstance holds runtime state for a single provider container.
type providerInstance struct {
	name          string
	port          int
	image         string
	containerName string
	provider      Provider // for SupportsLogLevel
}

// containerEnvironment runs providers in containers with port publishing.
// The analyzer runs in-process on the host and connects via gRPC.
type containerEnvironment struct {
	cfg EnvironmentConfig
	log logr.Logger

	// State populated during Start
	providers      []providerInstance
	volumeName     string
	mavenCacheVol  string
	tempDirs       []string
	rulesetsDir    string
	containerNames []string
	addresses      map[string]string // provider name -> "localhost:PORT"
	configs        []analyzerprovider.Config
}

func newContainerEnvironment(cfg EnvironmentConfig) *containerEnvironment {
	log := cfg.Log
	if log.GetSink() == nil {
		log = logr.Discard()
	}
	return &containerEnvironment{
		cfg:       cfg,
		log:       log,
		addresses: make(map[string]string),
	}
}

// Start creates volumes, starts provider containers, and waits for health checks.
func (e *containerEnvironment) Start(ctx context.Context) error {
	// Resolve provider info: allocate ports and look up Provider implementations
	if err := e.resolveProviders(); err != nil {
		return err
	}

	// Create analysis log writer (discard -- callers manage their own log files)
	logWriter := io.Discard

	// Run parallel startup tasks: volume creation + ruleset extraction
	if err := e.parallelStartup(ctx, logWriter); err != nil {
		return err
	}

	// Start provider containers
	if err := e.startProviders(ctx, logWriter); err != nil {
		return err
	}

	// Wait for all providers to become ready
	if err := e.waitForProviders(ctx); err != nil {
		return err
	}

	// Build provider addresses map
	for i := range e.providers {
		p := &e.providers[i]
		e.addresses[p.name] = fmt.Sprintf("localhost:%d", p.port)
	}

	// Generate provider configs
	providerNames := make([]string, 0, len(e.addresses)+1)
	for name := range e.addresses {
		providerNames = append(providerNames, name)
	}
	providerNames = append(providerNames, "builtin")

	// Maven settings path: use container path since providers run in containers
	mavenSettingsPath := ""
	if e.cfg.MavenSettingsFile != "" {
		mavenSettingsPath = path.Join(util.ConfigMountPath, "settings.xml")
	}

	// For file inputs (e.g., .war/.jar), the Location must include the
	// filename so the Java provider knows it's analyzing a binary file.
	// The volume mounts the parent directory at SourceMountPath, so the
	// file appears at SourceMountPath/filename inside the container.
	containerLocation := util.SourceMountPath
	if e.cfg.IsFileInput {
		containerLocation = path.Join(util.SourceMountPath, filepath.Base(e.cfg.Input))
	}

	e.configs = DefaultProviderConfig(ModeNetwork, DefaultOptions{
		Providers:         providerNames,
		Location:          containerLocation,
		LocalLocation:     e.cfg.Input,
		AnalysisMode:      e.cfg.AnalysisMode,
		ProviderAddresses: e.addresses,
		ContextLines:      e.cfg.ContextLines,
		HTTPProxy:         e.cfg.HTTPProxy,
		HTTPSProxy:        e.cfg.HTTPSProxy,
		NoProxy:           e.cfg.NoProxy,
		MavenSettingsFile: mavenSettingsPath,
		JvmMaxMem:         e.cfg.JvmMaxMem,
	})

	e.log.Info("all providers are ready")
	return nil
}

// Stop stops and removes provider containers, volumes, and temp dirs.
func (e *containerEnvironment) Stop(ctx context.Context) error {
	if !e.cfg.Cleanup {
		return nil
	}

	// Remove temp dirs
	for _, dir := range e.tempDirs {
		if err := os.RemoveAll(dir); err != nil {
			e.log.V(1).Error(err, "failed to delete temporary dir", "dir", dir)
		}
	}

	// Stop and remove provider containers
	for _, name := range e.containerNames {
		stopCmd := exec.CommandContext(ctx, e.cfg.ContainerBinary, "stop", name)
		e.log.V(1).Info("stopping provider container", "container", name)
		if err := stopCmd.Run(); err != nil {
			e.log.V(1).Error(err, "failed to stop container", "container", name)
			continue
		}
		rmCmd := exec.CommandContext(ctx, e.cfg.ContainerBinary, "rm", name)
		e.log.V(1).Info("removing provider container", "container", name)
		if err := rmCmd.Run(); err != nil {
			e.log.V(1).Error(err, "failed to remove container", "container", name)
		}
	}

	// Remove source volume (but NOT maven cache volume)
	if e.volumeName != "" {
		cmd := exec.CommandContext(ctx, e.cfg.ContainerBinary, "volume", "rm", e.volumeName)
		e.log.V(1).Info("removing created volume", "volume", e.volumeName)
		if err := cmd.Run(); err != nil {
			e.log.V(1).Error(err, "failed to remove volume", "volume", e.volumeName)
		}
	}

	return nil
}

// ProviderConfigs returns ModeNetwork provider configurations.
// Must be called after Start.
func (e *containerEnvironment) ProviderConfigs() []analyzerprovider.Config {
	return e.configs
}

// Rules returns rule file/directory paths. For container mode, default
// rulesets are extracted from the runner container image.
func (e *containerEnvironment) Rules(userRules []string, enableDefaults bool) ([]string, error) {
	rules := make([]string, len(userRules))
	copy(rules, userRules)
	if enableDefaults && e.rulesetsDir != "" {
		rules = append(rules, e.rulesetsDir)
	}
	return rules, nil
}

// ExtraOptions returns path mappings for binary analysis or
// ignore-additional-builtin-configs for source analysis.
func (e *containerEnvironment) ExtraOptions(ctx context.Context, isBinaryAnalysis bool) ExtraEnvironmentOptions {
	if isBinaryAnalysis {
		containerRoot := util.SourceMountPath
		hostRoot := filepath.Dir(e.cfg.Input)
		if e.volumeName != "" {
			hostRoot = ResolveVolumeHostPath(
				ctx, e.log, e.cfg.ContainerBinary, e.volumeName, hostRoot)
		}
		mappings := BuildPathMappings(containerRoot, hostRoot)
		return ExtraEnvironmentOptions{PathMappings: mappings}
	}
	return ExtraEnvironmentOptions{IgnoreAdditionalBuiltinConfigs: true}
}

// PostAnalysis collects provider container logs.
func (e *containerEnvironment) PostAnalysis(ctx context.Context) error {
	if len(e.containerNames) == 0 {
		return nil
	}
	if e.cfg.OutputDir == "" {
		return nil
	}
	providerLogFilePath := filepath.Join(e.cfg.OutputDir, "provider.log")
	providerLog, err := os.Create(providerLogFilePath)
	if err != nil {
		return fmt.Errorf("failed creating provider log file at %s", providerLogFilePath)
	}
	defer providerLog.Close()

	for _, conName := range e.containerNames {
		e.log.V(1).Info("getting provider container logs", "container", conName)
		cmd := exec.CommandContext(ctx, e.cfg.ContainerBinary, "logs", conName)
		cmd.Stdout = providerLog
		cmd.Stderr = providerLog
		if err := cmd.Run(); err != nil {
			e.log.V(1).Error(err, "failed to get provider container logs", "container", conName)
		}
	}
	return nil
}

// --- Internal methods ---

// resolveProviders allocates ports and looks up Provider implementations
// from the registry for each configured provider.
func (e *containerEnvironment) resolveProviders() error {
	for _, info := range e.cfg.Providers {
		port, err := freeport.GetFreePort()
		if err != nil {
			return fmt.Errorf("failed to allocate port for provider %s: %w", info.Name, err)
		}

		// Look up the Provider implementation for SupportsLogLevel
		p, ok := providerRegistry[info.Name]
		if !ok {
			return fmt.Errorf("unknown provider: %s", info.Name)
		}

		e.providers = append(e.providers, providerInstance{
			name:     info.Name,
			port:     port,
			image:    info.Image,
			provider: p,
		})
	}
	return nil
}

// parallelStartup runs volume creation and ruleset extraction concurrently.
func (e *containerEnvironment) parallelStartup(ctx context.Context, logWriter io.Writer) error {
	type result struct {
		name string
		err  error
	}

	var wg sync.WaitGroup
	resultChan := make(chan result, 2)

	// Task 1: Create container volume
	wg.Add(1)
	go func() {
		defer wg.Done()
		vol, err := e.createVolume(ctx, e.cfg.Input)
		if err == nil {
			e.volumeName = vol
		}
		resultChan <- result{name: "volume creation", err: err}
	}()

	// Task 2: Extract default rulesets (if enabled)
	if e.cfg.EnableDefaultRulesets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dir, err := e.extractDefaultRulesets(ctx, logWriter)
			if err == nil {
				e.rulesetsDir = dir
			}
			resultChan <- result{name: "ruleset extraction", err: err}
		}()
	}

	wg.Wait()
	close(resultChan)

	for r := range resultChan {
		if r.err != nil {
			// Cleanup any successfully created volume
			if e.volumeName != "" {
				cmd := exec.CommandContext(ctx, e.cfg.ContainerBinary, "volume", "rm", e.volumeName)
				if cleanupErr := cmd.Run(); cleanupErr != nil {
					e.log.Error(cleanupErr, "failed to cleanup volume after startup failure")
				}
			}
			return fmt.Errorf("%s failed: %w", r.name, r.err)
		}
	}
	return nil
}

// createVolume creates a container volume from the input path.
func (e *containerEnvironment) createVolume(ctx context.Context, inputPath string) (string, error) {
	volName := fmt.Sprintf("volume-%v", container.RandomName())
	input, err := filepath.Abs(inputPath)
	if err != nil {
		return "", err
	}

	if e.cfg.IsFileInput {
		// Create temp dir and copy binary file for mounting
		file := filepath.Base(input)
		tempDir, err := os.MkdirTemp("", "java-bin-")
		if err != nil {
			return "", err
		}
		e.tempDirs = append(e.tempDirs, tempDir)

		if err := util.CopyFileContents(input, filepath.Join(tempDir, file)); err != nil {
			return "", err
		}
		input = tempDir
	}

	if runtime.GOOS == "windows" {
		volumeName := filepath.VolumeName(input)
		remainingPath := input[len(volumeName):]
		remainingPath = filepath.ToSlash(remainingPath)
		driveLetter := strings.ToLower(strings.TrimSuffix(volumeName, ":"))
		input = fmt.Sprintf("/mnt/%s%s", driveLetter, remainingPath)
	}

	args := []string{
		"volume", "create",
		"--opt", "type=none",
		"--opt", fmt.Sprintf("device=%v", input),
		"--opt", "o=bind",
		volName,
	}
	cmd := exec.Command(e.cfg.ContainerBinary, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return "", err
	}
	e.log.V(1).Info("created container volume", "volume", volName)
	return volName, nil
}

// createMavenCacheVolume creates or reuses a persistent volume for Maven
// dependency caching. Returns empty string if caching is disabled.
func (e *containerEnvironment) createMavenCacheVolume() (string, error) {
	if os.Getenv("KANTRA_SKIP_MAVEN_CACHE") == "true" {
		e.log.V(1).Info("Maven cache disabled via KANTRA_SKIP_MAVEN_CACHE")
		return "", nil
	}

	volName := "maven-cache-volume"
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	m2RepoPath := filepath.Join(homeDir, ".m2", "repository")
	if err := os.MkdirAll(m2RepoPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create maven cache directory: %w", err)
	}

	mountPath := m2RepoPath
	if runtime.GOOS == "windows" {
		volumeName := filepath.VolumeName(mountPath)
		remainingPath := mountPath[len(volumeName):]
		remainingPath = filepath.ToSlash(remainingPath)
		driveLetter := strings.ToLower(strings.TrimSuffix(volumeName, ":"))
		mountPath = fmt.Sprintf("/mnt/%s%s", driveLetter, remainingPath)
	}

	args := []string{
		"volume", "create",
		"--opt", "type=none",
		"--opt", fmt.Sprintf("device=%v", mountPath),
		"--opt", "o=bind",
		volName,
	}
	cmd := exec.Command(e.cfg.ContainerBinary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := string(output)
		if strings.Contains(errMsg, "already exists") || strings.Contains(errMsg, "volume name") {
			checkCmd := exec.Command(e.cfg.ContainerBinary, "volume", "inspect", volName)
			if checkErr := checkCmd.Run(); checkErr == nil {
				e.log.V(1).Info("reusing existing maven cache volume", "volume", volName)
				e.mavenCacheVol = volName
				return volName, nil
			}
		}
		return "", fmt.Errorf("failed to create maven cache volume: %w\nOutput: %s", err, string(output))
	}

	e.log.V(1).Info("created maven cache volume", "volume", volName, "host_path", m2RepoPath)
	e.mavenCacheVol = volName
	return volName, nil
}

// startProviders starts each provider container with port publishing.
func (e *containerEnvironment) startProviders(ctx context.Context, logWriter io.Writer) error {
	volumes := map[string]string{
		e.volumeName: util.SourceMountPath,
	}

	// Mount maven settings if specified
	if e.cfg.MavenSettingsFile != "" {
		tempDir, err := os.MkdirTemp("", "analyze-config-")
		if err != nil {
			return fmt.Errorf("failed creating temp dir for config: %w", err)
		}
		e.tempDirs = append(e.tempDirs, tempDir)
		volumes[tempDir] = util.ConfigMountPath
		volumes[e.cfg.MavenSettingsFile] = fmt.Sprintf("%s/%s", util.ConfigMountPath, "settings.xml")
	}

	// Mount dependency folders
	for i, depFolder := range e.cfg.DepFolders {
		newDepPath := path.Join(util.InputPath, fmt.Sprintf("deps%v", i))
		volumes[depFolder] = newDepPath
	}

	// Create Maven cache volume for Java provider
	hasJava := false
	for _, p := range e.providers {
		if p.name == util.JavaProvider {
			hasJava = true
			break
		}
	}
	if hasJava {
		mavenCacheVolName, err := e.createMavenCacheVolume()
		if err != nil {
			e.log.V(1).Error(err, "failed to create maven cache volume, continuing without cache")
		} else if mavenCacheVolName != "" {
			mavenCacheDir := path.Join(util.M2Dir, "repository")
			volumes[mavenCacheVolName] = mavenCacheDir
			e.log.V(1).Info("mounted maven cache volume", "container_path", mavenCacheDir)
		}
	}

	for i := range e.providers {
		p := &e.providers[i]
		args := []string{fmt.Sprintf("--port=%v", p.port)}
		if e.cfg.LogLevel != nil && p.provider.SupportsLogLevel() {
			args = append(args, fmt.Sprintf("--log-level=%v", *e.cfg.LogLevel))
		}

		portMapping := fmt.Sprintf("%d:%d", p.port, p.port)

		e.log.Info("starting provider with port publishing", "provider", p.name, "port", p.port)
		con := container.NewContainer()
		err := con.Run(
			ctx,
			container.WithImage(p.image),
			container.WithLog(e.log.V(1)),
			container.WithVolumes(volumes),
			container.WithContainerToolBin(e.cfg.ContainerBinary),
			container.WithEntrypointArgs(args...),
			container.WithPortPublish(portMapping),
			container.WithDetachedMode(true),
			container.WithCleanup(false),
			container.WithName(fmt.Sprintf("provider-%v", container.RandomName())),
			container.WithProxy(e.cfg.HTTPProxy, e.cfg.HTTPSProxy, e.cfg.NoProxy),
			container.WithStdout(logWriter),
			container.WithStderr(logWriter),
		)
		if err != nil {
			return fmt.Errorf("failed to start provider %s: %w", p.name, err)
		}

		p.containerName = con.Name
		e.containerNames = append(e.containerNames, con.Name)
		e.log.V(1).Info("provider started", "provider", p.name, "container", con.Name)
	}
	return nil
}

// waitForProviders waits for all provider containers to become ready.
func (e *containerEnvironment) waitForProviders(ctx context.Context) error {
	timeout := 30 * time.Second
	if e.cfg.HealthCheckTimeout > 0 {
		timeout = time.Duration(e.cfg.HealthCheckTimeout) * time.Second
	}

	type healthResult struct {
		providerName string
		err          error
	}
	healthChan := make(chan healthResult, len(e.providers))

	for i := range e.providers {
		p := &e.providers[i]
		go func() {
			err := waitForProvider(ctx, p.name, p.port, timeout, e.log)
			healthChan <- healthResult{providerName: p.name, err: err}
		}()
	}

	for range e.providers {
		result := <-healthChan
		if result.err != nil {
			return fmt.Errorf("provider %s health check failed: %w", result.providerName, result.err)
		}
	}
	return nil
}

// waitForProvider polls a provider's port until it's ready or timeout is reached.
func waitForProvider(ctx context.Context, providerName string, port int, timeout time.Duration, log logr.Logger) error {
	deadline := time.Now().Add(timeout)
	address := fmt.Sprintf("localhost:%d", port)
	backoff := 50 * time.Millisecond
	maxBackoff := 500 * time.Millisecond

	log.V(1).Info("waiting for provider to become ready", "provider", providerName, "address", address, "timeout", timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("provider health check cancelled: %w", ctx.Err())
		default:
		}

		conn, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			log.V(1).Info("provider is ready", "provider", providerName, "address", address)
			return nil
		}

		log.V(2).Info("provider not ready yet, retrying", "provider", providerName, "error", err, "backoff", backoff)

		select {
		case <-ctx.Done():
			return fmt.Errorf("provider health check cancelled: %w", ctx.Err())
		case <-time.After(backoff):
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

// extractDefaultRulesets extracts rulesets from the runner container image.
func (e *containerEnvironment) extractDefaultRulesets(ctx context.Context, logWriter io.Writer) (string, error) {
	if !e.cfg.EnableDefaultRulesets {
		return "", nil
	}
	if e.cfg.OutputDir == "" {
		return "", fmt.Errorf("output directory required for ruleset extraction")
	}

	version := e.cfg.Version
	if version == "" {
		version = "latest"
	}

	rulesetsDir := filepath.Join(e.cfg.OutputDir, fmt.Sprintf(".rulesets-%s", version))

	// Check if rulesets already extracted (cached from previous run)
	if _, err := os.Stat(rulesetsDir); os.IsNotExist(err) {
		e.log.Info("extracting default rulesets from container to host", "version", version, "dir", rulesetsDir)

		tempName := fmt.Sprintf("ruleset-extract-%v", container.RandomName())
		createCmd := exec.CommandContext(ctx, e.cfg.ContainerBinary,
			"create", "--name", tempName, e.cfg.RunnerImage)
		createCmd.Stdout = logWriter
		createCmd.Stderr = logWriter
		if err := createCmd.Run(); err != nil {
			return "", fmt.Errorf("failed to create temp container for ruleset extraction: %w", err)
		}

		defer func() {
			rmCmd := exec.CommandContext(ctx, e.cfg.ContainerBinary, "rm", tempName)
			rmCmd.Run()
		}()

		copyCmd := exec.CommandContext(ctx, e.cfg.ContainerBinary,
			"cp", fmt.Sprintf("%s:/opt/rulesets", tempName), rulesetsDir)
		copyCmd.Stdout = logWriter
		copyCmd.Stderr = logWriter
		if err := copyCmd.Run(); err != nil {
			return "", fmt.Errorf("failed to copy rulesets from container: %w", err)
		}

		e.log.Info("extracted default rulesets to host", "version", version, "dir", rulesetsDir)
	} else {
		e.log.V(1).Info("using cached default rulesets", "version", version, "dir", rulesetsDir)
	}

	return rulesetsDir, nil
}
