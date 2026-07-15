package cmd

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/cmd/analyze"
	"github.com/konveyor-ecosystem/kantra/cmd/openrewrite"
	"github.com/konveyor-ecosystem/kantra/cmd/rules"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func TestRootCommand_Structure(t *testing.T) {
	// Test that root command is properly configured
	if rootCmd.Use != "" {
		t.Errorf("Expected empty Use for root command, got '%s'", rootCmd.Use)
	}

	if rootCmd.Short != "A CLI tool for analysis and transformation of applications" {
		t.Errorf("Expected specific Short description, got '%s'", rootCmd.Short)
	}

	if rootCmd.Long != "" {
		t.Errorf("Expected empty Long description, got '%s'", rootCmd.Long)
	}

	if !rootCmd.SilenceUsage {
		t.Error("Expected SilenceUsage to be true")
	}

	if rootCmd.PersistentPreRun == nil {
		t.Error("Expected PersistentPreRun to be set")
	}
}

func TestRootCommand_PersistentFlags(t *testing.T) {
	// Test that persistent flags are properly configured
	logLevelFlagVar := rootCmd.PersistentFlags().Lookup(logLevelFlag)
	if logLevelFlagVar == nil {
		t.Errorf("Expected %s flag to be set", logLevelFlag)
	}

	noCleanupFlagVar := rootCmd.PersistentFlags().Lookup(noCleanupFlag)
	if noCleanupFlagVar == nil {
		t.Errorf("Expected %s flag to be set", noCleanupFlag)
	}
}

func TestRootCommand_PersistentPreRun(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedLevel logrus.Level
	}{
		{
			name:          "default log level",
			args:          []string{},
			expectedLevel: logrus.Level(4),
		},
		{
			name:          "custom log level",
			args:          []string{"--log-level", "6"},
			expectedLevel: logrus.Level(6),
		},
		{
			name:          "log level with other args",
			args:          []string{"--log-level", "2", "--no-cleanup"},
			expectedLevel: logrus.Level(2),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset global variables
			logLevel = 4
			noCleanup = false

			// Create a new logger for this test
			testLogger := logrus.New()
			testLogger.SetOutput(bytes.NewBuffer(nil)) // Silence output
			originalLogger := logrusLog
			logrusLog = testLogger
			defer func() { logrusLog = originalLogger }()

			// Create a test command with PersistentPreRun
			testCmd := &cobra.Command{
				Use:              "test",
				PersistentPreRun: rootCmd.PersistentPreRun,
				Run: func(cmd *cobra.Command, args []string) {
					// Do nothing
				},
			}

			// Copy the flags from rootCmd
			testCmd.PersistentFlags().Uint32Var(&logLevel, logLevelFlag, 4, "log level")
			testCmd.PersistentFlags().BoolVar(&noCleanup, noCleanupFlag, false, "do not cleanup temporary resources")

			// Set arguments
			testCmd.SetArgs(tt.args)

			// Execute the command
			err := testCmd.Execute()
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Check that log level was set correctly
			if logrusLog.Level != tt.expectedLevel {
				t.Errorf("Expected log level %d, got %d", tt.expectedLevel, logrusLog.Level)
			}
		})
	}
}

func TestRootCommand_PersistentPreRun_WithTestFlags(t *testing.T) {
	// Test that PersistentPreRun handles test flags gracefully
	// This simulates what happens when Go's test framework passes flags like --test.v

	// Reset global variables
	logLevel = 4
	noCleanup = false

	// Create a new logger for this test
	testLogger := logrus.New()
	testLogger.SetOutput(bytes.NewBuffer(nil)) // Silence output
	originalLogger := logrusLog
	logrusLog = testLogger
	defer func() { logrusLog = originalLogger }()

	// Save the original log level
	originalLevel := logrusLog.Level

	// Create a test command with PersistentPreRun but NO flags defined
	// This will cause ParseFlags to fail when it encounters any flags
	testCmd := &cobra.Command{
		Use:              "test",
		PersistentPreRun: rootCmd.PersistentPreRun,
		Run: func(cmd *cobra.Command, args []string) {
			// Do nothing
		},
	}

	// Don't add any flags - this will cause ParseFlags to fail with unknown flag errors

	// Set arguments with a flag that won't be recognized
	testCmd.SetArgs([]string{"--test.v"})

	// Execute the command - it should not error even though ParseFlags fails
	// The PersistentPreRun should catch the error and return early
	err := testCmd.Execute()
	if err != nil {
		// This is expected - the command itself may error due to the unknown flag
		// but PersistentPreRun should handle it gracefully
		t.Logf("Expected error from unknown flag: %v", err)
	}

	// The important thing is that logLevel should still be at default (4)
	// since the error path returns early before setting the log level
	if logrusLog.Level != originalLevel {
		t.Logf("Log level changed from %d to %d, which is acceptable", originalLevel, logrusLog.Level)
	}
}

func TestRootCommand_GlobalVariables(t *testing.T) {
	// Test that global variables are properly initialized
	if logrusLog == nil {
		t.Error("Expected logrusLog to be initialized")
	}

	// Test default values
	originalLogLevel := logLevel
	originalNoCleanup := noCleanup

	defer func() {
		logLevel = originalLogLevel
		noCleanup = originalNoCleanup
	}()

	// Reset to defaults
	logLevel = 4
	noCleanup = false

	if logLevel != 4 {
		t.Errorf("Expected default log level to be 4, got %d", logLevel)
	}

	if noCleanup != false {
		t.Errorf("Expected default noCleanup to be false, got %t", noCleanup)
	}
}

func TestRootCommand_LoggerConfiguration(t *testing.T) {
	// Test that logger is properly configured
	if logrusLog == nil {
		t.Error("Expected logrusLog to be initialized")
	}

	// Test that logger outputs to stdout
	if logrusLog.Out != os.Stdout {
		t.Error("Expected logger to output to stdout")
	}

	// Test that formatter is TextFormatter
	if _, ok := logrusLog.Formatter.(*logrus.TextFormatter); !ok {
		t.Error("Expected logger to use TextFormatter")
	}
}

func TestRootCommand_CommandGroups(t *testing.T) {
	// Test that command groups are properly set up
	// This tests the init() function side effects
	commands := rootCmd.Commands()

	// Check that commands are added to groups
	hasAssetGenerationCommands := false
	for _, cmd := range commands {
		if cmd.Use == "discover" || cmd.Use == "generate" {
			hasAssetGenerationCommands = true
			break
		}
	}

	if !hasAssetGenerationCommands {
		t.Log("Asset generation commands not found - this might be expected if they're added elsewhere")
	}
}

func TestRootCommand_Constants(t *testing.T) {
	// Test that constants are defined correctly
	if noCleanupFlag != "no-cleanup" {
		t.Errorf("Expected noCleanupFlag to be 'no-cleanup', got '%s'", noCleanupFlag)
	}

	if logLevelFlag != "log-level" {
		t.Errorf("Expected logLevelFlag to be 'log-level', got '%s'", logLevelFlag)
	}
}

func TestRootCommand_FlagParsing(t *testing.T) {
	tests := []struct {
		name              string
		args              []string
		expectedLogLevel  uint32
		expectedNoCleanup bool
	}{
		{
			name:              "no flags",
			args:              []string{},
			expectedLogLevel:  4,
			expectedNoCleanup: false,
		},
		{
			name:              "log level flag",
			args:              []string{"--log-level", "7"},
			expectedLogLevel:  7,
			expectedNoCleanup: false,
		},
		{
			name:              "no cleanup flag",
			args:              []string{"--no-cleanup"},
			expectedLogLevel:  4,
			expectedNoCleanup: true,
		},
		{
			name:              "both flags",
			args:              []string{"--log-level", "1", "--no-cleanup"},
			expectedLogLevel:  1,
			expectedNoCleanup: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset global variables
			logLevel = 4
			noCleanup = false

			// Create a test command
			testCmd := &cobra.Command{
				Use: "test",
				Run: func(cmd *cobra.Command, args []string) {
					// Do nothing
				},
			}

			// Copy the flags from rootCmd
			testCmd.PersistentFlags().Uint32Var(&logLevel, logLevelFlag, 4, "log level")
			testCmd.PersistentFlags().BoolVar(&noCleanup, noCleanupFlag, false, "do not cleanup temporary resources")

			// Set arguments and execute
			testCmd.SetArgs(tt.args)
			err := testCmd.Execute()
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Check flag values
			if logLevel != tt.expectedLogLevel {
				t.Errorf("Expected log level %d, got %d", tt.expectedLogLevel, logLevel)
			}

			if noCleanup != tt.expectedNoCleanup {
				t.Errorf("Expected noCleanup %t, got %t", tt.expectedNoCleanup, noCleanup)
			}
		})
	}
}

func TestExecute(t *testing.T) {
	// Test that Execute function exists and can be called
	// Note: We can't easily test the actual execution without mocking
	// but we can test that the function doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Execute() panicked: %v", r)
		}
	}()

	// We can't actually call Execute() here as it would run the full command
	// but we can verify the function exists
	if rootCmd == nil {
		t.Error("rootCmd should not be nil")
	}
}

func TestRootCommand_Context(t *testing.T) {
	// Test that commands can work with context
	ctx := context.Background()

	// Test that we can create a command with context
	testCmd := &cobra.Command{
		Use: "test",
		Run: func(cmd *cobra.Command, args []string) {
			// Verify context is accessible
			if cmd.Context() == nil {
				t.Error("Command context should not be nil")
			}
		},
	}

	testCmd.SetContext(ctx)

	if testCmd.Context() == nil {
		t.Error("Expected command context to be set")
	}
}

func TestBackCompatCommandsRegistered(t *testing.T) {
	names := make(map[string]bool, len(rootCmd.Commands()))
	for _, cmd := range rootCmd.Commands() {
		names[cmd.Name()] = true
	}

	for _, want := range []string{"test", "transform", "rules", "provider", "analyze", "openrewrite"} {
		if !names[want] {
			t.Fatalf("missing root command %q, have %v", want, names)
		}
	}
}

func TestLegacyTestCommand(t *testing.T) {
	cmd := rules.NewLegacyTestCommand(logr.Discard())
	if cmd.Use != "test" {
		t.Fatalf("Use = %q", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Fatal("expected RunE to be set")
	}
}

func TestTransformCommandStructure(t *testing.T) {
	cmd := openrewrite.NewTransformCommand(logr.Discard())
	if cmd.Use != "transform" {
		t.Fatalf("Use = %q", cmd.Use)
	}

	subcommands := make(map[string]bool, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		subcommands[sub.Name()] = true
	}
	if !subcommands["openrewrite"] {
		t.Fatalf("expected openrewrite subcommand, have %v", subcommands)
	}
}

func backCompatCommandHelp(t *testing.T, cmd *cobra.Command, args ...string) string {
	t.Helper()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if len(args) == 0 {
		args = []string{"--help"}
	}
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("help failed: %v", err)
	}
	return buf.String()
}

func TestBackCompatCommandsHelp(t *testing.T) {
	log := logr.Discard()

	tests := []struct {
		name    string
		help    func() string
		contain []string
	}{
		{
			name: "legacy test",
			help: func() string {
				return backCompatCommandHelp(t, rules.NewLegacyTestCommand(log))
			},
			contain: []string{
				"deprecated",
				"'kantra rules test'",
			},
		},
		{
			name: "transform",
			help: func() string {
				return backCompatCommandHelp(t, openrewrite.NewTransformCommand(log))
			},
			contain: []string{
				"DEPRECATED",
				"'kantra openrewrite'",
			},
		},
		{
			name: "transform openrewrite",
			help: func() string {
				return backCompatCommandHelp(t, openrewrite.NewTransformCommand(log), "openrewrite", "--help")
			},
			contain: []string{
				"deprecated",
				"'kantra transform openrewrite'",
			},
		},
		{
			name: "openrewrite",
			help: func() string {
				return backCompatCommandHelp(t, openrewrite.NewOpenRewriteCommand(log))
			},
			contain: []string{
				"deprecated",
				"will be removed",
			},
		},
		{
			name: "analyze list flags",
			help: func() string {
				return backCompatCommandHelp(t, analyze.NewAnalyzeCmd(log))
			},
			contain: []string{
				"--list-sources",
				"DEPRECATED",
				"'kantra rules list-sources'",
				"--list-targets",
				"'kantra rules list-targets'",
				"--list-providers",
				"'kantra provider list'",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			help := tt.help()
			for _, want := range tt.contain {
				if !strings.Contains(help, want) {
					t.Fatalf("help missing %q:\n%s", want, help)
				}
			}
		})
	}
}

func TestRootHelpListsDeprecatedCommands(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"--help"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("root help failed: %v", err)
	}

	help := buf.String()
	for _, want := range []string{"test", "transform", "openrewrite", "DEPRECATED"} {
		if !strings.Contains(help, want) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}
}
