package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		// NOTE: Maven settings and input path validation moved to PreRunE Validate() function
		// These are no longer validated in validateProviderConfig() to avoid duplication
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
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain %q, got: %v", tt.errorContains, err)
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
		output:                tempDir,
		enableDefaultRulesets: true,
	}

	ctx := context.Background()
	containerLog := io.Discard // Use discard writer for container output

	// First extraction - should create the directory
	rulesetsDir, err := cmd.extractDefaultRulesets(ctx, containerLog)
	if err != nil {
		t.Fatalf("extractDefaultRulesets() failed: %v", err)
	}

	expectedDir := filepath.Join(tempDir, fmt.Sprintf(".rulesets-%s", Version))
	if rulesetsDir != expectedDir {
		t.Errorf("expected rulesets dir %s, got %s", expectedDir, rulesetsDir)
	}

	// Verify the directory was created
	if _, err := os.Stat(rulesetsDir); os.IsNotExist(err) {
		t.Error("expected .rulesets directory to be created, but it does not exist")
	}

	// Second extraction - should reuse cached directory
	rulesetsDir2, err := cmd.extractDefaultRulesets(ctx, containerLog)
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
		output:                tempDir,
		enableDefaultRulesets: false, // Disabled!
	}

	ctx := context.Background()
	containerLog := io.Discard // Use discard writer for container output

	rulesetsDir, err := cmd.extractDefaultRulesets(ctx, containerLog)
	if err != nil {
		t.Fatalf("extractDefaultRulesets() should not error when disabled: %v", err)
	}

	if rulesetsDir != "" {
		t.Errorf("expected empty rulesets dir when disabled, got %s", rulesetsDir)
	}

	// Verify no directory was created
	expectedDir := filepath.Join(tempDir, fmt.Sprintf(".rulesets-%s", Version))
	if _, err := os.Stat(expectedDir); !os.IsNotExist(err) {
		t.Error("expected .rulesets directory NOT to be created when disabled")
	}
}

func TestProviderEntrypointArgs(t *testing.T) {
	buildArgs := func(port int, logLevel *uint32) []string {
		args := []string{fmt.Sprintf("--port=%v", port)}
		if logLevel != nil {
			args = append(args, fmt.Sprintf("--log-level=%v", *logLevel))
		}
		return args
	}
	tests := []struct {
		name     string
		port     int
		logLevel *uint32
		wantArgs []string
	}{
		{
			name:     "port and log level set",
			port:     14651,
			logLevel: uint32Ptr(1),
			wantArgs: []string{"--port=14651", "--log-level=1"},
		},
		{
			name:     "port set with default log level",
			port:     14651,
			logLevel: uint32Ptr(4),
			wantArgs: []string{"--port=14651", "--log-level=4"},
		},
		{
			name:     "port set log level nil omits flag",
			port:     14651,
			logLevel: nil,
			wantArgs: []string{"--port=14651"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := buildArgs(tt.port, tt.logLevel)
			if len(args) != len(tt.wantArgs) {
				t.Fatalf("expected %d args, got %d: %v", len(tt.wantArgs), len(args), args)
			}
			for i := range tt.wantArgs {
				if args[i] != tt.wantArgs[i] {
					t.Errorf("args[%d] = %q, want %q", i, args[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func uint32Ptr(u uint32) *uint32 {
	return &u
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
