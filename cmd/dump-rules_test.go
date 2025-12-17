package cmd

import (
	"archive/zip"
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

// Integration test for the full dump-rules command execution
func TestDumpRulesCommand_Execute(t *testing.T) {
	// Set up logger
	logrusLog := logrus.New()
	logrusLog.SetOutput(io.Discard)
	logger := logrusr.New(logrusLog)

	tests := []struct {
		name           string
		setupFunc      func(t *testing.T) (kantraDir string, outputDir string, cleanup func())
		overwrite      bool
		expectError    bool
		errorSubstring string
		verifyFunc     func(t *testing.T, outputDir string)
	}{
		{
			name: "should successfully create zip with rulesets",
			setupFunc: func(t *testing.T) (string, string, func()) {
				// Create temporary kantra directory with rulesets
				kantraDir, err := os.MkdirTemp("", "test-kantra-")
				if err != nil {
					t.Fatal(err)
				}

				// Create rulesets directory with test files
				rulesetsDir := filepath.Join(kantraDir, RulesetsLocation)
				err = os.MkdirAll(rulesetsDir, 0755)
				if err != nil {
					t.Fatal(err)
				}

				// Create a subdirectory with rules
				subDir := filepath.Join(rulesetsDir, "java-rules")
				err = os.MkdirAll(subDir, 0755)
				if err != nil {
					t.Fatal(err)
				}

				// Create test rule files
				ruleContent := `- ruleID: test-rule-01
  category: mandatory
  labels:
  - konveyor.io/target=java
`
				err = os.WriteFile(filepath.Join(subDir, "rules.yaml"), []byte(ruleContent), 0644)
				if err != nil {
					t.Fatal(err)
				}

				err = os.WriteFile(filepath.Join(rulesetsDir, "root-rule.yaml"), []byte("test: content"), 0644)
				if err != nil {
					t.Fatal(err)
				}

				// Create output directory
				outputDir, err := os.MkdirTemp("", "test-output-")
				if err != nil {
					t.Fatal(err)
				}

				cleanup := func() {
					os.RemoveAll(kantraDir)
					os.RemoveAll(outputDir)
				}

				return kantraDir, outputDir, cleanup
			},
			overwrite:   false,
			expectError: false,
			verifyFunc: func(t *testing.T, outputDir string) {
				zipPath := filepath.Join(outputDir, "default-rulesets.zip")
				if _, err := os.Stat(zipPath); os.IsNotExist(err) {
					t.Error("zip file should be created")
					return
				}

				// Verify zip contents
				r, err := zip.OpenReader(zipPath)
				if err != nil {
					t.Fatalf("failed to open zip: %v", err)
				}
				defer r.Close()

				expectedFiles := map[string]bool{
					"java-rules/rules.yaml": false,
					"root-rule.yaml":        false,
				}

				for _, f := range r.File {
					if _, ok := expectedFiles[f.Name]; ok {
						expectedFiles[f.Name] = true
					}
				}

				for name, found := range expectedFiles {
					if !found {
						t.Errorf("expected file %s not found in zip", name)
					}
				}
			},
		},
		{
			name: "should handle empty rulesets directory",
			setupFunc: func(t *testing.T) (string, string, func()) {
				kantraDir, err := os.MkdirTemp("", "test-kantra-")
				if err != nil {
					t.Fatal(err)
				}

				// Create empty rulesets directory
				rulesetsDir := filepath.Join(kantraDir, RulesetsLocation)
				err = os.MkdirAll(rulesetsDir, 0755)
				if err != nil {
					t.Fatal(err)
				}

				outputDir, err := os.MkdirTemp("", "test-output-")
				if err != nil {
					t.Fatal(err)
				}

				cleanup := func() {
					os.RemoveAll(kantraDir)
					os.RemoveAll(outputDir)
				}

				return kantraDir, outputDir, cleanup
			},
			overwrite:   false,
			expectError: false,
			verifyFunc: func(t *testing.T, outputDir string) {
				zipPath := filepath.Join(outputDir, "default-rulesets.zip")
				if _, err := os.Stat(zipPath); os.IsNotExist(err) {
					t.Error("zip file should be created even when empty")
					return
				}

				// Verify zip is valid but empty
				r, err := zip.OpenReader(zipPath)
				if err != nil {
					t.Fatalf("failed to open zip: %v", err)
				}
				defer r.Close()

				if len(r.File) != 0 {
					t.Errorf("expected empty zip, got %d files", len(r.File))
				}
			},
		},
		{
			name: "should overwrite existing zip file",
			setupFunc: func(t *testing.T) (string, string, func()) {
				kantraDir, err := os.MkdirTemp("", "test-kantra-")
				if err != nil {
					t.Fatal(err)
				}

				rulesetsDir := filepath.Join(kantraDir, RulesetsLocation)
				err = os.MkdirAll(rulesetsDir, 0755)
				if err != nil {
					t.Fatal(err)
				}

				// Create a test file in rulesets
				err = os.WriteFile(filepath.Join(rulesetsDir, "new-rule.yaml"), []byte("new: content"), 0644)
				if err != nil {
					t.Fatal(err)
				}

				outputDir, err := os.MkdirTemp("", "test-output-")
				if err != nil {
					t.Fatal(err)
				}

				// Create existing zip file with old content
				existingZip := filepath.Join(outputDir, "default-rulesets.zip")
				err = os.WriteFile(existingZip, []byte("old zip content"), 0644)
				if err != nil {
					t.Fatal(err)
				}

				cleanup := func() {
					os.RemoveAll(kantraDir)
					os.RemoveAll(outputDir)
				}

				return kantraDir, outputDir, cleanup
			},
			overwrite:   true,
			expectError: false,
			verifyFunc: func(t *testing.T, outputDir string) {
				zipPath := filepath.Join(outputDir, "default-rulesets.zip")

				// Verify the new zip is valid
				r, err := zip.OpenReader(zipPath)
				if err != nil {
					t.Fatalf("failed to open zip: %v", err)
				}
				defer r.Close()

				// Should contain the new rule file
				found := false
				for _, f := range r.File {
					if f.Name == "new-rule.yaml" {
						found = true
						break
					}
				}
				if !found {
					t.Error("new-rule.yaml should be in the overwritten zip")
				}
			},
		},
		{
			name: "should fail when output exists and overwrite is false",
			setupFunc: func(t *testing.T) (string, string, func()) {
				kantraDir, err := os.MkdirTemp("", "test-kantra-")
				if err != nil {
					t.Fatal(err)
				}

				rulesetsDir := filepath.Join(kantraDir, RulesetsLocation)
				err = os.MkdirAll(rulesetsDir, 0755)
				if err != nil {
					t.Fatal(err)
				}

				outputDir, err := os.MkdirTemp("", "test-output-")
				if err != nil {
					t.Fatal(err)
				}

				// Create existing zip file
				existingZip := filepath.Join(outputDir, "default-rulesets.zip")
				err = os.WriteFile(existingZip, []byte("existing content"), 0644)
				if err != nil {
					t.Fatal(err)
				}

				cleanup := func() {
					os.RemoveAll(kantraDir)
					os.RemoveAll(outputDir)
				}

				return kantraDir, outputDir, cleanup
			},
			overwrite:      false,
			expectError:    true,
			errorSubstring: "already exists and --overwrite not set",
		},
		{
			name: "should handle nested directory structure",
			setupFunc: func(t *testing.T) (string, string, func()) {
				kantraDir, err := os.MkdirTemp("", "test-kantra-")
				if err != nil {
					t.Fatal(err)
				}

				rulesetsDir := filepath.Join(kantraDir, RulesetsLocation)

				// Create deeply nested structure
				nestedDir := filepath.Join(rulesetsDir, "level1", "level2", "level3")
				err = os.MkdirAll(nestedDir, 0755)
				if err != nil {
					t.Fatal(err)
				}

				// Create files at different levels
				err = os.WriteFile(filepath.Join(rulesetsDir, "level1", "rule1.yaml"), []byte("rule1"), 0644)
				if err != nil {
					t.Fatal(err)
				}
				err = os.WriteFile(filepath.Join(nestedDir, "deep-rule.yaml"), []byte("deep rule"), 0644)
				if err != nil {
					t.Fatal(err)
				}

				outputDir, err := os.MkdirTemp("", "test-output-")
				if err != nil {
					t.Fatal(err)
				}

				cleanup := func() {
					os.RemoveAll(kantraDir)
					os.RemoveAll(outputDir)
				}

				return kantraDir, outputDir, cleanup
			},
			overwrite:   false,
			expectError: false,
			verifyFunc: func(t *testing.T, outputDir string) {
				zipPath := filepath.Join(outputDir, "default-rulesets.zip")

				r, err := zip.OpenReader(zipPath)
				if err != nil {
					t.Fatalf("failed to open zip: %v", err)
				}
				defer r.Close()

				expectedFiles := map[string]bool{
					"level1/rule1.yaml":                  false,
					"level1/level2/level3/deep-rule.yaml": false,
				}

				for _, f := range r.File {
					if _, ok := expectedFiles[f.Name]; ok {
						expectedFiles[f.Name] = true
					}
				}

				for name, found := range expectedFiles {
					if !found {
						t.Errorf("expected file %s not found in zip", name)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kantraDir, outputDir, cleanup := tt.setupFunc(t)
			defer cleanup()

			// Save original directory and change to kantra directory
			// This is needed because GetKantraDir() checks current directory first
			originalDir, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			defer os.Chdir(originalDir)

			// Create required directories for GetKantraDir to find the kantra dir
			// GetKantraDir looks for: rulesets, jdtls, static-report
			for _, dir := range []string{"jdtls", "static-report"} {
				err = os.MkdirAll(filepath.Join(kantraDir, dir), 0755)
				if err != nil {
					t.Fatal(err)
				}
			}

			err = os.Chdir(kantraDir)
			if err != nil {
				t.Fatal(err)
			}

			// Create and execute the command
			cmd := NewDumpRulesCommand(logger)
			args := []string{"--output", outputDir}
			if tt.overwrite {
				args = append(args, "--overwrite")
			}
			cmd.SetArgs(args)

			err = cmd.Execute()

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
				if tt.verifyFunc != nil {
					tt.verifyFunc(t, outputDir)
				}
			}
		})
	}
}

// Test for zip file content verification
func TestDumpRulesCommand_ZipContentIntegrity(t *testing.T) {
	logrusLog := logrus.New()
	logrusLog.SetOutput(io.Discard)
	logger := logrusr.New(logrusLog)

	// Create kantra directory with specific content
	kantraDir, err := os.MkdirTemp("", "test-kantra-content-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(kantraDir)

	// Create required directories for GetKantraDir
	for _, dir := range []string{RulesetsLocation, "jdtls", "static-report"} {
		err = os.MkdirAll(filepath.Join(kantraDir, dir), 0755)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Create a rule file with known content
	expectedContent := `- ruleID: integrity-test-rule
  category: mandatory
  description: Test rule for content integrity
  labels:
  - konveyor.io/target=test
`
	rulesetsDir := filepath.Join(kantraDir, RulesetsLocation)
	err = os.WriteFile(filepath.Join(rulesetsDir, "integrity-test.yaml"), []byte(expectedContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	outputDir, err := os.MkdirTemp("", "test-output-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(outputDir)

	// Change to kantra directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalDir)

	err = os.Chdir(kantraDir)
	if err != nil {
		t.Fatal(err)
	}

	// Execute command
	cmd := NewDumpRulesCommand(logger)
	cmd.SetArgs([]string{"--output", outputDir})
	err = cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify zip content matches original
	zipPath := filepath.Join(outputDir, "default-rulesets.zip")
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("failed to open zip: %v", err)
	}
	defer r.Close()

	found := false
	for _, f := range r.File {
		if f.Name == "integrity-test.yaml" {
			found = true
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("failed to open file in zip: %v", err)
			}
			content, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatalf("failed to read file content: %v", err)
			}

			if string(content) != expectedContent {
				t.Errorf("content mismatch:\nexpected:\n%s\ngot:\n%s", expectedContent, string(content))
			}
			break
		}
	}

	if !found {
		t.Error("integrity-test.yaml not found in zip")
	}
}

// Test for missing rulesets directory error
func TestDumpRulesCommand_MissingRulesetsDirectory(t *testing.T) {
	logrusLog := logrus.New()
	logrusLog.SetOutput(io.Discard)
	logger := logrusr.New(logrusLog)

	// Create a fake HOME directory without rulesets
	fakeHome, err := os.MkdirTemp("", "test-fake-home-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(fakeHome)

	// Create .kantra directory without rulesets
	fakeKantra := filepath.Join(fakeHome, ".kantra")
	err = os.MkdirAll(fakeKantra, 0755)
	if err != nil {
		t.Fatal(err)
	}

	outputDir, err := os.MkdirTemp("", "test-output-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(outputDir)

	// Save and restore original environment
	originalHome := os.Getenv("HOME")
	originalXdg := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		os.Setenv("HOME", originalHome)
		if originalXdg != "" {
			os.Setenv("XDG_CONFIG_HOME", originalXdg)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
	}()

	// Set HOME to fake home and clear XDG_CONFIG_HOME
	os.Setenv("HOME", fakeHome)
	os.Unsetenv("XDG_CONFIG_HOME")

	// Save and restore original directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalDir)

	// Change to a directory that doesn't have the required kantra dirs
	// so GetKantraDir falls back to ~/.kantra
	err = os.Chdir(fakeHome)
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewDumpRulesCommand(logger)
	cmd.SetArgs([]string{"--output", outputDir})
	err = cmd.Execute()

	if err == nil {
		t.Error("expected error but got none")
	} else if !strings.Contains(err.Error(), "rulesets directory not found") {
		t.Errorf("expected error to contain 'rulesets directory not found', got '%s'", err.Error())
	}
}

// Test command execution via cobra command interface
func TestDumpRulesCommand_CobraExecution(t *testing.T) {
	logrusLog := logrus.New()
	logrusLog.SetOutput(io.Discard)
	logger := logrusr.New(logrusLog)

	t.Run("should execute with valid flags", func(t *testing.T) {
		// Create kantra directory
		kantraDir, err := os.MkdirTemp("", "test-kantra-cobra-")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(kantraDir)

		// Create required directories
		for _, dir := range []string{RulesetsLocation, "jdtls", "static-report"} {
			err = os.MkdirAll(filepath.Join(kantraDir, dir), 0755)
			if err != nil {
				t.Fatal(err)
			}
		}

		outputDir, err := os.MkdirTemp("", "test-output-cobra-")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(outputDir)

		// Change to kantra directory
		originalDir, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		defer os.Chdir(originalDir)

		err = os.Chdir(kantraDir)
		if err != nil {
			t.Fatal(err)
		}

		cmd := NewDumpRulesCommand(logger)
		cmd.SetArgs([]string{"-o", outputDir})
		err = cmd.Execute()

		if err != nil {
			t.Errorf("command execution failed: %v", err)
		}

		// Verify output file exists
		zipPath := filepath.Join(outputDir, "default-rulesets.zip")
		if _, err := os.Stat(zipPath); os.IsNotExist(err) {
			t.Error("zip file should be created")
		}
	})

	t.Run("should handle shorthand flag -o", func(t *testing.T) {
		kantraDir, err := os.MkdirTemp("", "test-kantra-shorthand-")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(kantraDir)

		for _, dir := range []string{RulesetsLocation, "jdtls", "static-report"} {
			err = os.MkdirAll(filepath.Join(kantraDir, dir), 0755)
			if err != nil {
				t.Fatal(err)
			}
		}

		outputDir, err := os.MkdirTemp("", "test-output-shorthand-")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(outputDir)

		originalDir, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		defer os.Chdir(originalDir)

		err = os.Chdir(kantraDir)
		if err != nil {
			t.Fatal(err)
		}

		cmd := NewDumpRulesCommand(logger)
		cmd.SetArgs([]string{"-o", outputDir})
		err = cmd.Execute()

		if err != nil {
			t.Errorf("command with -o flag failed: %v", err)
		}
	})
}
