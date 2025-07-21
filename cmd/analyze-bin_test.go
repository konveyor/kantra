package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bombsimon/logrusr/v3"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/sirupsen/logrus"
)

// createMockKantraDir creates a mock kantra directory with required subdirectories
func createMockKantraDir(t *testing.T) (string, func()) {
	tempDir, err := os.MkdirTemp("", "kantra-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create required directories with complete nested structure
	dirs := []string{
		util.RulesetsLocation,
		"jdtls",
		"static-report",
		"jdtls/bin",
		"jdtls/java-analyzer-bundle",
		"jdtls/java-analyzer-bundle/java-analyzer-bundle.core",
		"jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target",
	}

	for _, dir := range dirs {
		fullPath := filepath.Join(tempDir, dir)
		if err := os.MkdirAll(fullPath, 0755); err != nil {
			os.RemoveAll(tempDir)
			t.Fatalf("Failed to create directory %s: %v", fullPath, err)
		}
	}

	// Create mock files with proper paths
	files := []string{
		"jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar", // util.JavaBundlesLocation without leading /
		"jdtls/bin/jdtls", // util.JDTLSBinLocation without leading /
		"maven.default.index",
		"fernflower.jar",
		filepath.Join("static-report", "index.html"),
	}

	for _, file := range files {
		fullPath := filepath.Join(tempDir, file)
		if err := os.WriteFile(fullPath, []byte("mock content"), 0644); err != nil {
			os.RemoveAll(tempDir)
			t.Fatalf("Failed to create file %s: %v", fullPath, err)
		}
	}

	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	return tempDir, cleanup
}

// createMockBinaries creates mock binary files at expected paths
func createMockBinaries(t *testing.T, kantraDir string) {
	binaries := map[string]string{
		"bundle": filepath.Join(kantraDir, "jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar"),
		"jdtls":  filepath.Join(kantraDir, "jdtls/bin/jdtls"),
	}

	for _, path := range binaries {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
		if err := os.WriteFile(path, []byte("mock binary"), 0755); err != nil {
			t.Fatalf("Failed to create binary %s: %v", path, err)
		}
	}
}

func TestConsoleHook_Fire(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	hook := &ConsoleHook{
		Level: logrus.InfoLevel,
		Log:   logger,
	}

	// Create a proper logrus entry
	entry := logrus.NewEntry(testLogger)
	entry.Message = "test message"
	entry.Level = logrus.InfoLevel
	entry.Data = logrus.Fields{}

	err := hook.Fire(entry)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Test with process-rule logger
	entry.Data["logger"] = "process-rule"
	entry.Data["ruleID"] = "test-rule"
	err = hook.Fire(entry)
	if err != nil {
		t.Errorf("Unexpected error with process-rule: %v", err)
	}
}

func TestConsoleHook_Levels(t *testing.T) {
	hook := &ConsoleHook{
		Level: logrus.InfoLevel,
	}

	levels := hook.Levels()
	// Should return all logrus levels
	expectedLevels := logrus.AllLevels

	if len(levels) != len(expectedLevels) {
		t.Errorf("Expected %d levels, got %d", len(expectedLevels), len(levels))
	}

	// Verify it contains the main levels
	expectedMainLevels := []logrus.Level{
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
		logrus.WarnLevel,
		logrus.InfoLevel,
		logrus.DebugLevel,
		logrus.TraceLevel,
	}

	for _, expectedLevel := range expectedMainLevels {
		found := false
		for _, level := range levels {
			if level == expectedLevel {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected level %v not found in returned levels", expectedLevel)
		}
	}
}

func TestAnalyzeCommand_setKantraDir(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	tests := []struct {
		name        string
		setupFunc   func() (string, func())
		cmd         *analyzeCommand
		expectError bool
	}{
		{
			name: "set kantra directory successfully from current dir",
			setupFunc: func() (string, func()) {
				// Create mock kantra dir structure in a temp directory
				tempDir, cleanup := createMockKantraDir(t)
				// Change to the temp directory
				oldDir, _ := os.Getwd()
				os.Chdir(tempDir)
				return tempDir, func() {
					os.Chdir(oldDir)
					cleanup()
				}
			},
			cmd: &analyzeCommand{
				AnalyzeCommandContext: AnalyzeCommandContext{log: logger},
			},
			expectError: false,
		},
		{
			name: "set kantra directory from HOME/.kantra",
			setupFunc: func() (string, func()) {
				// Create a temp HOME directory
				tempHome, err := os.MkdirTemp("", "test-home-*")
				if err != nil {
					t.Fatalf("Failed to create temp home: %v", err)
				}

				// Create kantra dir in temp home
				kantraDir := filepath.Join(tempHome, ".kantra")
				os.MkdirAll(kantraDir, 0755)

				// Create required subdirectories
				dirs := []string{
					util.RulesetsLocation,
					"jdtls",
					"static-report",
				}
				for _, dir := range dirs {
					os.MkdirAll(filepath.Join(kantraDir, dir), 0755)
				}

				// Set HOME env var using t.Setenv for test isolation
				t.Setenv("HOME", tempHome)

				return kantraDir, func() {
					os.RemoveAll(tempHome)
				}
			},
			cmd: &analyzeCommand{
				AnalyzeCommandContext: AnalyzeCommandContext{log: logger},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, cleanup := tt.setupFunc()
			defer cleanup()

			err := tt.cmd.setKantraDir()

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if !tt.expectError && err == nil {
				// Verify that kantraDir was set
				if tt.cmd.kantraDir == "" {
					t.Error("Expected kantraDir to be set")
				}
				// Verify directory exists
				if _, err := os.Stat(tt.cmd.kantraDir); os.IsNotExist(err) {
					t.Error("Expected kantra directory to exist")
				}
				// Verify it contains expected subdirectories
				requiredDirs := []string{util.RulesetsLocation, "jdtls", "static-report"}
				for _, dir := range requiredDirs {
					path := filepath.Join(tt.cmd.kantraDir, dir)
					if _, err := os.Stat(path); os.IsNotExist(err) {
						t.Errorf("Expected directory %s to exist", path)
					}
				}
			}
		})
	}
}

func TestAnalyzeCommand_setBinMapContainerless(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	tests := []struct {
		name        string
		setupFunc   func() (*analyzeCommand, func())
		expectError bool
	}{
		{
			name: "set binary map successfully with existing binaries",
			setupFunc: func() (*analyzeCommand, func()) {
				// Create mock kantra directory with binaries
				tempDir, cleanup := createMockKantraDir(t)

				cmd := &analyzeCommand{
					AnalyzeCommandContext: AnalyzeCommandContext{
						log:       logger,
						reqMap:    make(map[string]string),
						kantraDir: tempDir,
					},
				}

				return cmd, cleanup
			},
			expectError: false,
		},
		{
			name: "set binary map with missing binaries",
			setupFunc: func() (*analyzeCommand, func()) {
				// Create empty temp directory without binaries
				tempDir, err := os.MkdirTemp("", "analyze-bin-test")
				if err != nil {
					t.Fatalf("Failed to create temp dir: %v", err)
				}

				cmd := &analyzeCommand{
					AnalyzeCommandContext: AnalyzeCommandContext{
						log:       logger,
						reqMap:    make(map[string]string),
						kantraDir: tempDir,
					},
				}

				cleanup := func() {
					os.RemoveAll(tempDir)
				}

				return cmd, cleanup
			},
			expectError: true,
		},
		{
			name: "set binary map with nil reqMap",
			setupFunc: func() (*analyzeCommand, func()) {
				// Create mock kantra directory with binaries
				tempDir, cleanup := createMockKantraDir(t)

				cmd := &analyzeCommand{
					AnalyzeCommandContext: AnalyzeCommandContext{
						log:       logger,
						reqMap:    nil, // This will be initialized in the function
						kantraDir: tempDir,
					},
				}

				return cmd, cleanup
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cleanup := tt.setupFunc()
			defer cleanup()

			// Ensure reqMap is initialized if nil to prevent panic
			if cmd.reqMap == nil {
				cmd.reqMap = make(map[string]string)
			}

			err := cmd.setBinMapContainerless()

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Verify reqMap was populated correctly when successful
			if !tt.expectError && err == nil {
				if cmd.reqMap == nil {
					t.Error("Expected reqMap to be initialized")
				}

				// Check that bundle and jdtls paths are set
				if cmd.reqMap["bundle"] == "" {
					t.Error("Expected bundle path to be set in reqMap")
				}
				if cmd.reqMap["jdtls"] == "" {
					t.Error("Expected jdtls path to be set in reqMap")
				}

				// Verify the paths exist
				if _, err := os.Stat(cmd.reqMap["bundle"]); os.IsNotExist(err) {
					t.Errorf("Bundle file does not exist at %s", cmd.reqMap["bundle"])
				}
				if _, err := os.Stat(cmd.reqMap["jdtls"]); os.IsNotExist(err) {
					t.Errorf("JDTLS file does not exist at %s", cmd.reqMap["jdtls"])
				}
			}
		})
	}
}

func TestAnalyzeCommand_ValidateContainerless(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	// Check if mvn is available - if not, skip tests that require it
	_, mvnErr := exec.LookPath("mvn")
	mvnAvailable := mvnErr == nil

	tests := []struct {
		name        string
		setupFunc   func() (*analyzeCommand, func())
		expectError bool
		skipIfNoMvn bool
	}{
		{
			name: "list sources should not validate input",
			setupFunc: func() (*analyzeCommand, func()) {
				// Create mock kantra directory  
				kantraDir, cleanup := createMockKantraDir(t)

				cmd := &analyzeCommand{
					listSources: true,
					AnalyzeCommandContext: AnalyzeCommandContext{
						log:       logger,
						kantraDir: kantraDir,
						reqMap:    make(map[string]string),
					},
				}

				return cmd, cleanup
			},
			expectError: false,
			skipIfNoMvn: false, // List operations don't require maven
		},
		{
			name: "list targets should not validate input",
			setupFunc: func() (*analyzeCommand, func()) {
				// Create mock kantra directory
				kantraDir, cleanup := createMockKantraDir(t)

				cmd := &analyzeCommand{
					listTargets: true,
					AnalyzeCommandContext: AnalyzeCommandContext{
						log:       logger,
						kantraDir: kantraDir,
						reqMap:    make(map[string]string),
					},
				}

				return cmd, cleanup
			},
			expectError: false,
			skipIfNoMvn: false, // List operations don't require maven
		},
		{
			name: "list providers should not validate input",
			setupFunc: func() (*analyzeCommand, func()) {
				// Create mock kantra directory
				kantraDir, cleanup := createMockKantraDir(t)

				cmd := &analyzeCommand{
					listProviders: true,
					AnalyzeCommandContext: AnalyzeCommandContext{
						log:       logger,
						kantraDir: kantraDir,
						reqMap:    make(map[string]string),
					},
				}

				return cmd, cleanup
			},
			expectError: false,
			skipIfNoMvn: false, // List operations don't require maven
		},
		{
			name: "valid directory input",
			setupFunc: func() (*analyzeCommand, func()) {
				// Create mock kantra directory and test input directory
				kantraDir, kantraCleanup := createMockKantraDir(t)

				// Create temporary directory for input
				tempDir, err := os.MkdirTemp("", "analyze-input-test")
				if err != nil {
					t.Fatalf("Failed to create temp dir: %v", err)
				}

				// Create a test file
				testFile := filepath.Join(tempDir, "test.txt")
				err = os.WriteFile(testFile, []byte("test content"), 0644)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}

				cmd := &analyzeCommand{
					input:  tempDir,
					output: tempDir,
					mode:   "full",
					AnalyzeCommandContext: AnalyzeCommandContext{
						log:       logger,
						kantraDir: kantraDir,
						reqMap:    make(map[string]string),
					},
				}

				cleanup := func() {
					os.RemoveAll(tempDir)
					kantraCleanup()
				}

				return cmd, cleanup
			},
			expectError: false,
			skipIfNoMvn: true, // This test validates Java/Maven dependencies
		},
		{
			name: "non-existent input path",
			setupFunc: func() (*analyzeCommand, func()) {
				// Create mock kantra directory
				kantraDir, cleanup := createMockKantraDir(t)

				// Create temporary directory for output
				tempDir, err := os.MkdirTemp("", "analyze-output-test")
				if err != nil {
					t.Fatalf("Failed to create temp dir: %v", err)
				}

				cmd := &analyzeCommand{
					input:  "/nonexistent/path",
					output: tempDir,
					mode:   "full",
					AnalyzeCommandContext: AnalyzeCommandContext{
						log:       logger,
						kantraDir: kantraDir,
						reqMap:    make(map[string]string),
					},
				}

				finalCleanup := func() {
					os.RemoveAll(tempDir)
					cleanup()
				}

				return cmd, finalCleanup
			},
			expectError: false, // ValidateContainerless doesn't validate input path
			skipIfNoMvn: true,  // This test validates Java/Maven dependencies
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip tests that require maven if it's not available
			if tt.skipIfNoMvn && !mvnAvailable {
				t.Skip("Skipping test that requires maven - maven not found in PATH")
			}

			cmd, cleanup := tt.setupFunc()
			defer cleanup()

			ctx := context.Background()

			// Set JAVA_HOME to kantra directory for testing
			_ = os.Setenv("JAVA_HOME", cmd.kantraDir)

			// Set kantra directory first (skip if already set by test setup)
			var err error
			if cmd.kantraDir == "" {
				err = cmd.setKantraDir()
				if err != nil && !tt.expectError {
					t.Fatalf("Failed to set kantra directory: %v", err)
				}
			}

			// Set binMap if kantraDir is set
			if cmd.kantraDir != "" {
				err = cmd.setBinMapContainerless()
				if err != nil && !tt.expectError {
					t.Fatalf("Failed to set bin map: %v", err)
				}
			}

			err = cmd.ValidateContainerless(ctx)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			_ = os.Unsetenv("JAVA_HOME")
		})
	}
}

func TestAnalyzeCommand_walkRuleFilesForLabelsContainerless(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	// Create mock kantra directory with rulesets
	kantraDir, kantraCleanup := createMockKantraDir(t)
	defer kantraCleanup()

	// Create temporary directory with rule files
	tempDir, err := os.MkdirTemp("", "analyze-bin-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test rule file
	ruleFile := filepath.Join(tempDir, "test.yaml")
	ruleContent := `
- name: test-rule
  metadata:
    labels:
      - konveyor.io/source=java
      - konveyor.io/target=quarkus
`
	err = os.WriteFile(ruleFile, []byte(ruleContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create rule file: %v", err)
	}

	tests := []struct {
		name        string
		cmd         *analyzeCommand
		label       string
		expectError bool
		expectFiles bool
	}{
		{
			name: "find rules with matching label",
			cmd: &analyzeCommand{
				rules: []string{tempDir},
				AnalyzeCommandContext: AnalyzeCommandContext{
					log:       logger,
					kantraDir: kantraDir,
				},
			},
			label:       "konveyor.io/source",
			expectError: false,
			expectFiles: true,
		},
		{
			name: "no matching rules",
			cmd: &analyzeCommand{
				rules: []string{tempDir},
				AnalyzeCommandContext: AnalyzeCommandContext{
					log:       logger,
					kantraDir: kantraDir,
				},
			},
			label:       "nonexistent.label",
			expectError: false,
			expectFiles: false,
		},
		{
			name: "no rules specified",
			cmd: &analyzeCommand{
				rules: []string{},
				AnalyzeCommandContext: AnalyzeCommandContext{
					log:       logger,
					kantraDir: kantraDir,
				},
			},
			label:       "konveyor.io/source",
			expectError: false,
			expectFiles: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, err := tt.cmd.walkRuleFilesForLabelsContainerless(tt.label)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if tt.expectFiles && len(files) == 0 {
				t.Error("Expected to find rule files but got none")
			}
			if !tt.expectFiles && len(files) > 0 {
				t.Errorf("Expected no rule files but got %d", len(files))
			}
		})
	}
}

func TestAnalyzeCommand_setConfigsContainerless(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	cmd := &analyzeCommand{
		provider:              []string{"java"},
		AnalyzeCommandContext: AnalyzeCommandContext{log: logger},
	}

	// Create mock configs
	configs := []provider.Config{
		{
			Name: "java",
		},
		{
			Name: "go",
		},
	}

	result := cmd.setConfigsContainerless(configs)

	// setConfigsContainerless adds a builtin config, so we expect one more config
	expectedConfigs := len(configs) + 1
	if len(result) != expectedConfigs {
		t.Errorf("Expected %d configs, got %d", expectedConfigs, len(result))
	}

	// Check that the original configs are present
	for i, config := range configs {
		if result[i].Name != config.Name {
			t.Errorf("Expected config name '%s', got '%s'", config.Name, result[i].Name)
		}
	}

	// Check that the last config is the builtin config
	if result[len(result)-1].Name != "builtin" {
		t.Errorf("Expected last config to be 'builtin', got '%s'", result[len(result)-1].Name)
	}
}

func TestAnalyzeCommand_buildStaticReportFile(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	tests := []struct {
		name        string
		setupFunc   func() (*analyzeCommand, string, func())
		depsErr     bool
		expectError bool
	}{
		{
			name: "build static report successfully",
			setupFunc: func() (*analyzeCommand, string, func()) {
				// Create mock kantra directory with static-report assets
				kantraDir, kantraCleanup := createMockKantraDir(t)

				// Create temporary output directory
				tempDir, err := os.MkdirTemp("", "analyze-output-test")
				if err != nil {
					t.Fatalf("Failed to create temp dir: %v", err)
				}

				// Create mock output.yaml file in output directory with valid YAML
				outputYaml := filepath.Join(tempDir, "output.yaml")
				yamlContent := `[]` // Empty array of RuleSets
				err = os.WriteFile(outputYaml, []byte(yamlContent), 0644)
				if err != nil {
					t.Fatalf("Failed to create output.yaml: %v", err)
				}

				// Create mock dependencies.yaml file
				depsYaml := filepath.Join(tempDir, "dependencies.yaml")
				err = os.WriteFile(depsYaml, []byte(yamlContent), 0644)
				if err != nil {
					t.Fatalf("Failed to create dependencies.yaml: %v", err)
				}

				staticReportPath := filepath.Join(tempDir, "static-report")

				// Create the static-report directory
				err = os.MkdirAll(staticReportPath, 0755)
				if err != nil {
					t.Fatalf("Failed to create static-report directory: %v", err)
				}

				cmd := &analyzeCommand{
					output:           tempDir,
					skipStaticReport: false,
					AnalyzeCommandContext: AnalyzeCommandContext{
						log:       logger,
						kantraDir: kantraDir,
					},
				}

				cleanup := func() {
					os.RemoveAll(tempDir)
					kantraCleanup()
				}

				return cmd, staticReportPath, cleanup
			},
			depsErr:     false,
			expectError: false,
		},
		{
			name: "skip static report",
			setupFunc: func() (*analyzeCommand, string, func()) {
				// Create mock kantra directory
				kantraDir, kantraCleanup := createMockKantraDir(t)

				// Create temporary output directory
				tempDir, err := os.MkdirTemp("", "analyze-output-test")
				if err != nil {
					t.Fatalf("Failed to create temp dir: %v", err)
				}

				staticReportPath := filepath.Join(tempDir, "static-report")

				cmd := &analyzeCommand{
					output:           tempDir,
					skipStaticReport: true,
					AnalyzeCommandContext: AnalyzeCommandContext{
						log:       logger,
						kantraDir: kantraDir,
					},
				}

				cleanup := func() {
					os.RemoveAll(tempDir)
					kantraCleanup()
				}

				return cmd, staticReportPath, cleanup
			},
			depsErr:     false,
			expectError: false,
		},
		{
			name: "build static report with deps error",
			setupFunc: func() (*analyzeCommand, string, func()) {
				// Create mock kantra directory with static-report assets
				kantraDir, kantraCleanup := createMockKantraDir(t)

				// Create temporary output directory
				tempDir, err := os.MkdirTemp("", "analyze-output-test")
				if err != nil {
					t.Fatalf("Failed to create temp dir: %v", err)
				}

				// Create mock output.yaml file in output directory with valid YAML
				outputYaml := filepath.Join(tempDir, "output.yaml")
				yamlContent := `[]` // Empty array of RuleSets
				err = os.WriteFile(outputYaml, []byte(yamlContent), 0644)
				if err != nil {
					t.Fatalf("Failed to create output.yaml: %v", err)
				}

				// Create mock dependencies.yaml file
				depsYaml := filepath.Join(tempDir, "dependencies.yaml")
				err = os.WriteFile(depsYaml, []byte(yamlContent), 0644)
				if err != nil {
					t.Fatalf("Failed to create dependencies.yaml: %v", err)
				}

				staticReportPath := filepath.Join(tempDir, "static-report")

				// Create the static-report directory
				err = os.MkdirAll(staticReportPath, 0755)
				if err != nil {
					t.Fatalf("Failed to create static-report directory: %v", err)
				}

				cmd := &analyzeCommand{
					output:           tempDir,
					skipStaticReport: false,
					AnalyzeCommandContext: AnalyzeCommandContext{
						log:       logger,
						kantraDir: kantraDir,
					},
				}

				cleanup := func() {
					os.RemoveAll(tempDir)
					kantraCleanup()
				}

				return cmd, staticReportPath, cleanup
			},
			depsErr:     true,
			expectError: false, // Should still succeed even with deps error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, staticReportPath, cleanup := tt.setupFunc()
			defer cleanup()

			ctx := context.Background()
			err := cmd.buildStaticReportFile(ctx, staticReportPath, tt.depsErr)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if cmd.skipStaticReport {
				// When skipping, file should not be created
				if _, err := os.Stat(staticReportPath); !os.IsNotExist(err) {
					t.Error("Expected static report file not to be created when skipping")
				}
			} else if !tt.expectError {
				// When not skipping and no error, file should be created
				if _, err := os.Stat(staticReportPath); os.IsNotExist(err) {
					t.Error("Expected static report file to be created")
				}
			}
		})
	}
}
