package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	provider2 "github.com/konveyor-ecosystem/kantra/pkg/provider"
	"github.com/konveyor-ecosystem/kantra/pkg/util"

	"github.com/devfile/alizer/pkg/apis/model"
	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/container"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/phayes/freeport"
	"gopkg.in/yaml.v2"
)

type AnalyzeCommandContext struct {
	providersMap map[string]ProviderInit

	// tempDirs list of temporary dirs created, used for cleanup
	tempDirs []string
	log      logr.Logger
	// isFileInput is set when input points to a file and not a dir
	isFileInput  bool
	needsBuiltin bool
	// used for cleanup
	networkName string
	volumeName  string
	// mavenCacheVolumeName tracks the persistent Maven cache volume (NOT removed during cleanup)
	mavenCacheVolumeName   string
	providerContainerNames []string

	// for containerless cmd
	reqMap    map[string]string
	kantraDir string
}

func (c *AnalyzeCommandContext) setProviders(providers []string, languages []model.Language, foundProviders []string) ([]string, error) {
	if len(providers) > 0 {
		for _, p := range providers {
			foundProviders = append(foundProviders, p)
		}
		return foundProviders, nil
	}
	for _, l := range languages {
		if l.CanBeComponent {
			c.log.V(5).Info("Got language", "component language", l)
			if l.Name == "C#" {
				foundProviders = append(foundProviders, util.CsharpProvider)
				continue
			}

			// typescript ls supports both TS and JS
			if l.Name == "JavaScript" || l.Name == "TypeScript" {
				foundProviders = append(foundProviders, util.NodeJSProvider)

			} else {
				foundProviders = append(foundProviders, strings.ToLower(l.Name))
			}
		}
	}
	return foundProviders, nil
}

func (c *AnalyzeCommandContext) setProviderInitInfo(foundProviders []string) error {
	for _, prov := range foundProviders {
		port, err := freeport.GetFreePort()
		if err != nil {
			return err
		}

		switch prov {
		case util.JavaProvider:
			c.providersMap[util.JavaProvider] = ProviderInit{
				port:     port,
				image:    Settings.JavaProviderImage,
				provider: &provider2.JavaProvider{},
			}
		case util.GoProvider:
			c.providersMap[util.GoProvider] = ProviderInit{
				port:     port,
				image:    Settings.GenericProviderImage,
				provider: &provider2.GoProvider{},
			}
		case util.PythonProvider:
			c.providersMap[util.PythonProvider] = ProviderInit{
				port:     port,
				image:    Settings.GenericProviderImage,
				provider: &provider2.PythonProvider{},
			}
		case util.NodeJSProvider:
			c.providersMap[util.NodeJSProvider] = ProviderInit{
				port:     port,
				image:    Settings.GenericProviderImage,
				provider: &provider2.NodeJsProvider{},
			}
		case util.CsharpProvider:
			c.providersMap[util.CsharpProvider] = ProviderInit{
				port:     port,
				image:    Settings.CsharpProviderImage,
				provider: &provider2.CsharpProvider{},
			}
		}
	}
	return nil
}

func (c *AnalyzeCommandContext) handleDir(p string, tempDir string, basePath string) error {
	newDir, err := filepath.Rel(basePath, p)
	if err != nil {
		return err
	}
	tempDir = filepath.Join(tempDir, newDir)
	c.log.Info("creating nested tmp dir", "tempDir", tempDir, "newDir", newDir)
	err = os.Mkdir(tempDir, 0777)
	if err != nil {
		return err
	}
	c.log.V(5).Info("create temp rule set for dir", "dir", tempDir)
	err = c.createTempRuleSet(tempDir, filepath.Base(p))
	if err != nil {
		c.log.V(1).Error(err, "failed to create temp ruleset", "path", tempDir)
		return err
	}
	return err
}

func (c *AnalyzeCommandContext) createTempRuleSet(path string, name string) error {
	c.log.Info("creating temp ruleset ", "path", path, "name", name)
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	tempRuleSet := engine.RuleSet{
		Name:        name,
		Description: "temp ruleset",
	}
	yamlData, err := yaml.Marshal(&tempRuleSet)
	if err != nil {
		return err
	}
	err = os.WriteFile(filepath.Join(path, "ruleset.yaml"), yamlData, os.ModePerm)
	if err != nil {
		return err
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

	cmd := exec.Command(Settings.ContainerBinary, args...)
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
	cmd := exec.Command(Settings.ContainerBinary, args...)
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

	cmd := exec.Command(Settings.ContainerBinary, args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		// Check if volume already exists (race condition with concurrent creation)
		// Both podman and docker return similar error messages for existing volumes
		errMsg := string(output)
		if strings.Contains(errMsg, "already exists") || strings.Contains(errMsg, "volume name") {
			// Volume exists, verify it and reuse it
			checkCmd := exec.Command(Settings.ContainerBinary, "volume", "inspect", volName)
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
