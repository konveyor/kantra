package cmd

import (
	"os"
	"path/filepath"
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
