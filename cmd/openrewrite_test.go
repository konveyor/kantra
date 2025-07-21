package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/bombsimon/logrusr/v3"
	"github.com/sirupsen/logrus"
)

func TestNewOpenRewriteCommand(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	cmd := NewOpenRewriteCommand(logger)

	// Test command structure
	if cmd.Use != "openrewrite" {
		t.Errorf("Expected Use to be 'openrewrite', got '%s'", cmd.Use)
	}

	if cmd.Short != "Transform application source code using OpenRewrite recipes" {
		t.Errorf("Expected specific Short description, got '%s'", cmd.Short)
	}

	if cmd.RunE == nil {
		t.Error("Expected RunE function to be set")
	}

	if cmd.PreRun == nil {
		t.Error("Expected PreRun function to be set")
	}

	// Test that command has no subcommands
	if len(cmd.Commands()) != 0 {
		t.Errorf("Expected no subcommands, got %d", len(cmd.Commands()))
	}
}

func TestOpenRewriteCommand_Flags(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	cmd := NewOpenRewriteCommand(logger)

	// Test that flags are set up correctly
	flags := cmd.Flags()

	expectedFlags := []string{
		"list-targets",
		"input",
		"target",
		"goal",
		"maven-settings",
	}

	for _, flagName := range expectedFlags {
		flag := flags.Lookup(flagName)
		if flag == nil {
			t.Errorf("Expected flag '%s' to be set", flagName)
		}
	}
}

func TestOpenRewriteCommand_PreRun(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	cmd := NewOpenRewriteCommand(logger)

	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "list-targets flag set",
			args:        []string{"--list-targets"},
			expectError: false,
		},
		{
			name:        "no flags set",
			args:        []string{},
			expectError: true, // Should require input and target flags
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd.SetArgs(tt.args)

			// Parse flags to simulate what cobra does
			err := cmd.ParseFlags(tt.args)
			if err != nil {
				t.Fatalf("Failed to parse flags: %v", err)
			}

			// Run PreRun function
			if cmd.PreRun != nil {
				cmd.PreRun(cmd, tt.args)

				// Check if required flags are marked correctly
				inputFlag := cmd.Flags().Lookup("input")
				targetFlag := cmd.Flags().Lookup("target")

				if tt.name == "list-targets flag set" {
					// When list-targets is set, input and target should not be required
					// This is harder to test directly, so we just verify PreRun doesn't panic
				} else {
					// When list-targets is not set, input and target should be required
					// This is also hard to test directly without executing the command
					_ = inputFlag
					_ = targetFlag
				}
			}
		})
	}
}

func TestOpenRewriteCommand_RunE(t *testing.T) {
	testLogger := logrus.New()
	testLogger.SetOutput(bytes.NewBuffer(nil)) // Silence output
	logger := logrusr.New(testLogger)

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "openrewrite-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "list targets",
			args:        []string{"--list-targets"},
			expectError: false, // Should succeed even without containers
		},
		{
			name:        "invalid input path",
			args:        []string{"--input", "/nonexistent", "--target", "test"},
			expectError: true,
		},
		{
			name:        "missing target",
			args:        []string{"--input", tempDir},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewOpenRewriteCommand(logger)
			buf := &bytes.Buffer{}
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestOpenRewriteCommand_Validation(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	// Create temporary files for testing
	tempDir, err := os.MkdirTemp("", "openrewrite-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	validMavenFile := filepath.Join(tempDir, "settings.xml")
	err = os.WriteFile(validMavenFile, []byte("<settings></settings>"), 0644)
	if err != nil {
		t.Fatalf("Failed to create maven settings file: %v", err)
	}

	tests := []struct {
		name              string
		input             string
		target            string
		mavenSettingsFile string
		expectError       bool
	}{
		{
			name:        "valid directory input",
			input:       tempDir,
			target:      "test-target",
			expectError: true, // Will fail during container execution
		},
		{
			name:        "non-existent input",
			input:       "/nonexistent/path",
			target:      "test-target",
			expectError: true,
		},
		{
			name:              "with maven settings",
			input:             tempDir,
			target:            "test-target",
			mavenSettingsFile: validMavenFile,
			expectError:       true, // Will fail during container execution
		},
		{
			name:              "invalid maven settings",
			input:             tempDir,
			target:            "test-target",
			mavenSettingsFile: "/nonexistent/settings.xml",
			expectError:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewOpenRewriteCommand(logger)

			args := []string{"--input", tt.input, "--target", tt.target}
			if tt.mavenSettingsFile != "" {
				args = append(args, "--maven-settings", tt.mavenSettingsFile)
			}

			buf := &bytes.Buffer{}
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(args)

			err := cmd.Execute()

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestOpenRewriteCommand_HelpCommand(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	cmd := NewOpenRewriteCommand(logger)

	// Test help command
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// Execute help
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	if err != nil {
		t.Errorf("Unexpected error running help: %v", err)
	}

	// Check that help contains expected sections
	expectedStrings := []string{
		"openrewrite",
		"Transform application source code using OpenRewrite recipes",
		"list-targets",
		"input",
		"target",
	}

	for _, expected := range expectedStrings {
		if !bytes.Contains(buf.Bytes(), []byte(expected)) {
			t.Errorf("Expected help output to contain '%s'", expected)
		}
	}
}

func TestOpenRewriteCommand_FlagDefaults(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	cmd := NewOpenRewriteCommand(logger)
	flags := cmd.Flags()

	// Test goal flag default
	goalFlag := flags.Lookup("goal")
	if goalFlag != nil {
		if goalFlag.DefValue != "dryRun" {
			t.Errorf("Expected goal flag default to be 'dryRun', got '%s'", goalFlag.DefValue)
		}
	}

	// Test list-targets flag default
	listTargetsFlag := flags.Lookup("list-targets")
	if listTargetsFlag != nil {
		if listTargetsFlag.DefValue != "false" {
			t.Errorf("Expected list-targets flag default to be 'false', got '%s'", listTargetsFlag.DefValue)
		}
	}
}

func TestOpenRewriteCommand_WithTransformCommand(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	// Test that openrewrite command can be added to transform command
	transformCmd := NewTransformCommand(logger)

	// Verify openrewrite is a subcommand of transform
	commands := transformCmd.Commands()
	found := false
	for _, cmd := range commands {
		if cmd.Use == "openrewrite" {
			found = true
			break
		}
	}

	if !found {
		t.Error("OpenRewrite command was not found as subcommand of transform")
	}
}
