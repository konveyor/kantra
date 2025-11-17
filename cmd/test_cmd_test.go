package cmd

import (
	"bytes"
	"testing"

	"github.com/bombsimon/logrusr/v3"
	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func TestNewTestCommand(t *testing.T) {
	// Create a test logger
	testLogger := logrus.New()
	testLogger.SetOutput(bytes.NewBuffer(nil)) // Silence output
	logger := logrusr.New(testLogger)

	cmd := NewTestCommand(logger)

	// Test command structure
	if cmd.Use != "test" {
		t.Errorf("Expected Use to be 'test', got '%s'", cmd.Use)
	}

	if cmd.Short != "Test YAML rules" {
		t.Errorf("Expected Short to be 'Test YAML rules', got '%s'", cmd.Short)
	}

	if cmd.RunE == nil {
		t.Error("Expected RunE function to be set")
	}

	// Test that command has no subcommands
	if len(cmd.Commands()) != 0 {
		t.Errorf("Expected no subcommands, got %d", len(cmd.Commands()))
	}
}

func TestTestCommand_Flags(t *testing.T) {
	testLogger := logrus.New()
	testLogger.SetOutput(bytes.NewBuffer(nil))
	logger := logrusr.New(testLogger)

	cmd := NewTestCommand(logger)
	
	// Test that flags are set up correctly
	flags := cmd.Flags()
	
	// Check for expected flags
	expectedFlags := []string{
		"test-filter",
		"base-provider-settings", 
		"prune",
	}
	
	for _, flagName := range expectedFlags {
		flag := flags.Lookup(flagName)
		if flag == nil {
			t.Errorf("Expected flag '%s' to be set", flagName)
		}
	}
}

func TestTestCommand_WithRootCommand(t *testing.T) {
	testLogger := logrus.New()
	testLogger.SetOutput(bytes.NewBuffer(nil))
	logger := logrusr.New(testLogger)

	// Test that test command can be added to root command
	rootCmd := &cobra.Command{Use: "test-root"}
	testCmd := NewTestCommand(logger)
	
	rootCmd.AddCommand(testCmd)
	
	// Verify it was added
	commands := rootCmd.Commands()
	found := false
	for _, cmd := range commands {
		if cmd.Use == "test" {
			found = true
			break
		}
	}
	
	if !found {
		t.Error("Test command was not properly added to root command")
	}
}

func TestTestCommand_RunEFunction(t *testing.T) {
	testLogger := logrus.New()
	testLogger.SetOutput(bytes.NewBuffer(nil))
	logger := logrusr.New(testLogger)

	cmd := NewTestCommand(logger)
	
	// Test that RunE function exists and can be called
	if cmd.RunE == nil {
		t.Error("Expected RunE function to be set")
		return
	}
	
	// Test with empty args (should handle gracefully)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	
	err := cmd.RunE(cmd, []string{})
	// This might return an error or succeed depending on implementation
	// We just want to make sure it doesn't panic
	if err != nil {
		t.Logf("RunE returned error (expected): %v", err)
	}
}

func TestTestCommand_HelpCommand(t *testing.T) {
	testLogger := logrus.New()
	testLogger.SetOutput(bytes.NewBuffer(nil))
	logger := logrusr.New(testLogger)

	cmd := NewTestCommand(logger)
	
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
		"test",
		"Test YAML rules",
	}
	
	for _, expected := range expectedStrings {
		if !bytes.Contains(buf.Bytes(), []byte(expected)) {
			t.Errorf("Expected help output to contain '%s'", expected)
		}
	}
}

func TestTestCommand_LoggerTypes(t *testing.T) {
	// Test with different logger types
	tests := []struct {
		name   string
		logger logr.Logger
	}{
		{
			name:   "with logrus logger",
			logger: func() logr.Logger {
				testLogger := logrus.New()
				testLogger.SetOutput(bytes.NewBuffer(nil))
				return logrusr.New(testLogger)
			}(),
		},
		{
			name:   "with null logger",
			logger: logr.Discard(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewTestCommand(tt.logger)
			
			if cmd == nil {
				t.Error("Expected command to be created")
			}
			
			if cmd.Use != "test" {
				t.Errorf("Expected Use to be 'test', got '%s'", cmd.Use)
			}
			
			if cmd.RunE == nil {
				t.Error("Expected RunE function to be set")
			}
		})
	}
}

func TestTestCommand_FlagValues(t *testing.T) {
	testLogger := logrus.New()
	testLogger.SetOutput(bytes.NewBuffer(nil))
	logger := logrusr.New(testLogger)

	cmd := NewTestCommand(logger)
	
	// Test flag defaults
	flags := cmd.Flags()
	
	// Test test-filter flag
	filterFlag := flags.Lookup("test-filter")
	if filterFlag != nil {
		if filterFlag.DefValue != "" {
			t.Errorf("Expected test-filter flag default to be empty, got '%s'", filterFlag.DefValue)
		}
	}
	
	// Test base-provider-settings flag
	baseProviderFlag := flags.Lookup("base-provider-settings")
	if baseProviderFlag != nil {
		if baseProviderFlag.DefValue != "" {
			t.Errorf("Expected base-provider-settings flag default to be empty, got '%s'", baseProviderFlag.DefValue)
		}
	}
	
	// Test prune flag
	pruneFlag := flags.Lookup("prune")
	if pruneFlag != nil {
		if pruneFlag.DefValue != "false" {
			t.Errorf("Expected prune flag default to be 'false', got '%s'", pruneFlag.DefValue)
		}
	}
}

func TestTestCommand_ExecuteWithArgs(t *testing.T) {
	testLogger := logrus.New()
	testLogger.SetOutput(bytes.NewBuffer(nil))
	logger := logrusr.New(testLogger)

	cmd := NewTestCommand(logger)
	
	// Test execution with various arguments
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "with test-filter flag",
			args: []string{"--test-filter", "test-filter"},
		},
		{
			name: "with prune flag",
			args: []string{"--prune"},
		},
		{
			name: "with base-provider-settings flag",
			args: []string{"--base-provider-settings", "settings.yaml"},
		},
		{
			name: "with multiple flags",
			args: []string{"--test-filter", "test", "--prune", "--base-provider-settings", "settings.yaml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			// Execute command - it may return an error (expected for testing)
			err := cmd.Execute()
			if err != nil {
				t.Logf("Command execution returned error (may be expected): %v", err)
			}
			
			// The main test is that it doesn't panic
		})
	}
}

func TestTestCommand_Validation(t *testing.T) {
	testLogger := logrus.New()
	testLogger.SetOutput(bytes.NewBuffer(nil))
	logger := logrusr.New(testLogger)

	cmd := NewTestCommand(logger)
	
	// Test that command accepts file arguments
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	
	// Test with fake file arguments
	cmd.SetArgs([]string{"test-file.yaml"})
	
	err := cmd.Execute()
	// This will likely fail because the file doesn't exist, but that's expected
	if err != nil {
		t.Logf("Expected error for non-existent file: %v", err)
	}
}