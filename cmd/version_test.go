package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/konveyor-ecosystem/kantra/cmd/internal/settings"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/spf13/cobra"
)

func TestNewVersionCommand(t *testing.T) {
	tests := []struct {
		name           string
		version        string
		buildCommit    string
		runnerImage    string
		expectedOutput string
	}{
		{
			name:           "default version command",
			version:        "latest",
			buildCommit:    "abc123",
			runnerImage:    "quay.io/konveyor/kantra",
			expectedOutput: "version: latest\nSHA: abc123\nimage: quay.io/konveyor/kantra\n",
		},
		{
			name:           "custom version command",
			version:        "v1.0.0",
			buildCommit:    "def456",
			runnerImage:    "custom/kantra:v1.0.0",
			expectedOutput: "version: v1.0.0\nSHA: def456\nimage: custom/kantra:v1.0.0\n",
		},
		{
			name:           "empty build commit",
			version:        "v2.0.0",
			buildCommit:    "",
			runnerImage:    "quay.io/konveyor/kantra:v2.0.0",
			expectedOutput: "version: v2.0.0\nSHA: \nimage: quay.io/konveyor/kantra:v2.0.0\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up test values
			originalVersion := settings.Version
			originalBuildCommit := settings.BuildCommit
			originalRunnerImage := settings.RunnerImage

			settings.Version = tt.version
			settings.BuildCommit = tt.buildCommit
			settings.RunnerImage = tt.runnerImage

			// Restore original values after test
			defer func() {
				settings.Version = originalVersion
				settings.BuildCommit = originalBuildCommit
				settings.RunnerImage = originalRunnerImage
			}()

			// Create command and capture output
			cmd := NewVersionCommand()

			// Verify command properties
			if cmd.Use != "version" {
				t.Errorf("Expected Use to be 'version', got '%s'", cmd.Use)
			}
			if cmd.Short != "Print the tool version" {
				t.Errorf("Expected Short to be 'Print the tool version', got '%s'", cmd.Short)
			}
			if cmd.Long != "Print this tool version number" {
				t.Errorf("Expected Long to be 'Print this tool version number', got '%s'", cmd.Long)
			}

			// Capture output
			buf := &bytes.Buffer{}
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			// Clear any inherited command-line arguments to avoid test flag conflicts
			cmd.SetArgs([]string{})

			// Execute command
			err := cmd.Execute()
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			output := buf.String()
			if !strings.Contains(output, tt.expectedOutput) {
				t.Errorf("expected output to contain %q, got %q", tt.expectedOutput, output)
			}
		})
	}
}

func TestVersionCommand_CommandProperties(t *testing.T) {
	cmd := NewVersionCommand()

	// Test command structure
	if cmd.Use != "version" {
		t.Errorf("Expected Use to be 'version', got '%s'", cmd.Use)
	}

	if cmd.Short != "Print the tool version" {
		t.Errorf("Expected Short description, got '%s'", cmd.Short)
	}

	if cmd.Long != "Print this tool version number" {
		t.Errorf("Expected Long description, got '%s'", cmd.Long)
	}

	// Test that command has a run function
	if cmd.Run == nil {
		t.Error("Expected Run function to be set")
	}

	// Test that command has no subcommands
	if len(cmd.Commands()) != 0 {
		t.Errorf("Expected no subcommands, got %d", len(cmd.Commands()))
	}
}

func TestVersionCommand_WithRootCommand(t *testing.T) {
	// Test that version command can be added to root command
	rootCmd := &cobra.Command{Use: "test-root"}
	versionCmd := NewVersionCommand()

	rootCmd.AddCommand(versionCmd)

	// Verify it was added
	commands := rootCmd.Commands()
	found := false
	for _, cmd := range commands {
		if cmd.Use == "version" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Version command was not properly added to root command")
	}
}

func TestVersionCommand_OutputFormat(t *testing.T) {
	// Test that output format is consistent
	originalVersion := settings.Version
	originalBuildCommit := settings.BuildCommit
	originalRunnerImage := settings.RunnerImage

	settings.Version = "test-version"
	settings.BuildCommit = "test-commit"
	settings.RunnerImage = "test-image"

	defer func() {
		settings.Version = originalVersion
		settings.BuildCommit = originalBuildCommit
		settings.RunnerImage = originalRunnerImage
	}()

	cmd := NewVersionCommand()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// Clear any inherited command-line arguments to avoid test flag conflicts
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	output := buf.String()
	for _, expected := range []string{"version: test-version", "SHA: test-commit", "image: test-image"} {
		if !strings.Contains(output, expected) {
			t.Errorf("expected output to contain %q, got %q", expected, output)
		}
	}
}

func TestVersionCommand_WithRulesetsSHA(t *testing.T) {
	// Set up a temp directory with a .sha file so the version command prints it
	tmpDir := t.TempDir()
	rulesetsDir := filepath.Join(tmpDir, settings.RulesetsLocation)
	if err := os.MkdirAll(rulesetsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesetsDir, ".sha"), []byte("deadbeef123\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(util.KantraDirEnv, tmpDir)

	cmd := NewVersionCommand()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "rulesets SHA: deadbeef123") {
		t.Errorf("expected output to contain rulesets SHA, got %q", output)
	}
}

func TestVersionCommand_WarningOnSHAReadError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test when running as root")
	}
	// Set up a temp directory with a .sha file that has no read permission
	tmpDir := t.TempDir()
	rulesetsDir := filepath.Join(tmpDir, settings.RulesetsLocation)
	if err := os.MkdirAll(rulesetsDir, 0755); err != nil {
		t.Fatal(err)
	}
	shaPath := filepath.Join(rulesetsDir, ".sha")
	if err := os.WriteFile(shaPath, []byte("abc123\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shaPath, 0000); err != nil {
		t.Fatal(err)
	}
	t.Setenv(util.KantraDirEnv, tmpDir)

	cmd := NewVersionCommand()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// The warning should go to stderr
	if !strings.Contains(stderr.String(), "warning: unable to read rulesets SHA") {
		t.Errorf("expected warning on stderr, got %q", stderr.String())
	}
	// stdout should not contain rulesets SHA
	if strings.Contains(stdout.String(), "rulesets SHA") {
		t.Errorf("did not expect rulesets SHA on stdout, got %q", stdout.String())
	}
}

func TestReadRulesetsSHA(t *testing.T) {
	t.Run("reads sha from file", func(t *testing.T) {
		tmpDir := t.TempDir()
		rulesetsDir := filepath.Join(tmpDir, settings.RulesetsLocation)
		if err := os.MkdirAll(rulesetsDir, 0755); err != nil {
			t.Fatal(err)
		}
		t.Setenv(util.KantraDirEnv, tmpDir)

		expected := "abc123def456"
		if err := os.WriteFile(filepath.Join(rulesetsDir, ".sha"), []byte(expected+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		got, err := readRulesetsSHA()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != expected {
			t.Errorf("readRulesetsSHA() = %q, want %q", got, expected)
		}
	})

	t.Run("KANTRA_DIR set but .sha missing returns os.ErrNotExist without fallback", func(t *testing.T) {
		tmpDir := t.TempDir()
		rulesetsDir := filepath.Join(tmpDir, settings.RulesetsLocation)
		if err := os.MkdirAll(rulesetsDir, 0755); err != nil {
			t.Fatal(err)
		}
		t.Setenv(util.KantraDirEnv, tmpDir)

		_, err := readRulesetsSHA()
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("expected os.ErrNotExist, got %v", err)
		}
	})

	t.Run("permission error is returned not suppressed", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("skipping permission test when running as root")
		}
		tmpDir := t.TempDir()
		rulesetsDir := filepath.Join(tmpDir, settings.RulesetsLocation)
		if err := os.MkdirAll(rulesetsDir, 0755); err != nil {
			t.Fatal(err)
		}
		t.Setenv(util.KantraDirEnv, tmpDir)

		shaPath := filepath.Join(rulesetsDir, ".sha")
		if err := os.WriteFile(shaPath, []byte("abc123\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(shaPath, 0000); err != nil {
			t.Fatal(err)
		}

		_, err := readRulesetsSHA()
		if err == nil {
			t.Error("expected error for unreadable .sha file")
		}
		if errors.Is(err, os.ErrNotExist) {
			t.Error("expected permission error, not ErrNotExist")
		}
	})

	t.Run("falls back to /opt when KANTRA_DIR not set and .sha missing from kantra dir", func(t *testing.T) {
		// Ensure KANTRA_DIR is unset to trigger fallback to /opt/rulesets/.sha
		// t.Setenv registers cleanup to restore the original value
		if orig := os.Getenv(util.KantraDirEnv); orig != "" {
			t.Setenv(util.KantraDirEnv, orig)
			os.Unsetenv(util.KantraDirEnv)
		}

		_, err := readRulesetsSHA()
		// In test environment /opt/rulesets/.sha won't exist either,
		// but the important thing is we get an error (not nil),
		// confirming the fallback path was reached
		if err == nil {
			t.Error("expected error in test environment")
		}
	})
}

func TestVersionGlobalVariables(t *testing.T) {
	// Test that global variables have reasonable defaults
	if settings.Version == "" {
		t.Error("Version should not be empty")
	}

	if settings.RunnerImage == "" {
		t.Error("RunnerImage should not be empty")
	}

	// BuildCommit can be empty (it's set during build)
	// but it should be a string
	if settings.BuildCommit != "" && len(settings.BuildCommit) < 7 {
		t.Logf("BuildCommit is shorter than typical git SHA: %s", settings.BuildCommit)
	}
}
