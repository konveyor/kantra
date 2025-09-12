package container

import (
	"os"
	"testing"
)

func TestWithProxy(t *testing.T) {
	tests := []struct {
		name                   string
		httpProxy              string
		httpsProxy             string
		noProxy                string
		envVars                map[string]string
		expectedEnvVars        map[string]string
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
			name:      "ignores empty proxy parameters",
			httpProxy: "",
			httpsProxy: "",
			noProxy:   "",
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
				"HTTP_PROXY":   "http://host-proxy.example.com:8080",
				"HTTPS_PROXY":  "https://host-secure-proxy.example.com:8443",
				"NO_PROXY":     "host-localhost,127.0.0.1",
				"ALL_PROXY":    "socks://all-proxy.example.com:1080",
				"http_proxy":   "http://lowercase-proxy.example.com:8080",
				"https_proxy":  "https://lowercase-secure-proxy.example.com:8443",
				"no_proxy":     "lowercase-localhost",
				"all_proxy":    "socks://lowercase-all-proxy.example.com:1080",
			},
			expectedEnvVars: map[string]string{
				"HTTP_PROXY":   "http://host-proxy.example.com:8080",
				"HTTPS_PROXY":  "https://host-secure-proxy.example.com:8443",
				"NO_PROXY":     "host-localhost,127.0.0.1",
				"ALL_PROXY":    "socks://all-proxy.example.com:1080",
				"http_proxy":   "http://lowercase-proxy.example.com:8080",
				"https_proxy":  "https://lowercase-secure-proxy.example.com:8443",
				"no_proxy":     "lowercase-localhost",
				"all_proxy":    "socks://lowercase-all-proxy.example.com:1080",
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
		"HTTP_PROXY":   "http://host-http.example.com:8080",
		"HTTPS_PROXY":  "https://host-https.example.com:8443",
		"NO_PROXY":     "host-no-proxy",
		"ALL_PROXY":    "socks://host-all-proxy.example.com:1080",
		"http_proxy":   "http://host-http-lower.example.com:8080",
		"https_proxy":  "https://host-https-lower.example.com:8443",
		"no_proxy":     "host-no-proxy-lower",
		"all_proxy":    "socks://host-all-proxy-lower.example.com:1080",
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