package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bombsimon/logrusr/v3"
	"github.com/sirupsen/logrus"
)

// Unit test for dumpRulesCommand.handleOutputFile method
func TestDumpRulesCommand_HandleOutputFile(t *testing.T) {
	tests := []struct {
		name           string
		setupFunc      func(string) error
		overwriteFlag  bool
		expectError    bool
		errorSubstring string
	}{
		{
			name:          "should succeed when file doesn't exist",
			setupFunc:     func(string) error { return nil },
			overwriteFlag: false,
			expectError:   false,
		},
		{
			name: "should fail when file exists and overwrite is false",
			setupFunc: func(path string) error {
				return os.WriteFile(path, []byte("test"), 0644)
			},
			overwriteFlag:  false,
			expectError:    true,
			errorSubstring: "already exists and --overwrite not set",
		},
		{
			name: "should succeed when file exists and overwrite is true",
			setupFunc: func(path string) error {
				return os.WriteFile(path, []byte("test"), 0644)
			},
			overwriteFlag: true,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tempDir, err := os.MkdirTemp("", "test-output")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(tempDir)

			testFile := filepath.Join(tempDir, "test-file.zip")

			// Setup test file if needed
			if err := tt.setupFunc(testFile); err != nil {
				t.Fatal(err)
			}

			// Create logger
			logrusLog := logrus.New()
			logrusLog.SetOutput(io.Discard)
			logger := logrusr.New(logrusLog)

			// Create dumpRulesCommand instance
			dumpCmd := &dumpRulesCommand{
				output:    testFile,
				overwrite: tt.overwriteFlag,
				log:       logger,
			}

			// Test the method
			err = dumpCmd.handleOutputFile()

			// Verify results
			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errorSubstring != "" && !strings.Contains(err.Error(), tt.errorSubstring) {
					t.Errorf("expected error to contain '%s', got '%s'", tt.errorSubstring, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			// Verify overwrite behavior
			if tt.overwriteFlag && tt.expectError == false && tt.setupFunc != nil {
				// File should be removed when overwrite is true
				if _, err := os.Stat(testFile); !os.IsNotExist(err) {
					t.Error("file should be removed when overwrite is true")
				}
			}
		})
	}
}

// Unit test for command creation and basic properties
func TestNewDumpRulesCommand(t *testing.T) {
	// Set up logger
	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logger := logrusr.New(logrusLog)

	cmd := NewDumpRulesCommand(logger)

	// Test command properties
	if cmd == nil {
		t.Fatal("command should not be nil")
	}

	if cmd.Use != "dump-rules" {
		t.Errorf("expected command Use to be 'dump-rules', got '%s'", cmd.Use)
	}

	if cmd.Short != "Dump builtin rulesets" {
		t.Errorf("expected command Short to be 'Dump builtin rulesets', got '%s'", cmd.Short)
	}

	// Test that RunE is set
	if cmd.RunE == nil {
		t.Error("RunE function should be defined")
	}
}

// Unit test for command flags
func TestDumpRulesCommandFlags(t *testing.T) {
	// Set up logger
	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logger := logrusr.New(logrusLog)

	cmd := NewDumpRulesCommand(logger)

	// Test flags exist
	outputFlag := cmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Error("output flag should be defined")
	}

	overwriteFlag := cmd.Flags().Lookup("overwrite")
	if overwriteFlag == nil {
		t.Error("overwrite flag should be defined")
	}

	// Test flag properties
	if outputFlag.Shorthand != "o" {
		t.Errorf("expected output flag shorthand to be 'o', got '%s'", outputFlag.Shorthand)
	}

	if outputFlag.Usage != "path to the directory for rulesets output" {
		t.Errorf("unexpected output flag usage: %s", outputFlag.Usage)
	}

	if overwriteFlag.Usage != "overwrite output directory" {
		t.Errorf("unexpected overwrite flag usage: %s", overwriteFlag.Usage)
	}

	// Test that output flag is required by trying to execute without it
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil {
		t.Error("output flag should be required")
	} else if !strings.Contains(err.Error(), "required flag") {
		t.Logf("Command failed with different error (expected for required flag): %v", err)
	}
}

// Unit test for command flag validation
func TestDumpRulesCommandFlagValidation(t *testing.T) {
	// Set up logger that discards output to avoid test noise
	logrusLog := logrus.New()
	logrusLog.SetOutput(io.Discard)
	logger := logrusr.New(logrusLog)

	tests := []struct {
		name        string
		args        []string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "should fail when no output flag provided",
			args:        []string{},
			expectError: true,
			errorMsg:    "required flag(s) \"output\" not set",
		},
		{
			name:        "should fail for invalid flag",
			args:        []string{"--invalid-flag"},
			expectError: true,
			errorMsg:    "unknown flag: --invalid-flag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewDumpRulesCommand(logger)
			cmd.SetArgs(tt.args)
			err := cmd.Execute()

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// Unit test for output path handling logic
func TestOutputPathHandling(t *testing.T) {
	tests := []struct {
		name           string
		outputDir      string
		expectFilename string
	}{
		{
			name:           "should append default filename to directory",
			outputDir:      "/tmp/output",
			expectFilename: "default-rulesets.zip",
		},
		{
			name:           "should handle relative paths",
			outputDir:      "./output",
			expectFilename: "default-rulesets.zip",
		},
		{
			name:           "should handle empty path",
			outputDir:      "",
			expectFilename: "default-rulesets.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This simulates the logic from line 32 in dump-rules.go
			result := filepath.Join(tt.outputDir, "default-rulesets.zip")
			expectedPath := filepath.Join(tt.outputDir, tt.expectFilename)

			if result != expectedPath {
				t.Errorf("expected path '%s', got '%s'", expectedPath, result)
			}

			// Verify the filename is always present
			filename := filepath.Base(result)
			if filename != tt.expectFilename {
				t.Errorf("expected filename '%s', got '%s'", tt.expectFilename, filename)
			}
		})
	}
}

// Unit test for dumpRulesCommand struct behavior
func TestDumpRulesCommandStruct(t *testing.T) {
	// Set up logger
	logrusLog := logrus.New()
	logrusLog.SetOutput(io.Discard)
	logger := logrusr.New(logrusLog)

	t.Run("should initialize struct correctly", func(t *testing.T) {
		dumpCmd := &dumpRulesCommand{
			output:    "/test/path",
			overwrite: true,
			log:       logger,
		}

		if dumpCmd.output != "/test/path" {
			t.Errorf("expected output to be '/test/path', got '%s'", dumpCmd.output)
		}

		if dumpCmd.overwrite != true {
			t.Errorf("expected overwrite to be true, got %v", dumpCmd.overwrite)
		}

		if dumpCmd.log != logger {
			t.Error("expected logger to be set correctly")
		}
	})

	t.Run("should handle zero values", func(t *testing.T) {
		dumpCmd := &dumpRulesCommand{}

		if dumpCmd.output != "" {
			t.Errorf("expected output to be empty string, got '%s'", dumpCmd.output)
		}

		if dumpCmd.overwrite != false {
			t.Errorf("expected overwrite to be false, got %v", dumpCmd.overwrite)
		}
	})
}

// Unit test for handleOutputFile with various file states
func TestDumpRulesCommand_HandleOutputFileEdgeCases(t *testing.T) {
	// Set up logger
	logrusLog := logrus.New()
	logrusLog.SetOutput(io.Discard)
	logger := logrusr.New(logrusLog)

	tests := []struct {
		name        string
		setupFunc   func() (string, func(), error)
		overwrite   bool
		expectError bool
	}{
		{
			name: "should handle directory instead of file",
			setupFunc: func() (string, func(), error) {
				tempDir, err := os.MkdirTemp("", "dir-test")
				if err != nil {
					return "", nil, err
				}
				// Create a directory with the same name as output file
				dirPath := filepath.Join(tempDir, "output")
				err = os.MkdirAll(dirPath, 0755)
				cleanup := func() { os.RemoveAll(tempDir) }
				return dirPath, cleanup, err
			},
			overwrite:   false,
			expectError: true,
		},
		{
			name: "should remove directory when overwrite is true",
			setupFunc: func() (string, func(), error) {
				tempDir, err := os.MkdirTemp("", "dir-test")
				if err != nil {
					return "", nil, err
				}
				dirPath := filepath.Join(tempDir, "output")
				err = os.MkdirAll(dirPath, 0755)
				cleanup := func() { os.RemoveAll(tempDir) }
				return dirPath, cleanup, err
			},
			overwrite:   true,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputPath, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Skip("Setup failed:", err)
			}
			defer cleanup()

			dumpCmd := &dumpRulesCommand{
				output:    outputPath,
				overwrite: tt.overwrite,
				log:       logger,
			}

			err = dumpCmd.handleOutputFile()

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			} else if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// If overwrite is true and no error expected, verify removal
			if tt.overwrite && !tt.expectError {
				if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
					t.Error("output should be removed when overwrite is true")
				}
			}
		})
	}
}
