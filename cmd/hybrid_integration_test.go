package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

// TestWaitForProviderTimeout tests that waitForProvider correctly times out
// when a provider is not available and returns an appropriate error.
func TestWaitForProviderTimeout(t *testing.T) {
	// Skip if container binary is not available
	binary := getContainerBinary()
	if binary == "" {
		t.Skip("Container runtime not available, skipping integration test")
	}

	ctx := context.Background()

	// Try to wait for a provider on a port that's definitely not in use
	// Use a very short timeout since we expect this to fail
	err := waitForProvider(ctx, "test-provider", 59999, 500*time.Millisecond, logr.Discard())

	if err == nil {
		t.Error("expected waitForProvider to timeout and return error, but got nil")
	}

	if err != nil && err.Error() == "" {
		t.Error("expected error message, got empty string")
	}
}

// TestWaitForProviderCancellation tests that waitForProvider respects
// context cancellation and returns promptly.
func TestWaitForProviderCancellation(t *testing.T) {
	// Skip if container binary is not available
	binary := getContainerBinary()
	if binary == "" {
		t.Skip("Container runtime not available, skipping integration test")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel context immediately
	cancel()

	start := time.Now()
	err := waitForProvider(ctx, "test-provider", 59999, 30*time.Second, logr.Discard())
	elapsed := time.Since(start)

	if err == nil {
		t.Error("expected waitForProvider to return error on cancelled context")
	}

	// Should return quickly (within 1 second) when context is already cancelled
	if elapsed > 1*time.Second {
		t.Errorf("waitForProvider took too long to respect cancellation: %v", elapsed)
	}
}

// TestHybridProviderValidation tests that provider configuration validation
// catches common errors before attempting to start containers.
func TestHybridProviderValidation(t *testing.T) {
	// Skip if container binary is not available
	binary := getContainerBinary()
	if binary == "" {
		t.Skip("Container runtime not available, skipping integration test")
	}

	// Initialize Settings for tests
	if Settings.ContainerBinary == "" {
		Settings.ContainerBinary = binary
	}

	tests := []struct {
		name          string
		setupFunc     func(*analyzeCommand)
		expectError   bool
		errorContains string
	}{
		{
			name: "valid configuration",
			setupFunc: func(a *analyzeCommand) {
				a.providersMap = map[string]ProviderInit{
					"java": {
						port:  65530, // Use high port unlikely to be in use
						image: "test-image",
					},
				}
			},
			expectError: false,
		},
		{
			name: "nonexistent maven settings file",
			setupFunc: func(a *analyzeCommand) {
				a.mavenSettingsFile = "/nonexistent/path/settings.xml"
				a.providersMap = map[string]ProviderInit{
					"java": {
						port:  65531,
						image: "test-image",
					},
				}
			},
			expectError:   true,
			errorContains: "Maven settings file not found",
		},
		{
			name: "nonexistent input path",
			setupFunc: func(a *analyzeCommand) {
				a.input = "/nonexistent/input/path"
				a.providersMap = map[string]ProviderInit{
					"java": {
						port:  65532,
						image: "test-image",
					},
				}
			},
			expectError:   true,
			errorContains: "Input path not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &analyzeCommand{
				AnalyzeCommandContext: AnalyzeCommandContext{
					log:          logr.Discard(),
					providersMap: make(map[string]ProviderInit),
				},
			}

			tt.setupFunc(cmd)

			err := cmd.validateProviderConfig()

			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}

			if tt.expectError && err != nil && tt.errorContains != "" {
				if err.Error() == "" {
					t.Error("expected error message but got empty string")
				}
			}
		})
	}
}

// TestExtractDefaultRulesets tests that default rulesets can be extracted
// from the container and cached properly.
func TestExtractDefaultRulesets(t *testing.T) {
	// Skip if container binary is not available
	binary := getContainerBinary()
	if binary == "" {
		t.Skip("Container runtime not available, skipping integration test")
	}

	// Skip if runner image is not available
	// This test requires the actual kantra runner image to exist
	if !containerImageExists(Settings.RunnerImage) {
		t.Skipf("Runner image %s not available, skipping integration test", Settings.RunnerImage)
	}

	// Initialize Settings for tests
	if Settings.ContainerBinary == "" {
		Settings.ContainerBinary = binary
	}

	// Create a temporary output directory
	tempDir, err := os.MkdirTemp("", "test-extract-rulesets-")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cmd := &analyzeCommand{
		AnalyzeCommandContext: AnalyzeCommandContext{
			log: logr.Discard(),
		},
		output:               tempDir,
		enableDefaultRulesets: true,
	}

	ctx := context.Background()

	// First extraction - should create the directory
	rulesetsDir, err := cmd.extractDefaultRulesets(ctx)
	if err != nil {
		t.Fatalf("extractDefaultRulesets() failed: %v", err)
	}

	expectedDir := filepath.Join(tempDir, ".rulesets")
	if rulesetsDir != expectedDir {
		t.Errorf("expected rulesets dir %s, got %s", expectedDir, rulesetsDir)
	}

	// Verify the directory was created
	if _, err := os.Stat(rulesetsDir); os.IsNotExist(err) {
		t.Error("expected .rulesets directory to be created, but it does not exist")
	}

	// Second extraction - should reuse cached directory
	rulesetsDir2, err := cmd.extractDefaultRulesets(ctx)
	if err != nil {
		t.Fatalf("second extractDefaultRulesets() failed: %v", err)
	}

	if rulesetsDir != rulesetsDir2 {
		t.Error("expected same rulesets directory on second extraction (caching)")
	}
}

// TestExtractDefaultRulesetsDisabled tests that ruleset extraction is skipped
// when default rulesets are disabled.
func TestExtractDefaultRulesetsDisabled(t *testing.T) {
	// Skip if container binary is not available
	binary := getContainerBinary()
	if binary == "" {
		t.Skip("Container runtime not available, skipping integration test")
	}

	// Initialize Settings for tests
	if Settings.ContainerBinary == "" {
		Settings.ContainerBinary = binary
	}

	tempDir, err := os.MkdirTemp("", "test-extract-disabled-")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cmd := &analyzeCommand{
		AnalyzeCommandContext: AnalyzeCommandContext{
			log: logr.Discard(),
		},
		output:               tempDir,
		enableDefaultRulesets: false, // Disabled!
	}

	ctx := context.Background()

	rulesetsDir, err := cmd.extractDefaultRulesets(ctx)
	if err != nil {
		t.Fatalf("extractDefaultRulesets() should not error when disabled: %v", err)
	}

	if rulesetsDir != "" {
		t.Errorf("expected empty rulesets dir when disabled, got %s", rulesetsDir)
	}

	// Verify no directory was created
	expectedDir := filepath.Join(tempDir, ".rulesets")
	if _, err := os.Stat(expectedDir); !os.IsNotExist(err) {
		t.Error("expected .rulesets directory NOT to be created when disabled")
	}
}

// Helper function to check if a container image exists locally
func containerImageExists(imageName string) bool {
	binary := getContainerBinary()
	if binary == "" {
		return false
	}

	cmd := exec.Command(binary, "image", "inspect", imageName)
	return cmd.Run() == nil
}
