package cmd

import (
	"os"
	"os/exec"
	"testing"
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
		name                 string
		containerTool        string
		podmanBin           string
		expectContainerTool  string
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
			podmanBin:          "/usr/local/bin/podman",
			expectContainerTool: "/usr/local/bin/podman",
			expectError:         false,
		},
		{
			name:                "fallback to podman in PATH",
			containerTool:       "",
			podmanBin:          "",
			expectError:         false,
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

func TestConfig_loadCommandName(t *testing.T) {
	// Note: This function depends on util.RootCommandName which is typically a constant
	// We can only test that the function executes without error since we can't easily mock the util package
	c := &Config{}
	err := c.loadCommandName()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	
	// The function should complete without error regardless of util.RootCommandName value
	// If util.RootCommandName != "kantra", it will set CMD_NAME environment variable
	// but we can't easily test this without modifying the util package
}

func TestConfig_loadProviders(t *testing.T) {
	tests := []struct {
		name                   string
		existingJavaProvider   string
		existingGenericProvider string
		existingDotnetProvider string
		version                string
		expectError            bool
	}{
		{
			name:                   "existing provider images are respected",
			existingJavaProvider:   "custom/java:tag",
			existingGenericProvider: "custom/generic:tag",
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
			os.Unsetenv("GENERIC_PROVIDER_IMG")
			os.Unsetenv("DOTNET_PROVIDER_IMG")
			
			// Set up test environment
			if tt.existingJavaProvider != "" {
				os.Setenv("JAVA_PROVIDER_IMG", tt.existingJavaProvider)
			}
			if tt.existingGenericProvider != "" {
				os.Setenv("GENERIC_PROVIDER_IMG", tt.existingGenericProvider)
			}
			if tt.existingDotnetProvider != "" {
				os.Setenv("DOTNET_PROVIDER_IMG", tt.existingDotnetProvider)
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
			if tt.existingGenericProvider != "" && os.Getenv("GENERIC_PROVIDER_IMG") != tt.existingGenericProvider {
				t.Errorf("Expected GENERIC_PROVIDER_IMG=%s, got %s", tt.existingGenericProvider, os.Getenv("GENERIC_PROVIDER_IMG"))
			}
			if tt.existingDotnetProvider != "" && os.Getenv("DOTNET_PROVIDER_IMG") != tt.existingDotnetProvider {
				t.Errorf("Expected DOTNET_PROVIDER_IMG=%s, got %s", tt.existingDotnetProvider, os.Getenv("DOTNET_PROVIDER_IMG"))
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
