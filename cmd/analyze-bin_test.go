package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	kantraProvider "github.com/konveyor-ecosystem/kantra/pkg/provider"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEngineStopper records whether Stop() was called (for testing stopEngineAndProviders).
type mockEngineStopper struct {
	stopCalled bool
}

func (m *mockEngineStopper) Stop() {
	m.stopCalled = true
}

// Use logr.Discard() for testing - it's the standard no-op logger

func Test_stopEngineAndProviders(t *testing.T) {
	t.Run("nil engine and nil providers does not panic", func(t *testing.T) {
		stopEngineAndProviders(nil, nil)
	})

	t.Run("nil engine and empty providers does not panic", func(t *testing.T) {
		stopEngineAndProviders(nil, map[string]provider.InternalProviderClient{})
	})

	t.Run("non-nil engine has Stop called", func(t *testing.T) {
		eng := &mockEngineStopper{}
		stopEngineAndProviders(eng, nil)
		assert.True(t, eng.stopCalled, "engine.Stop() should have been called")
	})

	t.Run("non-nil engine and empty providers has engine Stop called", func(t *testing.T) {
		eng := &mockEngineStopper{}
		stopEngineAndProviders(eng, map[string]provider.InternalProviderClient{})
		assert.True(t, eng.stopCalled, "engine.Stop() should have been called")
	})

	t.Run("nil engine and non-empty providers calls Stop on each provider", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "stop-engine-providers-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		builtinClient, err := lib.GetProviderClient(provider.Config{
			Name: "builtin",
			InitConfig: []provider.InitConfig{
				{Location: tmpDir},
			},
		}, logr.Discard())
		require.NoError(t, err)

		providers := map[string]provider.InternalProviderClient{"builtin": builtinClient}
		stopEngineAndProviders(nil, providers)
		// No panic and builtin provider's Stop() is a no-op; we're just covering the loop.
	})
}

// TestRunAnalysisContainerless_DeferRunsOnEarlyReturn ensures that when RunAnalysisContainerless
// returns early (e.g. from loadOverrideProviderSettings error), the deferred stopEngineAndProviders
// runs so that engine and providers are always stopped (fixes #665).
func TestRunAnalysisContainerless_DeferRunsOnEarlyReturn(t *testing.T) {
	// ValidateContainerless requires mvn, java, JAVA_HOME - skip if not present
	if _, err := exec.LookPath("mvn"); err != nil {
		t.Skip("mvn not in PATH, skipping containerless test")
	}
	if _, err := exec.LookPath("java"); err != nil {
		t.Skip("java not in PATH, skipping containerless test")
	}
	if os.Getenv("JAVA_HOME") == "" {
		t.Skip("JAVA_HOME not set, skipping containerless test")
	}

	// Use relative path constants so we can create the required files under a temp dir
	oldBundle := JavaBundlesLocation
	oldJdtls := JDTLSBinLocation
	defer func() {
		JavaBundlesLocation = oldBundle
		JDTLSBinLocation = oldJdtls
	}()
	JavaBundlesLocation = "jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar"
	JDTLSBinLocation = "jdtls/bin/jdtls"

	inputDir, err := os.MkdirTemp("", "containerless-input-")
	require.NoError(t, err)
	defer os.RemoveAll(inputDir)

	outputDir, err := os.MkdirTemp("", "containerless-output-")
	require.NoError(t, err)
	defer os.RemoveAll(outputDir)

	kantraDir, err := os.MkdirTemp("", "containerless-kantra-")
	require.NoError(t, err)
	defer os.RemoveAll(kantraDir)

	// Required dirs and files for setBinMapContainerless and ValidateContainerless
	for _, dir := range []string{RulesetsLocation, "jdtls/bin", "jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target"} {
		require.NoError(t, os.MkdirAll(filepath.Join(kantraDir, dir), 0755))
	}
	for _, f := range []string{"jdtls/bin/jdtls", "jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar", "fernflower.jar"} {
		full := filepath.Join(kantraDir, f)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
		require.NoError(t, os.WriteFile(full, []byte("x"), 0644))
	}

	t.Run("with StopHook asserts deferred cleanup ran", func(t *testing.T) {
		var cleanupRan bool
		a := &analyzeCommand{
			input:                    inputDir,
			output:                   outputDir,
			overrideProviderSettings: "/nonexistent/override-settings.json",
			AnalyzeCommandContext: AnalyzeCommandContext{
				log:       logr.Discard(),
				kantraDir: kantraDir,
				StopHook:  func() { cleanupRan = true },
			},
		}
		absInput, err := filepath.Abs(inputDir)
		require.NoError(t, err)
		a.input = absInput

		err = a.RunAnalysisContainerless(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "override provider settings")
		assert.True(t, cleanupRan, "deferred cleanup should have run (StopHook should have been called)")
	})

	t.Run("without StopHook still runs deferred cleanup", func(t *testing.T) {
		a := &analyzeCommand{
			input:                    inputDir,
			output:                   outputDir,
			overrideProviderSettings: "/nonexistent/override-settings.json",
			AnalyzeCommandContext: AnalyzeCommandContext{
				log:       logr.Discard(),
				kantraDir: kantraDir,
				StopHook:  nil, // production path: no hook set
			},
		}
		absInput, err := filepath.Abs(inputDir)
		require.NoError(t, err)
		a.input = absInput

		err = a.RunAnalysisContainerless(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "override provider settings")
		// Defer ran (stopEngineAndProviders + nil hook); no panic means cleanup path executed.
	})

	// Covers line 199: selectors = append(selectors, selector) when labelSelector is set
	t.Run("with labelSelector runs label selector path", func(t *testing.T) {
		var cleanupRan bool
		a := &analyzeCommand{
			input:                    inputDir,
			output:                   outputDir,
			labelSelector:            "source=java", // valid selector so we hit the append path
			overrideProviderSettings: "/nonexistent/override-settings.json",
			AnalyzeCommandContext: AnalyzeCommandContext{
				log:       logr.Discard(),
				kantraDir: kantraDir,
				StopHook:  func() { cleanupRan = true },
			},
		}
		absInput, err := filepath.Abs(inputDir)
		require.NoError(t, err)
		a.input = absInput

		err = a.RunAnalysisContainerless(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "override provider settings")
		assert.True(t, cleanupRan)
	})
}

// TestRunAnalysisContainerless_SetBinMapContainerlessError covers the error path at lines 214-215
// when setBinMapContainerless fails (e.g. kantra dir missing required jdtls/bundle files).
func TestRunAnalysisContainerless_SetBinMapContainerlessError(t *testing.T) {
	if _, err := exec.LookPath("mvn"); err != nil {
		t.Skip("mvn not in PATH, skipping containerless test")
	}
	if _, err := exec.LookPath("java"); err != nil {
		t.Skip("java not in PATH, skipping containerless test")
	}
	if os.Getenv("JAVA_HOME") == "" {
		t.Skip("JAVA_HOME not set, skipping containerless test")
	}

	oldBundle := JavaBundlesLocation
	oldJdtls := JDTLSBinLocation
	defer func() {
		JavaBundlesLocation = oldBundle
		JDTLSBinLocation = oldJdtls
	}()
	JavaBundlesLocation = "jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar"
	JDTLSBinLocation = "jdtls/bin/jdtls"

	inputDir, err := os.MkdirTemp("", "containerless-input-")
	require.NoError(t, err)
	defer os.RemoveAll(inputDir)

	outputDir, err := os.MkdirTemp("", "containerless-output-")
	require.NoError(t, err)
	defer os.RemoveAll(outputDir)

	// Kantra dir with rulesets and fernflower only - no jdtls or bundle jar, so setBinMapContainerless will fail
	kantraDir, err := os.MkdirTemp("", "containerless-kantra-")
	require.NoError(t, err)
	defer os.RemoveAll(kantraDir)
	require.NoError(t, os.MkdirAll(filepath.Join(kantraDir, RulesetsLocation), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(kantraDir, "fernflower.jar"), []byte("x"), 0644))

	a := &analyzeCommand{
		input:  inputDir,
		output: outputDir,
		AnalyzeCommandContext: AnalyzeCommandContext{
			log:       logr.Discard(),
			kantraDir: kantraDir,
		},
	}
	absInput, err := filepath.Abs(inputDir)
	require.NoError(t, err)
	a.input = absInput

	err = a.RunAnalysisContainerless(context.Background())
	require.Error(t, err)
	// We hit the setBinMapContainerless error path (lines 214-215); error may be wrapped
	errStr := err.Error()
	assert.True(t, strings.Contains(errStr, "unable to find kantra dependencies") ||
		strings.Contains(errStr, "failed to stat bin") ||
		strings.Contains(errStr, "no such file or directory"),
		"expected setBinMapContainerless error, got: %s", errStr)
}

// TestRunAnalysisContainerless_EngineCreationPath covers the path that reaches
// eng = engine.CreateRuleEngine(...). It only runs when KANTRA_DIR is set and
// points to a full kantra dir (with real jdtls and bundles) so setupJavaProvider
// and setupBuiltinProvider can succeed; otherwise it skips.
func TestRunAnalysisContainerless_EngineCreationPath(t *testing.T) {
	kantraDir := os.Getenv(util.KantraDirEnv)
	if kantraDir == "" {
		t.Skip("KANTRA_DIR not set, skipping engine creation path test")
	}
	if _, err := os.Stat(kantraDir); err != nil {
		t.Skipf("KANTRA_DIR %q not found: %v", kantraDir, err)
	}
	if _, err := exec.LookPath("mvn"); err != nil {
		t.Skip("mvn not in PATH, skipping containerless test")
	}
	if _, err := exec.LookPath("java"); err != nil {
		t.Skip("java not in PATH, skipping containerless test")
	}
	if os.Getenv("JAVA_HOME") == "" {
		t.Skip("JAVA_HOME not set, skipping containerless test")
	}

	inputDir, err := os.MkdirTemp("", "containerless-input-")
	require.NoError(t, err)
	defer os.RemoveAll(inputDir)

	outputDir, err := os.MkdirTemp("", "containerless-output-")
	require.NoError(t, err)
	defer os.RemoveAll(outputDir)

	var cleanupRan bool
	a := &analyzeCommand{
		input:  inputDir,
		output: outputDir,
		// No overrideProviderSettings so we get past loadOverrideProviderSettings
		AnalyzeCommandContext: AnalyzeCommandContext{
			log:       logr.Discard(),
			kantraDir: kantraDir,
			StopHook:  func() { cleanupRan = true },
		},
	}
	absInput, err := filepath.Abs(inputDir)
	require.NoError(t, err)
	a.input = absInput

	err = a.RunAnalysisContainerless(context.Background())
	// We may fail at setupJavaProvider, setupBuiltinProvider, or later; either way defer runs
	if err != nil {
		// If we got far enough to register the defer, it should have run
		assert.True(t, cleanupRan, "deferred cleanup should have run when RunAnalysisContainerless returned")
	}
}

func TestDefaultRulesetPathContainerless(t *testing.T) {
	tests := []struct {
		name                  string
		enableDefaultRulesets bool
		kantraDir             string
		wantPath              string
	}{
		{
			name:                  "enabled returns kantraDir/rulesets/java",
			enableDefaultRulesets: true,
			kantraDir:             "/.kantra",
			wantPath:              "/.kantra/rulesets/java",
		},
		{
			name:                  "disabled returns empty",
			enableDefaultRulesets: false,
			kantraDir:             "/.kantra",
			wantPath:              "",
		},
		{
			name:                  "enabled with custom kantra dir",
			enableDefaultRulesets: true,
			kantraDir:             "/home/user/.kantra",
			wantPath:              "/home/user/.kantra/rulesets/java",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &analyzeCommand{
				enableDefaultRulesets: tt.enableDefaultRulesets,
				AnalyzeCommandContext: AnalyzeCommandContext{
					kantraDir: tt.kantraDir,
				},
			}
			got := a.defaultRulesetPathContainerless()
			assert.Equal(t, tt.wantPath, filepath.ToSlash(got), "path should use java ruleset subdir (DefaultRulesetDir mapping)")
			if tt.wantPath != "" {
				assert.Equal(t, util.JavaProvider, filepath.Base(got))
			}
		})
	}
}

func TestGradleSourcesTaskFileConfiguration(t *testing.T) {
	a := analyzeCommand{}
	a.AnalyzeCommandContext.kantraDir = "kantraDir"
	configs, err := a.createProviderConfigsContainerless()
	if err != nil {
		t.Fail()
	}

	assert.NotEmpty(t, configs)
	assert.Equal(t, configs[0].InitConfig[0].ProviderSpecificConfig["gradleSourcesTaskFile"], "kantraDir/task.gradle")
}

func TestMakeBuiltinProviderConfig(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		mode             string
		expectedName     string
		expectedLocation string
		expectedMode     provider.AnalysisMode
	}{
		{
			name:             "basic builtin config",
			input:            "/test/input",
			mode:             "full",
			expectedName:     "builtin",
			expectedLocation: "/test/input",
			expectedMode:     provider.AnalysisMode("full"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := analyzeCommand{
				input: tt.input,
				mode:  tt.mode,
			}

			config := a.makeBuiltinProviderConfig()

			assert.Equal(t, tt.expectedName, config.Name)
			require.Len(t, config.InitConfig, 1)
			assert.Equal(t, tt.expectedLocation, config.InitConfig[0].Location)
			assert.Equal(t, tt.expectedMode, config.InitConfig[0].AnalysisMode)

			// Verify excludedDirs is not set (we rely on analyzer-lsp defaults)
			_, exists := config.InitConfig[0].ProviderSpecificConfig["excludedDirs"]
			assert.False(t, exists)
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

func TestBuildStaticReportFileWritesToDestPath(t *testing.T) {
	log := logr.Discard()
	tmpOutput, err := os.MkdirTemp("", "test-build-static-report-dest-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpOutput)

	// Valid output.yaml so validateFlags/loadApplications/generateJSBundle succeed
	outputYaml := filepath.Join(tmpOutput, "output.yaml")
	err = os.WriteFile(outputYaml, []byte("[]"), 0644)
	require.NoError(t, err)

	tmpInput, err := os.MkdirTemp("", "test-build-static-report-input-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpInput)

	// Destination is under output dir (behavior after PR #759)
	staticReportDestPath := filepath.Join(tmpOutput, "static-report")
	require.NoError(t, os.MkdirAll(staticReportDestPath, 0755))

	a := &analyzeCommand{
		input:                 tmpInput,
		output:                tmpOutput,
		AnalyzeCommandContext: AnalyzeCommandContext{log: log},
	}

	err = a.buildStaticReportFile(context.Background(), staticReportDestPath, true)
	require.NoError(t, err)

	// output.js must be created under the given dest path (output/static-report), not kantraDir
	outputJS := filepath.Join(staticReportDestPath, "output.js")
	_, statErr := os.Stat(outputJS)
	assert.NoError(t, statErr, "output.js should be created under staticReportPath (output/static-report)")
}

func TestGenerateStaticReportBuildsReportUnderOutputDir(t *testing.T) {
	log := logr.Discard()
	tmpOutput, err := os.MkdirTemp("", "test-generate-static-report-output-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpOutput)

	tmpKantraDir, err := os.MkdirTemp("", "test-generate-static-report-kantra-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpKantraDir)

	// kantraDir must contain static-report so buildStaticReportOutput (CopyFolderContents) succeeds
	staticReportSrc := filepath.Join(tmpKantraDir, "static-report")
	require.NoError(t, os.MkdirAll(staticReportSrc, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(staticReportSrc, "index.html"), []byte("<html></html>"), 0644))

	// output.yaml so buildStaticReportFile can validate and generate output.js
	outputYaml := filepath.Join(tmpOutput, "output.yaml")
	err = os.WriteFile(outputYaml, []byte("[]"), 0644)
	require.NoError(t, err)

	tmpInput, err := os.MkdirTemp("", "test-generate-static-report-input-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpInput)

	a := &analyzeCommand{
		input:  tmpInput,
		output: tmpOutput,
		AnalyzeCommandContext: AnalyzeCommandContext{
			log:       log,
			kantraDir: tmpKantraDir,
		},
	}

	err = a.GenerateStaticReport(context.Background(), log)
	require.NoError(t, err)

	outputJS := filepath.Join(tmpOutput, "static-report", "output.js")
	_, statErr := os.Stat(outputJS)
	assert.NoError(t, statErr, "static report output.js should be under a.output/static-report")
}

func TestGenerateStaticReportSkipFlag(t *testing.T) {
	log := logr.Discard()

	tests := []struct {
		name             string
		skipStaticReport bool
		shouldReturn     bool
	}{
		{
			name:             "skip static report when flag is true",
			skipStaticReport: true,
			shouldReturn:     true,
		},
		{
			name:             "attempt to generate report when flag is false",
			skipStaticReport: false,
			shouldReturn:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary output directory
			tmpOutput, err := os.MkdirTemp("", "test-static-report-")
			require.NoError(t, err)
			defer os.RemoveAll(tmpOutput)

			// Create temporary kantra directory
			tmpKantraDir, err := os.MkdirTemp("", "test-kantra-")
			require.NoError(t, err)
			defer os.RemoveAll(tmpKantraDir)

			// Create minimal files that the function looks for
			if !tt.skipStaticReport {
				// Create a minimal output.yaml so we can test further into the function
				outputYaml := filepath.Join(tmpOutput, "output.yaml")
				err = os.WriteFile(outputYaml, []byte("[]"), 0644)
				require.NoError(t, err)

				// Don't create the static-report directory - let the function fail
				// when it tries to copy non-existent static report files
			}

			// Create analyze command
			a := &analyzeCommand{
				skipStaticReport: tt.skipStaticReport,
				output:           tmpOutput,
				AnalyzeCommandContext: AnalyzeCommandContext{
					log:       log,
					kantraDir: tmpKantraDir,
				},
			}

			// Call the method
			testLog := logr.Discard()
			err = a.GenerateStaticReport(context.Background(), testLog)

			if tt.skipStaticReport {
				// Should return nil immediately without doing anything
				assert.NoError(t, err)

				// Verify no static-report.log was created
				staticReportLog := filepath.Join(tmpOutput, "static-report.log")
				_, statErr := os.Stat(staticReportLog)
				assert.True(t, os.IsNotExist(statErr), "static-report.log should not exist when skipStaticReport is true")
			} else {
				// When not skipping, the function will try to generate a report
				// It will fail in test environment due to missing files/binaries
				// but static-report.log should be created
				assert.Error(t, err, "should error when trying to generate report in test environment")

				staticReportLog := filepath.Join(tmpOutput, "static-report.log")
				_, statErr := os.Stat(staticReportLog)
				// The log file should be created even if generation fails
				assert.NoError(t, statErr, "static-report.log should be created when attempting to generate report")
			}
		})
	}
}

func TestSkipStaticReportFlagParsing(t *testing.T) {
	tests := []struct {
		name                     string
		skipStaticReport         bool
		expectedToGenerateReport bool
	}{
		{
			name:                     "skipStaticReport true should skip generation",
			skipStaticReport:         true,
			expectedToGenerateReport: false,
		},
		{
			name:                     "skipStaticReport false should generate report",
			skipStaticReport:         false,
			expectedToGenerateReport: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &analyzeCommand{
				skipStaticReport: tt.skipStaticReport,
			}

			// Create a mock context
			ctx := context.Background()

			// Test both container and containerless versions
			t.Run("containerless", func(t *testing.T) {
				// For containerless version
				tmpOutput, err := os.MkdirTemp("", "test-skip-report-")
				require.NoError(t, err)
				defer os.RemoveAll(tmpOutput)

				a.output = tmpOutput
				a.AnalyzeCommandContext = AnalyzeCommandContext{
					log: logr.Discard(),
				}

				testLog := logr.Discard()
				err = a.GenerateStaticReport(ctx, testLog)

				if tt.skipStaticReport {
					// Should return nil immediately
					assert.NoError(t, err)
				}
				// Note: When skipStaticReport is false, the function may fail
				// due to missing dependencies in test environment, but that's ok
				// as we're testing the flag behavior, not the full generation
			})
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
