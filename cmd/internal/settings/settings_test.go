package settings

import (
	"os"
	"os/exec"
	"testing"

	"github.com/konveyor-ecosystem/kantra/pkg/util"
)

// Test RUNNER_IMG settings
func TestRunnerImgDefault(t *testing.T) {
	os.Unsetenv("RUNNER_IMG") // Ensure empty variable
	s := &Config{}
	s.Load()
	if s.RunnerImage != "quay.io/konveyor/kantra:latest" {
		t.Errorf("Unexpected RUNNER_IMG default: %s", s.RunnerImage)
	}
}

func TestRunnerImgCustom(t *testing.T) {
	os.Setenv("RUNNER_IMG", "quay.io/some-contributor/my-kantra")
	s := &Config{}
	s.Load()
	if s.RunnerImage != "quay.io/some-contributor/my-kantra" {
		t.Errorf("Unexpected RUNNER_IMG: %s", s.RunnerImage)
	}
}

func TestConfig_loadDefaultPodmanBin(t *testing.T) {
	tests := []struct {
		name                string
		containerTool       string
		podmanBin           string
		expectContainerTool string
		expectError         bool
	}{
		{
			name:                "existing CONTAINER_TOOL is respected",
			containerTool:       "/usr/bin/podman",
			expectContainerTool: "/usr/bin/podman",
			expectError:         false,
		},
		{
			name:                "PODMAN_BIN is used when CONTAINER_TOOL is empty",
			containerTool:       "",
			podmanBin:           "/usr/local/bin/podman",
			expectContainerTool: "/usr/local/bin/podman",
			expectError:         false,
		},
		{
			name:          "fallback to podman in PATH",
			containerTool: "",
			podmanBin:     "",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up environment
			os.Unsetenv("CONTAINER_TOOL")
			os.Unsetenv("PODMAN_BIN")

			// Set up test environment
			if tt.containerTool != "" {
				os.Setenv("CONTAINER_TOOL", tt.containerTool)
			}
			if tt.podmanBin != "" {
				os.Setenv("PODMAN_BIN", tt.podmanBin)
			}

			c := &Config{}
			err := c.loadDefaultPodmanBin()

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if tt.expectContainerTool != "" && os.Getenv("CONTAINER_TOOL") != tt.expectContainerTool {
				t.Errorf("Expected CONTAINER_TOOL=%s, got %s", tt.expectContainerTool, os.Getenv("CONTAINER_TOOL"))
			}
		})
	}
}

func TestConfig_trySetDefaultPodmanBin(t *testing.T) {
	tests := []struct {
		name        string
		file        string
		expectError bool
	}{
		{
			name:        "try to find podman in PATH",
			file:        "podman",
			expectError: false,
		},
		{
			name:        "try to find docker in PATH",
			file:        "docker",
			expectError: false,
		},
		{
			name:        "file not found",
			file:        "nonexistent-tool",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{ContainerBinary: "/usr/bin/podman"}
			found, err := c.trySetDefaultPodmanBin(tt.file)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Check that the method behaves consistently with exec.LookPath
			_, lookErr := exec.LookPath(tt.file)
			if lookErr == nil {
				// If tool exists in PATH, we expect it to be found (unless path equals ContainerBinary)
				if tt.file != "nonexistent-tool" {
					t.Logf("Tool %s found in PATH, returned found=%t", tt.file, found)
				}
			} else {
				// If tool doesn't exist, we expect found=false
				if found {
					t.Errorf("Expected found=false for non-existent tool %s, got true", tt.file)
				}
			}
		})
	}
}

func TestConfig_loadRunnerImg(t *testing.T) {
	tests := []struct {
		name              string
		existingRunnerImg string
		version           string
		expectRunnerImg   string
		expectError       bool
	}{
		{
			name:              "existing RUNNER_IMG is respected",
			existingRunnerImg: "custom/image:tag",
			expectRunnerImg:   "custom/image:tag",
			expectError:       false,
		},
		{
			name:              "generates versioned image when RUNNER_IMG is empty",
			existingRunnerImg: "",
			version:           "v1.0.0",
			expectError:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up environment
			os.Unsetenv("RUNNER_IMG")

			// Set up test environment
			if tt.existingRunnerImg != "" {
				os.Setenv("RUNNER_IMG", tt.existingRunnerImg)
			}
			if tt.version != "" {
				originalVersion := Version
				Version = tt.version
				defer func() { Version = originalVersion }()
			}

			c := &Config{}
			err := c.loadRunnerImg()

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if tt.expectRunnerImg != "" && os.Getenv("RUNNER_IMG") != tt.expectRunnerImg {
				t.Errorf("Expected RUNNER_IMG=%s, got %s", tt.expectRunnerImg, os.Getenv("RUNNER_IMG"))
			}
		})
	}
}

func TestConfigDirBasename(t *testing.T) {
	orig := ConfigDirName
	defer func() { ConfigDirName = orig }()

	tests := []struct {
		dirName string
		want    string
	}{
		{dirName: "", want: ".kantra"},
		{dirName: "kantra", want: ".kantra"},
		{dirName: "mytool", want: ".mytool"},
		{dirName: ".custom", want: ".custom"},
	}
	for _, tt := range tests {
		t.Run(tt.dirName, func(t *testing.T) {
			ConfigDirName = tt.dirName
			if got := ConfigDirBasename(); got != tt.want {
				t.Errorf("ConfigDirBasename() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfig_loadConfigDir(t *testing.T) {
	orig := ConfigDirName
	defer func() { ConfigDirName = orig }()

	ConfigDirName = "mytool"
	c := &Config{}
	if err := c.loadConfigDir(); err != nil {
		t.Fatalf("loadConfigDir() error = %v", err)
	}
	if got := util.ConfigDirBasename(); got != ".mytool" {
		t.Errorf("util.ConfigDirBasename() = %q, want %q", got, ".mytool")
	}
}

func TestConfig_loadCommandName(t *testing.T) {
	// Note: This function depends on RootCommandName which is a package-level var
	c := &Config{}
	err := c.loadCommandName()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestConfig_loadProviders(t *testing.T) {
	tests := []struct {
		name                   string
		existingJavaProvider   string
		existingGoProvider     string
		existingPythonProvider string
		existingNodeJSProvider string
		existingDotnetProvider string
		version                string
		expectError            bool
	}{
		{
			name:                   "existing provider images are respected",
			existingJavaProvider:   "custom/java:tag",
			existingGoProvider:     "custom/go:tag",
			existingPythonProvider: "custom/python:tag",
			existingNodeJSProvider: "custom/nodejs:tag",
			existingDotnetProvider: "custom/dotnet:tag",
			expectError:            false,
		},
		{
			name:        "generates versioned providers when empty",
			version:     "v1.0.0",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up environment
			os.Unsetenv("JAVA_PROVIDER_IMG")
			os.Unsetenv("GO_PROVIDER_IMG")
			os.Unsetenv("PYTHON_PROVIDER_IMG")
			os.Unsetenv("NODEJS_PROVIDER_IMG")
			os.Unsetenv("CSHARP_PROVIDER_IMG")

			// Set up test environment
			if tt.existingJavaProvider != "" {
				os.Setenv("JAVA_PROVIDER_IMG", tt.existingJavaProvider)
			}
			if tt.existingGoProvider != "" {
				os.Setenv("GO_PROVIDER_IMG", tt.existingGoProvider)
			}
			if tt.existingPythonProvider != "" {
				os.Setenv("PYTHON_PROVIDER_IMG", tt.existingPythonProvider)
			}
			if tt.existingNodeJSProvider != "" {
				os.Setenv("NODEJS_PROVIDER_IMG", tt.existingNodeJSProvider)
			}
			if tt.existingDotnetProvider != "" {
				os.Setenv("CSHARP_PROVIDER_IMG", tt.existingDotnetProvider)
			}
			if tt.version != "" {
				originalVersion := Version
				Version = tt.version
				defer func() { Version = originalVersion }()
			}

			c := &Config{}
			err := c.loadProviders()

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Check that environment variables are set appropriately
			if tt.existingJavaProvider != "" && os.Getenv("JAVA_PROVIDER_IMG") != tt.existingJavaProvider {
				t.Errorf("Expected JAVA_PROVIDER_IMG=%s, got %s", tt.existingJavaProvider, os.Getenv("JAVA_PROVIDER_IMG"))
			}
			if tt.existingGoProvider != "" && os.Getenv("GO_PROVIDER_IMG") != tt.existingGoProvider {
				t.Errorf("Expected GO_PROVIDER_IMG=%s, got %s", tt.existingGoProvider, os.Getenv("GO_PROVIDER_IMG"))
			}
			if tt.existingPythonProvider != "" && os.Getenv("PYTHON_PROVIDER_IMG") != tt.existingPythonProvider {
				t.Errorf("Expected PYTHON_PROVIDER_IMG=%s, got %s", tt.existingPythonProvider, os.Getenv("PYTHON_PROVIDER_IMG"))
			}
			if tt.existingNodeJSProvider != "" && os.Getenv("NODEJS_PROVIDER_IMG") != tt.existingNodeJSProvider {
				t.Errorf("Expected NODEJS_PROVIDER_IMG=%s, got %s", tt.existingNodeJSProvider, os.Getenv("NODEJS_PROVIDER_IMG"))
			}
			if tt.existingDotnetProvider != "" && os.Getenv("CSHARP_PROVIDER_IMG") != tt.existingDotnetProvider {
				t.Errorf("Expected CSHARP_PROVIDER_IMG=%s, got %s", tt.existingDotnetProvider, os.Getenv("CSHARP_PROVIDER_IMG"))
			}
		})
	}
}

func TestConfig_Load(t *testing.T) {
	tests := []struct {
		name        string
		expectError bool
	}{
		{
			name:        "successful load",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{}
			err := c.Load()

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}
