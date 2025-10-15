package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	kantraProvider "github.com/konveyor-ecosystem/kantra/pkg/provider"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Use logr.Discard() for testing - it's the standard no-op logger

func TestGradleSourcesTaskFileConfiguration(t *testing.T) {
	a := analyzeCommand{}
	a.AnalyzeCommandContext.kantraDir = "kantraDir"
	configs, err := a.createProviderConfigsContainerless([]interface{}{})
	if err != nil {
		t.Fail()
	}

	assert.NotEmpty(t, configs)
	assert.Equal(t, configs[0].InitConfig[0].ProviderSpecificConfig["gradleSourcesTaskFile"], "kantraDir/task.gradle")
}

func TestMakeBuiltinProviderConfig(t *testing.T) {
	tests := []struct {
		name                string
		input               string
		mode                string
		excludedTargetPaths []interface{}
		expectedName        string
		expectedLocation    string
		expectedMode        provider.AnalysisMode
		expectExcludedDirs  bool
	}{
		{
			name:                "basic builtin config",
			input:               "/test/input",
			mode:                "full",
			excludedTargetPaths: nil,
			expectedName:        "builtin",
			expectedLocation:    "/test/input",
			expectedMode:        provider.AnalysisMode("full"),
			expectExcludedDirs:  false,
		},
		{
			name:                "builtin config with excluded paths",
			input:               "/test/input",
			mode:                "source-only",
			excludedTargetPaths: []interface{}{"target", "build"},
			expectedName:        "builtin",
			expectedLocation:    "/test/input",
			expectedMode:        provider.AnalysisMode("source-only"),
			expectExcludedDirs:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := analyzeCommand{
				input: tt.input,
				mode:  tt.mode,
			}

			config := a.makeBuiltinProviderConfig(tt.excludedTargetPaths)

			assert.Equal(t, tt.expectedName, config.Name)
			require.Len(t, config.InitConfig, 1)
			assert.Equal(t, tt.expectedLocation, config.InitConfig[0].Location)
			assert.Equal(t, tt.expectedMode, config.InitConfig[0].AnalysisMode)

			if tt.expectExcludedDirs {
				excludedDirs, exists := config.InitConfig[0].ProviderSpecificConfig["excludedDirs"]
				assert.True(t, exists)
				assert.Equal(t, tt.excludedTargetPaths, excludedDirs)
			} else {
				_, exists := config.InitConfig[0].ProviderSpecificConfig["excludedDirs"]
				assert.False(t, exists)
			}
		})
	}
}

func TestMakeJavaProviderConfig(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		mode               string
		kantraDir          string
		mavenSettingsFile  string
		jvmMaxMem          string
		disableMavenSearch bool
		reqMap             map[string]string
	}{
		{
			name:      "basic java config",
			input:     "/test/input",
			mode:      "full",
			kantraDir: "/test/kantra",
			reqMap: map[string]string{
				"jdtls":  "/test/jdtls",
				"bundle": "/test/bundle",
			},
		},
		{
			name:               "java config with maven settings",
			input:              "/test/input",
			mode:               "source-only",
			kantraDir:          "/test/kantra",
			mavenSettingsFile:  "/test/settings.xml",
			jvmMaxMem:          "4g",
			disableMavenSearch: true,
			reqMap: map[string]string{
				"jdtls":  "/test/jdtls",
				"bundle": "/test/bundle",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.jvmMaxMem != "" {
				Settings.JvmMaxMem = tt.jvmMaxMem
				defer func() { Settings.JvmMaxMem = "" }() // cleanup
			}

			a := analyzeCommand{
				input:              tt.input,
				mode:               tt.mode,
				mavenSettingsFile:  tt.mavenSettingsFile,
				disableMavenSearch: tt.disableMavenSearch,
			}
			a.AnalyzeCommandContext.kantraDir = tt.kantraDir
			a.AnalyzeCommandContext.reqMap = tt.reqMap

			config := a.makeJavaProviderConfig()

			assert.Equal(t, "java", config.Name)
			assert.Equal(t, tt.reqMap["jdtls"], config.BinaryPath)
			require.Len(t, config.InitConfig, 1)

			initConfig := config.InitConfig[0]
			assert.Equal(t, tt.input, initConfig.Location)
			assert.Equal(t, provider.AnalysisMode(tt.mode), initConfig.AnalysisMode)

			psc := initConfig.ProviderSpecificConfig
			assert.Equal(t, true, psc["cleanExplodedBin"])
			assert.Equal(t, "/test/kantra/fernflower.jar", psc["fernFlowerPath"])
			assert.Equal(t, "java", psc["lspServerName"])
			assert.Equal(t, tt.reqMap["bundle"], psc["bundles"])
			assert.Equal(t, tt.reqMap["jdtls"], psc["lspServerPath"])
			assert.Equal(t, "/test/kantra/maven.default.index", psc["depOpenSourceLabelsFile"])
			assert.Equal(t, tt.disableMavenSearch, psc["disableMavenSearch"])
			assert.Equal(t, "/test/kantra/task.gradle", psc["gradleSourcesTaskFile"])

			if tt.mavenSettingsFile != "" {
				assert.Equal(t, tt.mavenSettingsFile, psc["mavenSettingsFile"])
			} else {
				_, exists := psc["mavenSettingsFile"]
				assert.False(t, exists)
			}

			if tt.jvmMaxMem != "" {
				assert.Equal(t, tt.jvmMaxMem, psc["jvmMaxMem"])
			} else {
				_, exists := psc["jvmMaxMem"]
				assert.False(t, exists)
			}
		})
	}
}

func TestWalkJavaPathForTarget(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "walk-java-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name          string
		isFileInput   bool
		setupDirs     []string
		setupFiles    []string
		expectedPaths []string
		expectError   bool
		errorContains string
	}{
		{
			name:          "directory with no target folders",
			isFileInput:   false,
			setupDirs:     []string{"src/main/java", "src/test/java"},
			setupFiles:    []string{"pom.xml", "src/main/java/App.java"},
			expectedPaths: []string{},
			expectError:   false,
		},
		{
			name:          "directory with single target folder",
			isFileInput:   false,
			setupDirs:     []string{"src/main/java", "target", "target/classes"},
			setupFiles:    []string{"pom.xml", "src/main/java/App.java"},
			expectedPaths: []string{"target"},
			expectError:   false,
		},
		{
			name:        "directory with multiple target folders",
			isFileInput: false,
			setupDirs: []string{
				"module1/src/main/java", "module1/target", "module1/target/classes",
				"module2/src/main/java", "module2/target", "module2/target/classes",
				"nested/module3/target",
			},
			setupFiles: []string{
				"pom.xml", "module1/pom.xml", "module2/pom.xml",
				"module1/src/main/java/App1.java", "module2/src/main/java/App2.java",
			},
			expectedPaths: []string{"module1/target", "module2/target", "nested/module3/target"},
			expectError:   false,
		},
		{
			name:        "nested target folders",
			isFileInput: false,
			setupDirs: []string{
				"parent/target", "parent/target/classes",
				"parent/child/target", "parent/child/target/classes",
			},
			setupFiles:    []string{"parent/pom.xml", "parent/child/pom.xml"},
			expectedPaths: []string{"parent/target", "parent/child/target"},
			expectError:   false,
		},
		{
			name:          "target as file not directory",
			isFileInput:   false,
			setupDirs:     []string{"src/main/java"},
			setupFiles:    []string{"pom.xml", "target", "src/main/java/App.java"},
			expectedPaths: []string{},
			expectError:   false,
		},
		{
			name:          "empty directory",
			isFileInput:   false,
			setupDirs:     []string{},
			setupFiles:    []string{},
			expectedPaths: []string{},
			expectError:   false,
		},
		{
			name:          "non-existent directory",
			isFileInput:   false,
			setupDirs:     []string{},
			setupFiles:    []string{},
			expectedPaths: nil,
			expectError:   true,
			errorContains: "no such file or directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := filepath.Join(tempDir, tt.name)
			err := os.MkdirAll(testDir, 0755)
			require.NoError(t, err)

			for _, dir := range tt.setupDirs {
				err := os.MkdirAll(filepath.Join(testDir, dir), 0755)
				require.NoError(t, err)
			}

			for _, file := range tt.setupFiles {
				filePath := filepath.Join(testDir, file)
				err := os.MkdirAll(filepath.Dir(filePath), 0755)
				require.NoError(t, err)
				f, err := os.Create(filePath)
				require.NoError(t, err)
				f.Close()
			}

			inputPath := testDir
			if tt.expectError && tt.errorContains == "no such file or directory" {
				inputPath = filepath.Join(testDir, "non-existent")
			}

			result, err := kantraProvider.WalkJavaPathForTarget(logr.Discard(), tt.isFileInput, inputPath)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			require.NoError(t, err)

			var resultPaths []string
			for _, path := range result {
				relPath, err := filepath.Rel(testDir, path.(string))
				require.NoError(t, err)
				resultPaths = append(resultPaths, relPath)
			}

			assert.ElementsMatch(t, tt.expectedPaths, resultPaths)
		})
	}
}

func TestWalkJavaPathForTargetFileInput(t *testing.T) {
	t.Run("file input with non-existent file", func(t *testing.T) {
		nonExistentFile := "/path/to/non/existent/file.jar"

		result, err := kantraProvider.WalkJavaPathForTarget(logr.Discard(), true, nonExistentFile)

		// Should return an error since the file doesn't exist
		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestWalkJavaPathForTargetIntegration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "java-project-integration-")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a realistic Maven multi-module project structure
	projectStructure := map[string][]string{
		"dirs": {
			"parent-project",
			"parent-project/module-a/src/main/java/com/example",
			"parent-project/module-a/src/test/java/com/example",
			"parent-project/module-a/target/classes/com/example",
			"parent-project/module-a/target/test-classes",
			"parent-project/module-b/src/main/java/com/example",
			"parent-project/module-b/target/classes",
			"parent-project/target/site",
		},
		"files": {
			"parent-project/pom.xml",
			"parent-project/module-a/pom.xml",
			"parent-project/module-a/src/main/java/com/example/AppA.java",
			"parent-project/module-a/src/test/java/com/example/AppATest.java",
			"parent-project/module-b/pom.xml",
			"parent-project/module-b/src/main/java/com/example/AppB.java",
		},
	}

	for _, dir := range projectStructure["dirs"] {
		err := os.MkdirAll(filepath.Join(tempDir, dir), 0755)
		require.NoError(t, err)
	}

	for _, file := range projectStructure["files"] {
		filePath := filepath.Join(tempDir, file)
		f, err := os.Create(filePath)
		require.NoError(t, err)
		f.Close()
	}

	projectRoot := filepath.Join(tempDir, "parent-project")
	result, err := kantraProvider.WalkJavaPathForTarget(logr.Discard(), false, projectRoot)

	require.NoError(t, err)
	require.NotNil(t, result)

	var relativePaths []string
	for _, path := range result {
		relPath, err := filepath.Rel(projectRoot, path.(string))
		require.NoError(t, err)
		relativePaths = append(relativePaths, relPath)
	}

	expectedPaths := []string{
		"module-a/target",
		"module-b/target",
		"target",
	}

	assert.ElementsMatch(t, expectedPaths, relativePaths)
}

func TestListLabelsContainerless(t *testing.T) {
	log := logr.Discard()

	tests := []struct {
		name           string
		listSources    bool
		listTargets    bool
		setupRules     map[string]string
		expectedOutput []string
		expectError    bool
		errorContains  string
	}{
		{
			name:        "list targets with valid rule files",
			listTargets: true,
			setupRules: map[string]string{
				"rules/java-rules.yaml": `
- category: mandatory
  labels:
  - konveyor.io/target=java
  - konveyor.io/target=jakarta-ee
  ruleID: java-01
  description: Test rule
`,
				"rules/spring-rules.yaml": `
- category: optional
  labels:
  - konveyor.io/target=spring
  - konveyor.io/target=spring-boot
  ruleID: spring-01
`,
			},
			expectedOutput: []string{
				"available target technologies:",
				"jakarta-ee",
				"java",
				"spring",
				"spring-boot",
			},
			expectError: false,
		},
		{
			name:        "list sources with valid rule files",
			listSources: true,
			setupRules: map[string]string{
				"rules/migration-rules.yaml": `
- category: mandatory
  labels:
  - konveyor.io/source=jboss-eap
  - konveyor.io/source=weblogic
  ruleID: migration-01
`,
			},
			expectedOutput: []string{
				"available source technologies:",
				"jboss-eap",
				"weblogic",
			},
			expectError: false,
		},
		{
			name:        "list targets with mixed labels",
			listTargets: true,
			setupRules: map[string]string{
				"rules/mixed-rules.yaml": `
- category: mandatory
  labels:
  - konveyor.io/target=kubernetes
  - konveyor.io/source=legacy-app
  - konveyor.io/target=openshift
  ruleID: mixed-01
`,
			},
			expectedOutput: []string{
				"available target technologies:",
				"kubernetes",
				"openshift",
			},
			expectError: false,
		},
		{
			name:        "list sources with no matching labels",
			listSources: true,
			setupRules: map[string]string{
				"rules/no-source-rules.yaml": `
- category: mandatory
  labels:
  - konveyor.io/target=java
  - some.other/label=value
  ruleID: target-only-01
`,
			},
			expectedOutput: []string{
				"available source technologies:",
			},
			expectError: false,
		},
		{
			name:        "empty rule directory",
			listTargets: true,
			setupRules:  map[string]string{},
			expectedOutput: []string{
				"available target technologies:",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary kantra directory structure
			tmpKantraDir, err := os.MkdirTemp("", "test-kantra-")
			require.NoError(t, err)
			defer os.RemoveAll(tmpKantraDir)

			// Create rulesets directory
			rulesetsDir := filepath.Join(tmpKantraDir, "rulesets")
			err = os.MkdirAll(rulesetsDir, 0755)
			require.NoError(t, err)

			// Setup rule files
			for filePath, content := range tt.setupRules {
				fullPath := filepath.Join(rulesetsDir, filePath)
				err = os.MkdirAll(filepath.Dir(fullPath), 0755)
				require.NoError(t, err)
				err = os.WriteFile(fullPath, []byte(content), 0644)
				require.NoError(t, err)
			}

			// Create analyze command with mocked kantra directory
			a := &analyzeCommand{
				listSources: tt.listSources,
				listTargets: tt.listTargets,
				AnalyzeCommandContext: AnalyzeCommandContext{
					log:       log,
					kantraDir: tmpKantraDir,
				},
			}

			// Capture output
			var output strings.Builder
			err = a.fetchLabelsContainerless(context.Background(), tt.listSources, tt.listTargets, &output)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			require.NoError(t, err)

			// Check output
			outputLines := strings.Split(strings.TrimSpace(output.String()), "\n")
			if len(tt.expectedOutput) == 1 && tt.expectedOutput[0] != "" {
				// Handle case where we expect only the header
				assert.Equal(t, tt.expectedOutput[0], outputLines[0])
			} else if len(tt.expectedOutput) > 1 {
				// Check that we have the expected number of lines
				assert.Len(t, outputLines, len(tt.expectedOutput))
				// Check header line
				assert.Equal(t, tt.expectedOutput[0], outputLines[0])
				// Check that all expected technologies are present (order may vary due to sorting)
				actualTechs := outputLines[1:]
				expectedTechs := tt.expectedOutput[1:]
				assert.ElementsMatch(t, expectedTechs, actualTechs)
			}
		})
	}
}

func TestAnalyzeCommandListTargetsContainerless(t *testing.T) {
	log := logr.Discard()

	tests := []struct {
		name           string
		cmdArgs        []string
		setupRules     map[string]string
		expectError    bool
		expectedOutput []string
	}{
		{
			name:    "list targets with run-local flag",
			cmdArgs: []string{"--list-targets", "--run-local=true"},
			setupRules: map[string]string{
				"rulesets/java/rules.yaml": `
- category: mandatory
  labels:
  - konveyor.io/target=java
  - konveyor.io/target=spring-boot
  ruleID: java-target-01
`,
			},
			expectError: false,
			expectedOutput: []string{
				"available target technologies:",
				"java",
				"spring-boot",
			},
		},
		{
			name:    "list targets with run-local true (default)",
			cmdArgs: []string{"--list-targets"},
			setupRules: map[string]string{
				"rulesets/kubernetes/rules.yaml": `
- category: optional
  labels:
  - konveyor.io/target=kubernetes
  - konveyor.io/target=openshift
  ruleID: k8s-target-01
`,
			},
			expectError: false,
			expectedOutput: []string{
				"available target technologies:",
				"kubernetes",
				"openshift",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary kantra directory structure
			tmpKantraDir, err := os.MkdirTemp("", "test-kantra-integration-")
			require.NoError(t, err)
			defer os.RemoveAll(tmpKantraDir)

			// Setup rule files
			for filePath, content := range tt.setupRules {
				fullPath := filepath.Join(tmpKantraDir, filePath)
				err = os.MkdirAll(filepath.Dir(fullPath), 0755)
				require.NoError(t, err)
				err = os.WriteFile(fullPath, []byte(content), 0644)
				require.NoError(t, err)
			}

			// Create analyze command
			a := &analyzeCommand{
				AnalyzeCommandContext: AnalyzeCommandContext{
					log:       log,
					kantraDir: tmpKantraDir,
				},
			}

			// Parse flags to set the command options
			for _, arg := range tt.cmdArgs {
				switch {
				case arg == "--list-targets":
					a.listTargets = true
				case arg == "--list-sources":
					a.listSources = true
				case arg == "--run-local=true":
					a.runLocal = true
				case arg == "--run-local=false":
					a.runLocal = false
				}
			}

			// Set default for runLocal if not specified (matches the actual default)
			if !sliceContains(tt.cmdArgs, "--run-local=false") {
				a.runLocal = true
			}

			// Capture output
			var output strings.Builder
			err = a.fetchLabelsContainerless(context.Background(), a.listSources, a.listTargets, &output)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Check output
			outputLines := strings.Split(strings.TrimSpace(output.String()), "\n")
			if len(tt.expectedOutput) > 0 {
				assert.Equal(t, tt.expectedOutput[0], outputLines[0])
				if len(tt.expectedOutput) > 1 {
					actualTechs := outputLines[1:]
					expectedTechs := tt.expectedOutput[1:]
					assert.ElementsMatch(t, expectedTechs, actualTechs)
				}
			}
		})
	}
}

func TestAnalyzeCommandListSourcesContainerless(t *testing.T) {
	log := logr.Discard()

	tests := []struct {
		name           string
		cmdArgs        []string
		setupRules     map[string]string
		expectError    bool
		expectedOutput []string
	}{
		{
			name:    "list sources with run-local flag",
			cmdArgs: []string{"--list-sources", "--run-local=true"},
			setupRules: map[string]string{
				"rulesets/migration/rules.yaml": `
- category: mandatory
  labels:
  - konveyor.io/source=jboss-eap
  - konveyor.io/source=weblogic
  ruleID: migration-source-01
`,
			},
			expectError: false,
			expectedOutput: []string{
				"available source technologies:",
				"jboss-eap",
				"weblogic",
			},
		},
		{
			name:    "list sources with run-local true (default)",
			cmdArgs: []string{"--list-sources"},
			setupRules: map[string]string{
				"rulesets/legacy/rules.yaml": `
- category: optional
  labels:
  - konveyor.io/source=legacy-app
  - konveyor.io/source=tomcat
  ruleID: legacy-source-01
`,
			},
			expectError: false,
			expectedOutput: []string{
				"available source technologies:",
				"legacy-app",
				"tomcat",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary kantra directory structure
			tmpKantraDir, err := os.MkdirTemp("", "test-kantra-integration-")
			require.NoError(t, err)
			defer os.RemoveAll(tmpKantraDir)

			// Setup rule files
			for filePath, content := range tt.setupRules {
				fullPath := filepath.Join(tmpKantraDir, filePath)
				err = os.MkdirAll(filepath.Dir(fullPath), 0755)
				require.NoError(t, err)
				err = os.WriteFile(fullPath, []byte(content), 0644)
				require.NoError(t, err)
			}

			// Create analyze command
			a := &analyzeCommand{
				AnalyzeCommandContext: AnalyzeCommandContext{
					log:       log,
					kantraDir: tmpKantraDir,
				},
			}

			// Parse flags to set the command options
			for _, arg := range tt.cmdArgs {
				switch {
				case arg == "--list-targets":
					a.listTargets = true
				case arg == "--list-sources":
					a.listSources = true
				case arg == "--run-local=true":
					a.runLocal = true
				case arg == "--run-local=false":
					a.runLocal = false
				}
			}

			// Set default for runLocal if not specified (matches the actual default)
			if !sliceContains(tt.cmdArgs, "--run-local=false") {
				a.runLocal = true
			}

			// Capture output
			var output strings.Builder
			err = a.fetchLabelsContainerless(context.Background(), a.listSources, a.listTargets, &output)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Check output
			outputLines := strings.Split(strings.TrimSpace(output.String()), "\n")
			if len(tt.expectedOutput) > 0 {
				assert.Equal(t, tt.expectedOutput[0], outputLines[0])
				if len(tt.expectedOutput) > 1 {
					actualTechs := outputLines[1:]
					expectedTechs := tt.expectedOutput[1:]
					assert.ElementsMatch(t, expectedTechs, actualTechs)
				}
			}
		})
	}
}

func TestListLabelsContainerlessErrorHandling(t *testing.T) {
	log := logr.Discard()

	tests := []struct {
		name          string
		listSources   bool
		listTargets   bool
		setupKantra   bool
		setupRulesets bool
		expectError   bool
		errorContains string
	}{
		{
			name:          "missing kantra directory should fail",
			listTargets:   true,
			setupKantra:   false,
			setupRulesets: false,
			expectError:   true,
			errorContains: "no such file or directory",
		},
		{
			name:          "missing rulesets directory should fail",
			listTargets:   true,
			setupKantra:   true,
			setupRulesets: false,
			expectError:   true,
			errorContains: "no such file or directory",
		},
		{
			name:          "empty rulesets directory should not fail",
			listTargets:   true,
			setupKantra:   true,
			setupRulesets: true,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tmpKantraDir string
			var err error

			if tt.setupKantra {
				tmpKantraDir, err = os.MkdirTemp("", "test-kantra-error-")
				require.NoError(t, err)
				defer os.RemoveAll(tmpKantraDir)

				if tt.setupRulesets {
					rulesetsDir := filepath.Join(tmpKantraDir, "rulesets")
					err = os.MkdirAll(rulesetsDir, 0755)
					require.NoError(t, err)
				}
			} else {
				tmpKantraDir = "/non/existent/kantra/directory"
			}

			// Create analyze command with potentially missing kantra directory
			a := &analyzeCommand{
				listSources: tt.listSources,
				listTargets: tt.listTargets,
				AnalyzeCommandContext: AnalyzeCommandContext{
					log:       log,
					kantraDir: tmpKantraDir,
				},
			}

			// Capture output
			var output strings.Builder
			err = a.fetchLabelsContainerless(context.Background(), tt.listSources, tt.listTargets, &output)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestListLabelsContainerlessOutputFormat(t *testing.T) {
	log := logr.Discard()

	tests := []struct {
		name           string
		listSources    bool
		listTargets    bool
		setupRules     map[string]string
		expectedHeader string
		expectedCount  int
	}{
		{
			name:        "targets output format verification",
			listTargets: true,
			setupRules: map[string]string{
				"rulesets/test-rules.yaml": `
- category: mandatory
  labels:
  - konveyor.io/target=java
  - konveyor.io/target=spring
  ruleID: test-01
`,
			},
			expectedHeader: "available target technologies:",
			expectedCount:  3, // header + 2 technologies
		},
		{
			name:        "sources output format verification",
			listSources: true,
			setupRules: map[string]string{
				"rulesets/test-rules.yaml": `
- category: mandatory
  labels:
  - konveyor.io/source=legacy
  - konveyor.io/source=monolith
  ruleID: test-01
`,
			},
			expectedHeader: "available source technologies:",
			expectedCount:  3, // header + 2 technologies
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary kantra directory structure
			tmpKantraDir, err := os.MkdirTemp("", "test-kantra-format-")
			require.NoError(t, err)
			defer os.RemoveAll(tmpKantraDir)

			// Setup rule files
			for filePath, content := range tt.setupRules {
				fullPath := filepath.Join(tmpKantraDir, filePath)
				err = os.MkdirAll(filepath.Dir(fullPath), 0755)
				require.NoError(t, err)
				err = os.WriteFile(fullPath, []byte(content), 0644)
				require.NoError(t, err)
			}

			// Create analyze command
			a := &analyzeCommand{
				listSources: tt.listSources,
				listTargets: tt.listTargets,
				AnalyzeCommandContext: AnalyzeCommandContext{
					log:       log,
					kantraDir: tmpKantraDir,
				},
			}

			// Capture output
			var output strings.Builder
			err = a.fetchLabelsContainerless(context.Background(), tt.listSources, tt.listTargets, &output)
			require.NoError(t, err)

			// Verify output format
			outputLines := strings.Split(strings.TrimSpace(output.String()), "\n")
			assert.Len(t, outputLines, tt.expectedCount)
			assert.Equal(t, tt.expectedHeader, outputLines[0])

			// Verify technologies are sorted (ListOptionsFromLabels sorts them)
			if len(outputLines) > 1 {
				techs := outputLines[1:]
				for i := 1; i < len(techs); i++ {
					assert.True(t, techs[i-1] <= techs[i], "Technologies should be sorted: %v", techs)
				}
			}
		})
	}
}

func TestValidateContainerlessInput(t *testing.T) {
	log := logr.Discard()

	currentDir, dirErr := os.Getwd()
	require.NoError(t, dirErr)

	tempDir, err := os.MkdirTemp("", "test-input-")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	tmpKantraDir, err := os.MkdirTemp("", "test-kantra-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpKantraDir)

	requiredDirs := []string{
		"rulesets",
		"jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target",
		"jdtls/bin",
	}
	for _, dir := range requiredDirs {
		err = os.MkdirAll(filepath.Join(tmpKantraDir, dir), 0755)
		require.NoError(t, err)
	}

	requiredFiles := []string{
		"fernflower.jar",
		"jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar",
		"jdtls/bin/jdtls",
	}
	for _, file := range requiredFiles {
		f, err := os.Create(filepath.Join(tmpKantraDir, file))
		require.NoError(t, err)
		f.Close()
	}

	tests := []struct {
		name        string
		input       string
		expectError bool
		errorMsg    string
		setupFunc   func() string
		getwdSetup  func(*testing.T)
		skipCheck   func() bool
	}{
		{
			name:        "absolute path to current directory should fail",
			expectError: true,
			errorMsg:    "cannot be the current directory",
			setupFunc: func() string {
				return currentDir
			},
		},
		{
			name:        "relative path '.' should fail",
			expectError: true,
			errorMsg:    "cannot be the current directory",
			setupFunc: func() string {
				return "."
			},
		},
		{
			name:        "relative path './' should fail",
			expectError: true,
			errorMsg:    "cannot be the current directory",
			setupFunc: func() string {
				return "./"
			},
		},
		{
			name:        "absolute path to different directory should pass",
			expectError: false,
			setupFunc: func() string {
				return tempDir
			},
		},
		{
			name:        "relative path to subdirectory should pass",
			expectError: false,
			setupFunc: func() string {
				subDir := "test-subdir"
				fullPath := filepath.Join(currentDir, subDir)
				os.MkdirAll(fullPath, 0755)
				return subDir
			},
		},
		{
			name:        "test Getwd() error",
			expectError: true,
			errorMsg:    "no such file or directory",
			setupFunc: func() string {
				return "/some/valid/path"
			},
			getwdSetup: func(t *testing.T) {
				originalDir, err := os.Getwd()
				require.NoError(t, err)
				t.Cleanup(func() {
					os.Chdir(originalDir)
				})
				tempTestDir, err := os.MkdirTemp("", "test-getwd-error-")
				require.NoError(t, err)

				err = os.Chdir(tempTestDir)
				require.NoError(t, err)

				err = os.Remove(tempTestDir)
				require.NoError(t, err)
			},
			skipCheck: func() bool {
				_, err := os.Getwd()
				return err == nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.getwdSetup != nil {
				tt.getwdSetup(t)
			}
			if tt.skipCheck != nil && tt.skipCheck() {
				t.Skipf("Skipping test %s", tt.name)
				return
			}

			inputPath := tt.setupFunc()
			a := &analyzeCommand{
				input: inputPath,
				AnalyzeCommandContext: AnalyzeCommandContext{
					log:       log,
					kantraDir: tmpKantraDir,
				},
			}

			if tt.getwdSetup == nil {
				if absPath, err := filepath.Abs(a.input); err == nil {
					a.input = absPath
				}
			}

			err := a.ValidateContainerless(context.Background())
			if tt.expectError {
				require.Error(t, err)
				if tt.getwdSetup != nil {
					assert.Error(t, err)
				} else {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				if err != nil {
					assert.NotContains(t, err.Error(), "cannot be the current directory")
				}
			}
		})
	}
}

func sliceContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
