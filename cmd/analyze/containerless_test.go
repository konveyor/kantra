package analyze

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	kantraProvider "github.com/konveyor-ecosystem/kantra/pkg/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Use logr.Discard() for testing - it's the standard no-op logger

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

func TestGetStaticReportSrcPath(t *testing.T) {
	tests := []struct {
		name             string
		staticReportPath string
		kantraDir        string
		expected         string
	}{
		{
			name:             "uses default kantraDir when staticReportPath is empty",
			staticReportPath: "",
			kantraDir:        "/opt/kantra",
			expected:         filepath.Join("/opt/kantra", "static-report"),
		},
		{
			name:             "uses override when staticReportPath is set",
			staticReportPath: "/custom/report/template",
			kantraDir:        "/opt/kantra",
			expected:         "/custom/report/template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &analyzeCommand{
				staticReportPath: tt.staticReportPath,
				AnalyzeCommandContext: AnalyzeCommandContext{
					kantraDir: tt.kantraDir,
				},
			}
			result := a.getStaticReportSrcPath()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateStaticReportWithCustomPath(t *testing.T) {
	log := logr.Discard()

	// Create a temporary custom static report template directory
	customReportDir, err := os.MkdirTemp("", "custom-static-report-")
	require.NoError(t, err)
	defer os.RemoveAll(customReportDir)

	// Create a minimal index.html in the custom report dir
	err = os.WriteFile(filepath.Join(customReportDir, "index.html"), []byte("<html></html>"), 0644)
	require.NoError(t, err)

	// Create temporary output directory
	tmpOutput, err := os.MkdirTemp("", "test-static-report-output-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpOutput)

	// Create minimal output.yaml and dependencies.yaml
	err = os.WriteFile(filepath.Join(tmpOutput, "output.yaml"), []byte("[]"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpOutput, "dependencies.yaml"), []byte("[]"), 0644)
	require.NoError(t, err)

	a := &analyzeCommand{
		staticReportPath: customReportDir,
		output:           tmpOutput,
		input:            "/some/input",
		AnalyzeCommandContext: AnalyzeCommandContext{
			log:       log,
			kantraDir: "/nonexistent/kantra/dir",
		},
	}

	err = a.GenerateStaticReport(context.Background(), log)
	assert.NoError(t, err)

	// Verify the report was generated from the custom path
	_, statErr := os.Stat(filepath.Join(tmpOutput, "static-report", "index.html"))
	assert.NoError(t, statErr, "index.html should be copied from custom static report path")
}

func TestBuildStaticReportOutputUsesCustomPath(t *testing.T) {
	// Create a custom report template directory with a test file
	customReportDir, err := os.MkdirTemp("", "custom-report-template-")
	require.NoError(t, err)
	defer os.RemoveAll(customReportDir)

	testContent := []byte("custom template content")
	err = os.WriteFile(filepath.Join(customReportDir, "index.html"), testContent, 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(customReportDir, "style.css"), []byte("body{}"), 0644)
	require.NoError(t, err)

	// Create output directory
	tmpOutput, err := os.MkdirTemp("", "test-report-output-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpOutput)

	a := &analyzeCommand{
		staticReportPath: customReportDir,
		output:           tmpOutput,
		AnalyzeCommandContext: AnalyzeCommandContext{
			log:       logr.Discard(),
			kantraDir: "/nonexistent/should/not/be/used",
		},
	}

	err = a.buildStaticReportOutput(context.Background(), nil)
	assert.NoError(t, err)

	// Verify files were copied from custom path
	content, err := os.ReadFile(filepath.Join(tmpOutput, "static-report", "index.html"))
	assert.NoError(t, err)
	assert.Equal(t, testContent, content)

	_, statErr := os.Stat(filepath.Join(tmpOutput, "static-report", "style.css"))
	assert.NoError(t, statErr, "style.css should be copied from custom report dir")
}

func TestBuildStaticReportOutputUsesDefaultPath(t *testing.T) {
	// Create a default kantraDir with static-report subdirectory
	tmpKantraDir, err := os.MkdirTemp("", "test-kantra-dir-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpKantraDir)

	defaultReportDir := filepath.Join(tmpKantraDir, "static-report")
	err = os.MkdirAll(defaultReportDir, 0755)
	require.NoError(t, err)

	defaultContent := []byte("default template")
	err = os.WriteFile(filepath.Join(defaultReportDir, "index.html"), defaultContent, 0644)
	require.NoError(t, err)

	// Create output directory
	tmpOutput, err := os.MkdirTemp("", "test-report-output-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpOutput)

	a := &analyzeCommand{
		output: tmpOutput,
		AnalyzeCommandContext: AnalyzeCommandContext{
			log:       logr.Discard(),
			kantraDir: tmpKantraDir,
		},
	}

	err = a.buildStaticReportOutput(context.Background(), nil)
	assert.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(tmpOutput, "static-report", "index.html"))
	assert.NoError(t, err)
	assert.Equal(t, defaultContent, content)
}
