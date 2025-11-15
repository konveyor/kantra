package cmd

import (
	"bytes"
	"testing"

	"github.com/bombsimon/logrusr/v3"
	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func TestNewTransformCommand(t *testing.T) {
	// Create a test logger
	testLogger := logrus.New()
	testLogger.SetOutput(bytes.NewBuffer(nil)) // Silence output
	logger := logrusr.New(testLogger)

	cmd := NewTransformCommand(logger)

	// Test command structure
	if cmd.Use != "transform" {
		t.Errorf("Expected Use to be 'transform', got '%s'", cmd.Use)
	}

	if cmd.Short != "Transform application source code" {
		t.Errorf("Expected specific Short description, got '%s'", cmd.Short)
	}

	if cmd.Run == nil {
		t.Error("Expected Run function to be set")
	}

	// Test that subcommands are added
	commands := cmd.Commands()
	expectedSubcommands := []string{"openrewrite"}

	if len(commands) != len(expectedSubcommands) {
		t.Errorf("Expected %d subcommands, got %d", len(expectedSubcommands), len(commands))
	}

	// Check that expected subcommands exist
	foundSubcommands := make(map[string]bool)
	for _, subcmd := range commands {
		foundSubcommands[subcmd.Use] = true
	}

	for _, expected := range expectedSubcommands {
		if !foundSubcommands[expected] {
			t.Errorf("Expected subcommand '%s' not found", expected)
		}
	}
}

func TestTransformCommand_RunFunction(t *testing.T) {
	testLogger := logrus.New()
	testLogger.SetOutput(bytes.NewBuffer(nil))
	logger := logrusr.New(testLogger)

	cmd := NewTransformCommand(logger)

	// Test that Run function shows help
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	
	// Clear any inherited command-line arguments to avoid test flag conflicts
	cmd.SetArgs([]string{})

	// Execute the command without arguments (should show help)
	err := cmd.Execute()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// The Run function calls cmd.Help(), so we should see help output
	output := buf.String()
	if output == "" {
		t.Error("Expected help output, got empty string")
	}

	// Check that help contains expected content
	if !bytes.Contains(buf.Bytes(), []byte("transform")) {
		t.Error("Expected help output to contain 'transform'")
	}
}

func TestTransformCommand_Subcommands(t *testing.T) {
	testLogger := logrus.New()
	testLogger.SetOutput(bytes.NewBuffer(nil))
	logger := logrusr.New(testLogger)

	cmd := NewTransformCommand(logger)
	commands := cmd.Commands()

	tests := []struct {
		name     string
		use      string
		expected bool
	}{
		{"openrewrite subcommand exists", "openrewrite", true},
		{"nonexistent subcommand", "nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := false
			for _, subcmd := range commands {
				if subcmd.Use == tt.use {
					found = true
					break
				}
			}

			if found != tt.expected {
				t.Errorf("Expected subcommand '%s' existence to be %t, got %t", tt.use, tt.expected, found)
			}
		})
	}
}

func TestTransformCommand_WithRootCommand(t *testing.T) {
	testLogger := logrus.New()
	testLogger.SetOutput(bytes.NewBuffer(nil))
	logger := logrusr.New(testLogger)

	// Test that transform command can be added to root command
	rootCmd := &cobra.Command{Use: "test-root"}
	transformCmd := NewTransformCommand(logger)
	
	rootCmd.AddCommand(transformCmd)
	
	// Verify it was added
	commands := rootCmd.Commands()
	found := false
	for _, cmd := range commands {
		if cmd.Use == "transform" {
			found = true
			break
		}
	}
	
	if !found {
		t.Error("Transform command was not properly added to root command")
	}
}

func TestTransformCommand_LoggerUsage(t *testing.T) {
	// Test that logger parameter is used (indirectly by checking subcommands)
	testLogger := logrus.New()
	testLogger.SetOutput(bytes.NewBuffer(nil))
	logger := logrusr.New(testLogger)

	cmd := NewTransformCommand(logger)
	
	// Check that subcommands are created with logger
	commands := cmd.Commands()
	for _, subcmd := range commands {
		// Each subcommand should have been created with the logger
		// This is an indirect test since we can't easily inspect the logger
		if subcmd.Use == "openrewrite" {
			// Test that the subcommand was created successfully
			if subcmd.RunE == nil && subcmd.Run == nil {
				t.Errorf("Subcommand '%s' should have a run function", subcmd.Use)
			}
		}
	}
}

func TestTransformCommand_HelpCommand(t *testing.T) {
	testLogger := logrus.New()
	testLogger.SetOutput(bytes.NewBuffer(nil))
	logger := logrusr.New(testLogger)

	cmd := NewTransformCommand(logger)
	
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
		"transform",
		"Transform application source code",
		"openrewrite",
	}
	
	for _, expected := range expectedStrings {
		if !bytes.Contains(buf.Bytes(), []byte(expected)) {
			t.Errorf("Expected help output to contain '%s'", expected)
		}
	}
}

func TestTransformCommand_NoArgs(t *testing.T) {
	testLogger := logrus.New()
	testLogger.SetOutput(bytes.NewBuffer(nil))
	logger := logrusr.New(testLogger)

	cmd := NewTransformCommand(logger)
	
	// Test that running without arguments shows help
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	
	// Clear any inherited command-line arguments to avoid test flag conflicts
	cmd.SetArgs([]string{})
	
	err := cmd.Execute()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	
	// Should show help output
	if buf.String() == "" {
		t.Error("Expected help output when no arguments provided")
	}
}

func TestTransformCommand_Structure(t *testing.T) {
	testLogger := logrus.New()
	testLogger.SetOutput(bytes.NewBuffer(nil))
	logger := logrusr.New(testLogger)

	cmd := NewTransformCommand(logger)
	
	// Test command properties
	if cmd.Use != "transform" {
		t.Errorf("Expected Use to be 'transform', got '%s'", cmd.Use)
	}
	
	if cmd.Short == "" {
		t.Error("Expected Short description to be set")
	}
	
	if cmd.Run == nil {
		t.Error("Expected Run function to be set")
	}
	
	// Test that it has the expected number of subcommands
	if len(cmd.Commands()) != 1 {
		t.Errorf("Expected 1 subcommand, got %d", len(cmd.Commands()))
	}
}

func TestTransformCommand_LoggerTypes(t *testing.T) {
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
			cmd := NewTransformCommand(tt.logger)
			
			if cmd == nil {
				t.Error("Expected command to be created")
			}
			
			if cmd.Use != "transform" {
				t.Errorf("Expected Use to be 'transform', got '%s'", cmd.Use)
			}
			
			// Test that subcommands are still created
			if len(cmd.Commands()) != 1 {
				t.Errorf("Expected 1 subcommand, got %d", len(cmd.Commands()))
			}
		})
	}
}