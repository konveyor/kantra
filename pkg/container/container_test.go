package container

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runWithNonexistentTool runs Run() with a container tool path that does not exist.
// Run() builds the command and sets reproducerCmd (if provided) before exec fails,
// so we can assert on the built command without executing a real container.
func runWithNonexistentTool(t *testing.T, toolPath string, opts ...Option) (reproducer string, err error) {
	t.Helper()
	var capture string
	opts = append(opts,
		WithContainerToolBin(toolPath),
		WithImage("test-image:latest"),
		WithName("test-name"),
		WithReproduceCmd(&capture),
	)
	c := NewContainer()
	err = c.Run(context.Background(), opts...)
	reproducer = capture
	return reproducer, err
}

func TestRun_PodmanAddsUserNamespaceFlags(t *testing.T) {
	// Use a path that does not exist so exec fails; Run() still builds args and sets reproducerCmd.
	reproducer, err := runWithNonexistentTool(t, "/nonexistent/podman")
	if err == nil {
		t.Fatal("expected Run to fail when container tool does not exist")
	}
	if !strings.Contains(reproducer, "--userns=keep-id") {
		t.Error("expected built command to contain --userns=keep-id when using podman")
	}
	if !strings.Contains(reproducer, "--user=") {
		t.Error("expected built command to contain --user= when using podman")
	}
}

func TestRun_PodmanWithPathVariations(t *testing.T) {
	// Reproducer is set before exec, so we can assert on it regardless of whether exec fails.
	for _, toolPath := range []string{"/nonexistent/usr/bin/podman", "/nonexistent/podman"} {
		reproducer, _ := runWithNonexistentTool(t, toolPath)
		if !strings.Contains(reproducer, "--userns=keep-id") {
			t.Errorf("expected --userns=keep-id for container tool %q, got: %s", toolPath, reproducer)
		}
	}
}

func TestRun_DockerOmitsUserNamespaceFlags(t *testing.T) {
	for _, toolPath := range []string{"/nonexistent/docker", "/nonexistent/usr/local/bin/docker"} {
		reproducer, err := runWithNonexistentTool(t, toolPath)
		if err == nil {
			t.Fatalf("expected Run to fail when container tool %q does not exist", toolPath)
		}
		if strings.Contains(reproducer, "--userns=keep-id") {
			t.Errorf("expected no --userns=keep-id when using docker (tool %q), got: %s", toolPath, reproducer)
		}
	}
}

func TestRun_BaseNameDeterminesPodmanVsDocker(t *testing.T) {
	// Only the base name (last path component) is used to decide podman vs docker.
	dir := filepath.Join(string(filepath.Separator), "nonexistent", "bin")
	podmanPath := filepath.Join(dir, "podman")
	dockerPath := filepath.Join(dir, "docker")

	repPodman, errPodman := runWithNonexistentTool(t, podmanPath)
	repDocker, errDocker := runWithNonexistentTool(t, dockerPath)

	if errPodman == nil || errDocker == nil {
		t.Skip("both runs expected to fail (nonexistent tool)")
	}
	if repPodman == repDocker {
		t.Error("podman and docker run commands should differ (podman has --userns=keep-id)")
	}
	if !strings.Contains(repPodman, "--userns=keep-id") {
		t.Error("podman path should produce --userns=keep-id")
	}
	if strings.Contains(repDocker, "--userns=keep-id") {
		t.Error("docker path should not produce --userns=keep-id")
	}
}

func TestRun_WithVolumesAndEnvInReproducer(t *testing.T) {
	tmpDir := t.TempDir()
	var capture string
	c := NewContainer()
	err := c.Run(context.Background(),
		WithContainerToolBin("/nonexistent/podman"),
		WithImage("test-image"),
		WithName("test-name"),
		WithVolumes(map[string]string{tmpDir: "/container/path"}),
		WithEnv("KEY", "value"),
		WithReproduceCmd(&capture),
	)
	if err == nil {
		t.Fatal("expected Run to fail when container tool does not exist")
	}
	if !strings.Contains(capture, "-v") {
		t.Error("expected reproducer to contain volume mount (-v)")
	}
	if !strings.Contains(capture, tmpDir) {
		t.Error("expected reproducer to contain source volume path")
	}
	if !strings.Contains(capture, "KEY=value") {
		t.Error("expected reproducer to contain env KEY=value")
	}
}

func TestRun_ValidatesRequiredFields(t *testing.T) {
	ctx := context.Background()
	t.Run("missing image", func(t *testing.T) {
		c := NewContainer()
		WithContainerToolBin("/nonexistent/podman")(c)
		WithName("x")(c)
		err := c.Run(ctx)
		if err == nil {
			t.Error("expected error when image is empty")
		}
		if err != nil && !strings.Contains(err.Error(), "image") {
			t.Errorf("expected error to mention image, got: %v", err)
		}
	})
	t.Run("missing container tool", func(t *testing.T) {
		c := NewContainer()
		WithContainerToolBin("")(c)
		WithImage("img")(c)
		WithName("x")(c)
		err := c.Run(ctx)
		if err == nil {
			t.Error("expected error when containerToolBin is empty")
		}
		if err != nil && !strings.Contains(err.Error(), "containerToolBin") {
			t.Errorf("expected error to mention containerToolBin, got: %v", err)
		}
	})
}

func TestWithProxy(t *testing.T) {
	tests := []struct {
		name            string
		httpProxy       string
		httpsProxy      string
		noProxy         string
		envVars         map[string]string
		expectedEnvVars map[string]string
	}{
		{
			name:       "sets proxy environment variables from parameters",
			httpProxy:  "http://proxy.example.com:8080",
			httpsProxy: "https://secure-proxy.example.com:8443",
			noProxy:    "localhost,127.0.0.1,.local",
			expectedEnvVars: map[string]string{
				"HTTP_PROXY":  "http://proxy.example.com:8080",
				"HTTPS_PROXY": "https://secure-proxy.example.com:8443",
				"NO_PROXY":    "localhost,127.0.0.1,.local",
			},
		},
		{
			name:            "ignores empty proxy parameters",
			httpProxy:       "",
			httpsProxy:      "",
			noProxy:         "",
			expectedEnvVars: map[string]string{},
		},
		{
			name:       "sets only non-empty proxy parameters",
			httpProxy:  "http://proxy.example.com:8080",
			httpsProxy: "",
			noProxy:    "localhost",
			expectedEnvVars: map[string]string{
				"HTTP_PROXY": "http://proxy.example.com:8080",
				"NO_PROXY":   "localhost",
			},
		},
		{
			name:       "inherits proxy environment variables from host",
			httpProxy:  "",
			httpsProxy: "",
			noProxy:    "",
			envVars: map[string]string{
				"HTTP_PROXY":  "http://host-proxy.example.com:8080",
				"HTTPS_PROXY": "https://host-secure-proxy.example.com:8443",
				"NO_PROXY":    "host-localhost,127.0.0.1",
				"ALL_PROXY":   "socks://all-proxy.example.com:1080",
				"http_proxy":  "http://lowercase-proxy.example.com:8080",
				"https_proxy": "https://lowercase-secure-proxy.example.com:8443",
				"no_proxy":    "lowercase-localhost",
				"all_proxy":   "socks://lowercase-all-proxy.example.com:1080",
			},
			expectedEnvVars: map[string]string{
				"HTTP_PROXY":  "http://host-proxy.example.com:8080",
				"HTTPS_PROXY": "https://host-secure-proxy.example.com:8443",
				"NO_PROXY":    "host-localhost,127.0.0.1",
				"ALL_PROXY":   "socks://all-proxy.example.com:1080",
				"http_proxy":  "http://lowercase-proxy.example.com:8080",
				"https_proxy": "https://lowercase-secure-proxy.example.com:8443",
				"no_proxy":    "lowercase-localhost",
				"all_proxy":   "socks://lowercase-all-proxy.example.com:1080",
			},
		},
		{
			name:       "parameters override host environment variables",
			httpProxy:  "http://override-proxy.example.com:8080",
			httpsProxy: "https://override-secure-proxy.example.com:8443",
			noProxy:    "override-localhost",
			envVars: map[string]string{
				"HTTP_PROXY":  "http://host-proxy.example.com:8080",
				"HTTPS_PROXY": "https://host-secure-proxy.example.com:8443",
				"NO_PROXY":    "host-localhost",
			},
			expectedEnvVars: map[string]string{
				"HTTP_PROXY":  "http://override-proxy.example.com:8080",
				"HTTPS_PROXY": "https://override-secure-proxy.example.com:8443",
				"NO_PROXY":    "override-localhost",
			},
		},
		{
			name:       "ignores empty host environment variables",
			httpProxy:  "",
			httpsProxy: "",
			noProxy:    "",
			envVars: map[string]string{
				"HTTP_PROXY":  "",
				"HTTPS_PROXY": "https://host-secure-proxy.example.com:8443",
				"NO_PROXY":    "",
			},
			expectedEnvVars: map[string]string{
				"HTTPS_PROXY": "https://host-secure-proxy.example.com:8443",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all proxy environment variables before each test
			proxyVars := []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "no_proxy", "all_proxy"}
			originalEnvVars := make(map[string]string)

			// Save original values and clear them
			for _, proxyVar := range proxyVars {
				originalEnvVars[proxyVar] = os.Getenv(proxyVar)
				os.Unsetenv(proxyVar)
			}

			// Set up test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			// Create a new container and apply the WithProxy option
			container := NewContainer()
			option := WithProxy(tt.httpProxy, tt.httpsProxy, tt.noProxy)
			option(container)

			// Verify the expected environment variables are set correctly
			for expectedKey, expectedValue := range tt.expectedEnvVars {
				actualValue, exists := container.env[expectedKey]
				if !exists {
					t.Errorf("Expected environment variable %s to be set", expectedKey)
				}
				if actualValue != expectedValue {
					t.Errorf("Expected environment variable %s to have value %s, got %s", expectedKey, expectedValue, actualValue)
				}
			}

			// Verify that no unexpected proxy environment variables are set
			for actualKey := range container.env {
				if _, expected := tt.expectedEnvVars[actualKey]; !expected {
					// Check if this is one of the proxy variables we're testing
					isProxyVar := false
					for _, proxyVar := range proxyVars {
						if actualKey == proxyVar {
							isProxyVar = true
							break
						}
					}
					if isProxyVar {
						t.Errorf("Unexpected environment variable %s with value %s", actualKey, container.env[actualKey])
					}
				}
			}

			// Restore original environment variables
			for _, proxyVar := range proxyVars {
				os.Unsetenv(proxyVar)
				if originalValue := originalEnvVars[proxyVar]; originalValue != "" {
					os.Setenv(proxyVar, originalValue)
				}
			}
		})
	}
}

func TestWithProxyDoesNotOverwriteExistingContainerEnv(t *testing.T) {
	// Create a container with existing environment variables
	container := NewContainer()
	container.env["EXISTING_VAR"] = "existing_value"
	container.env["HTTP_PROXY"] = "existing_proxy"

	// Apply WithProxy option
	option := WithProxy("http://new-proxy.example.com:8080", "", "")
	option(container)

	// Verify existing non-proxy environment variable is preserved
	if container.env["EXISTING_VAR"] != "existing_value" {
		t.Errorf("Expected EXISTING_VAR to be 'existing_value', got %s", container.env["EXISTING_VAR"])
	}

	// Verify HTTP_PROXY is overwritten by the WithProxy option
	if container.env["HTTP_PROXY"] != "http://new-proxy.example.com:8080" {
		t.Errorf("Expected HTTP_PROXY to be 'http://new-proxy.example.com:8080', got %s", container.env["HTTP_PROXY"])
	}
}

func TestWithProxyHandlesAllProxyVariables(t *testing.T) {
	// Set all possible proxy environment variables
	proxyVars := map[string]string{
		"HTTP_PROXY":  "http://host-http.example.com:8080",
		"HTTPS_PROXY": "https://host-https.example.com:8443",
		"NO_PROXY":    "host-no-proxy",
		"ALL_PROXY":   "socks://host-all-proxy.example.com:1080",
		"http_proxy":  "http://host-http-lower.example.com:8080",
		"https_proxy": "https://host-https-lower.example.com:8443",
		"no_proxy":    "host-no-proxy-lower",
		"all_proxy":   "socks://host-all-proxy-lower.example.com:1080",
	}

	// Clear and set environment variables
	originalEnvVars := make(map[string]string)
	for key, value := range proxyVars {
		originalEnvVars[key] = os.Getenv(key)
		os.Setenv(key, value)
	}

	// Create container and apply WithProxy
	container := NewContainer()
	option := WithProxy("", "", "")
	option(container)

	// Verify all proxy variables are copied from environment
	for key, expectedValue := range proxyVars {
		actualValue, exists := container.env[key]
		if !exists {
			t.Errorf("Expected environment variable %s to be set", key)
		}
		if actualValue != expectedValue {
			t.Errorf("Expected environment variable %s to have value %s, got %s", key, expectedValue, actualValue)
		}
	}

	// Restore original environment
	for key := range proxyVars {
		os.Unsetenv(key)
		if originalValue := originalEnvVars[key]; originalValue != "" {
			os.Setenv(key, originalValue)
		}
	}
}
