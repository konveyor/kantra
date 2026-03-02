package analyze

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/konveyor-ecosystem/kantra/cmd/internal/settings"
	"github.com/konveyor-ecosystem/kantra/pkg/container"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"golang.org/x/exp/maps"
)

func (a *analyzeCommand) getDepsFolders() (map[string]string, []string) {
	vols := map[string]string{}
	dependencyFolders := []string{}
	if len(a.depFolders) != 0 {
		for i := range a.depFolders {
			newDepPath := path.Join(util.InputPath, fmt.Sprintf("deps%v", i))
			vols[a.depFolders[i]] = newDepPath
			dependencyFolders = append(dependencyFolders, newDepPath)
		}
		return vols, dependencyFolders
	}

	return vols, dependencyFolders
}

func (a *analyzeCommand) RunProvidersHostNetwork(ctx context.Context, volName string, retry int, containerLogWriter io.Writer) error {
	volumes := map[string]string{
		volName: util.SourceMountPath,
	}
	a.providerContainerNames = map[string]string{}

	if a.mavenSettingsFile != "" {
		configVols, err := a.getConfigVolumes()
		if err != nil {
			a.log.V(1).Error(err, "failed to get config volumes for analysis")
			return err
		}
		maps.Copy(volumes, configVols)
	}

	vols, _ := a.getDepsFolders()
	if len(vols) != 0 {
		maps.Copy(volumes, vols)
	}

	// Only create Maven cache volume when Java provider is active.
	// The maven-cache-volume maps to the host's ~/.m2/repository and persists
	// across analysis runs to avoid re-downloading dependencies. This significantly
	// improves performance for subsequent analyses. If volume creation fails or
	// caching is disabled (KANTRA_SKIP_MAVEN_CACHE=true), we continue without
	// caching (graceful degradation).
	if _, hasJava := a.providersMap[util.JavaProvider]; hasJava {
		mavenCacheVolName, err := a.createMavenCacheVolume()
		if err != nil {
			a.log.V(1).Error(err, "failed to create maven cache volume, continuing without cache")
		} else if mavenCacheVolName != "" {
			mavenCacheDir := path.Join(util.M2Dir, "repository")
			volumes[mavenCacheVolName] = mavenCacheDir
			a.log.V(1).Info("mounted maven cache volume", "container_path", mavenCacheDir)
		}
	}

	for prov, init := range a.providersMap {
		args := []string{fmt.Sprintf("--port=%v", init.port)}
		if a.logLevel != nil && init.provider.SupportsLogLevel() {
			args = append(args, fmt.Sprintf("--log-level=%v", *a.logLevel))
		}

		// Publish port so it's accessible on macOS host (podman runs in VM)
		portMapping := fmt.Sprintf("%d:%d", init.port, init.port)

		a.log.Info("starting provider with port publishing", "provider", prov, "port", init.port)
		con := container.NewContainer()
		err := con.Run(
			ctx,
			container.WithImage(init.image),
			container.WithLog(a.log.V(1)),
			container.WithVolumes(volumes),
			container.WithContainerToolBin(settings.Settings.ContainerBinary),
			container.WithEntrypointArgs(args...),
			container.WithPortPublish(portMapping),
			container.WithDetachedMode(true),
			container.WithCleanup(false),
			container.WithName(fmt.Sprintf("provider-%v", container.RandomName())),
			container.WithProxy(a.httpProxy, a.httpsProxy, a.noProxy),
			container.WithStdout(containerLogWriter),
			container.WithStderr(containerLogWriter),
		)
		if err != nil {
			return fmt.Errorf("failed to start provider %s: %w", prov, err)
		}

		a.providerContainerNames[prov] = con.Name
		a.log.V(1).Info("provider started", "provider", prov, "container", con.Name)
	}

	return nil
}

// setupCommandOutput configures stdout and stderr for a command based on verbosity.
// At V(2) or higher, command output is captured and logged for debugging.
// Otherwise, output is discarded to keep the console clean.
// Returns buffers that can be passed to logCommandOutput after running the command.
func (c *AnalyzeCommandContext) setupCommandOutput(cmd *exec.Cmd) (stdout, stderr *bytes.Buffer) {
	if c.log.V(2).Enabled() {
		// Capture output for verbose logging
		stdout = &bytes.Buffer{}
		stderr = &bytes.Buffer{}
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		return stdout, stderr
	}
	// Discard output to keep console clean
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return nil, nil
}

// logCommandOutput logs the captured command output if verbosity is enabled.
// Call this after running a command that was set up with setupCommandOutput.
func (c *AnalyzeCommandContext) logCommandOutput(cmdName string, stdout, stderr *bytes.Buffer) {
	if stdout != nil && stdout.Len() > 0 {
		c.log.V(2).Info("command stdout", "command", cmdName, "output", stdout.String())
	}
	if stderr != nil && stderr.Len() > 0 {
		c.log.V(2).Info("command stderr", "command", cmdName, "output", stderr.String())
	}
}

func (c *AnalyzeCommandContext) createContainerNetwork() (string, error) {
	networkName := fmt.Sprintf("network-%v", container.RandomName())
	args := []string{
		"network",
		"create",
		networkName,
	}

	cmd := exec.Command(settings.Settings.ContainerBinary, args...)
	stdout, stderr := c.setupCommandOutput(cmd)
	err := cmd.Run()
	c.logCommandOutput("network create", stdout, stderr)
	if err != nil {
		return "", err
	}
	c.log.V(1).Info("created container network", "network", networkName)
	// for cleanup
	c.networkName = networkName
	return networkName, nil
}

// TODO: create for each source input once accepting multiple apps is completed
func (c *AnalyzeCommandContext) createContainerVolume(inputPath string) (string, error) {
	volName := fmt.Sprintf("volume-%v", container.RandomName())
	input, err := filepath.Abs(inputPath)
	if err != nil {
		return "", err
	}

	if c.isFileInput {
		//create temp dir and move bin file to mount
		file := filepath.Base(input)
		tempDir, err := os.MkdirTemp("", "java-bin-")
		if err != nil {
			c.log.V(1).Error(err, "failed creating temp dir", "dir", tempDir)
			return "", err
		}
		c.log.V(1).Info("created temp directory for Java input file", "dir", tempDir)
		// for cleanup
		c.tempDirs = append(c.tempDirs, tempDir)

		err = util.CopyFileContents(input, filepath.Join(tempDir, file))
		if err != nil {
			c.log.V(1).Error(err, "failed copying binary file")
			return "", err
		}
		input = tempDir
	}
	if runtime.GOOS == "windows" {
		// TODO(djzager): Thank ChatGPT
		// Extract the volume name (e.g., "C:")
		// Remove the volume name from the path to get the remaining part
		// Convert backslashes to forward slashes
		// Remove the colon from the volume name and convert to lowercase
		volumeName := filepath.VolumeName(input)
		remainingPath := input[len(volumeName):]
		remainingPath = filepath.ToSlash(remainingPath)
		driveLetter := strings.ToLower(strings.TrimSuffix(volumeName, ":"))

		// Construct the Linux-style path
		input = fmt.Sprintf("/mnt/%s%s", driveLetter, remainingPath)
	}

	args := []string{
		"volume",
		"create",
		"--opt",
		"type=none",
		"--opt",
		fmt.Sprintf("device=%v", input),
		"--opt",
		"o=bind",
		volName,
	}
	cmd := exec.Command(settings.Settings.ContainerBinary, args...)
	stdout, stderr := c.setupCommandOutput(cmd)
	err = cmd.Run()
	c.logCommandOutput("volume create", stdout, stderr)
	if err != nil {
		return "", err
	}
	c.log.V(1).Info("created container volume", "volume", volName)
	// for cleanup
	c.volumeName = volName
	return volName, nil
}

// createMavenCacheVolume creates or reuses a persistent volume for Maven dependency caching.
//
// This function creates a container volume named "maven-cache-volume" that maps to the host's
// ~/.m2/repository directory. The volume persists across analysis runs to avoid re-downloading
// Maven dependencies, significantly improving performance for subsequent analyses.
//
// Behavior:
//   - If the volume already exists, it is reused (no error)
//   - Creates ~/.m2/repository on the host if it doesn't exist
//   - On Windows, converts paths to Linux-style format for container compatibility
//   - Volume is NOT removed during cleanup (intentional for caching)
//   - Can be disabled by setting KANTRA_SKIP_MAVEN_CACHE=true (for CI/testing)
//
// Returns:
//   - Volume name on success ("maven-cache-volume")
//   - Empty string if caching is disabled via environment variable
//   - Error if volume creation or directory creation fails
//
// The volume can be manually removed with: podman volume rm maven-cache-volume
func (c *AnalyzeCommandContext) createMavenCacheVolume() (string, error) {
	// Allow skipping Maven cache for CI/testing environments where deterministic
	// dependency output is required. When disabled, dependencies are downloaded
	// to a temporary directory that's cleaned up after analysis.
	if os.Getenv("KANTRA_SKIP_MAVEN_CACHE") == "true" {
		c.log.V(1).Info("Maven cache disabled via KANTRA_SKIP_MAVEN_CACHE environment variable")
		return "", nil
	}

	volName := "maven-cache-volume"

	// Prepare volume creation parameters upfront to avoid race conditions
	// Use host's ~/.m2/repository for persistent caching across runs
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	m2RepoPath := filepath.Join(homeDir, ".m2", "repository")

	// Create directory if it doesn't exist
	if err := os.MkdirAll(m2RepoPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create maven cache directory: %w", err)
	}

	// Convert to container-compatible path format (same logic as createContainerVolume)
	mountPath := m2RepoPath
	if runtime.GOOS == "windows" {
		volumeName := filepath.VolumeName(mountPath)
		remainingPath := mountPath[len(volumeName):]
		remainingPath = filepath.ToSlash(remainingPath)
		driveLetter := strings.ToLower(strings.TrimSuffix(volumeName, ":"))
		mountPath = fmt.Sprintf("/mnt/%s%s", driveLetter, remainingPath)
	}

	// Try to create the volume (idempotent operation)
	// This handles race conditions where multiple kantra processes start simultaneously
	args := []string{
		"volume",
		"create",
		"--opt", "type=none",
		"--opt", fmt.Sprintf("device=%v", mountPath),
		"--opt", "o=bind",
		volName,
	}

	cmd := exec.Command(settings.Settings.ContainerBinary, args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		// Check if volume already exists (race condition with concurrent creation)
		// Both podman and docker return similar error messages for existing volumes
		errMsg := string(output)
		if strings.Contains(errMsg, "already exists") || strings.Contains(errMsg, "volume name") {
			// Volume exists, verify it and reuse it
			checkCmd := exec.Command(settings.Settings.ContainerBinary, "volume", "inspect", volName)
			if checkErr := checkCmd.Run(); checkErr == nil {
				c.log.V(1).Info("reusing existing maven cache volume (created by concurrent process)", "volume", volName)
				c.mavenCacheVolumeName = volName
				return volName, nil
			}
		}
		// Different error, fail
		return "", fmt.Errorf("failed to create maven cache volume: %w\nOutput: %s", err, string(output))
	}

	c.log.V(1).Info("created maven cache volume",
		"volume", volName,
		"host_path", m2RepoPath)

	// Track for cleanup (though we won't delete it - see RmVolumes)
	c.mavenCacheVolumeName = volName

	return volName, nil
}
