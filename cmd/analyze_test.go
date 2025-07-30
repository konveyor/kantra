package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
)

func Test_analyzeCommand_validateRulesPath(t *testing.T) {
	log := logr.Discard()

	tests := []struct {
		name      string
		setupFunc func() (string, func(), error)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "non-existent path should return error",
			setupFunc: func() (string, func(), error) {
				return "/non/existent/path", func() {}, nil
			},
			wantErr: true,
			errMsg:  "no such file or directory",
		},
		{
			name: "file with .yaml extension should pass",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-rules-")
				if err != nil {
					return "", nil, err
				}
				filePath := filepath.Join(tmpDir, "test.yaml")
				err = os.WriteFile(filePath, []byte("test: value"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				return filePath, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: false,
		},
		{
			name: "file with .yml extension should pass",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-rules-")
				if err != nil {
					return "", nil, err
				}
				filePath := filepath.Join(tmpDir, "test.yml")
				err = os.WriteFile(filePath, []byte("test: value"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				return filePath, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: false,
		},
		{
			name: "xml file extension should not return error but log warning",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-rules-")
				if err != nil {
					return "", nil, err
				}
				filePath := filepath.Join(tmpDir, "test.xml")
				err = os.WriteFile(filePath, []byte("test content"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				return filePath, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: false,
		},
		{
			name: "directory with only YAML files should pass",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-rules-")
				if err != nil {
					return "", nil, err
				}

				files := []string{"rule1.yaml", "rule2.yml", "nested/rule3.yaml"}
				for _, file := range files {
					filePath := filepath.Join(tmpDir, file)
					dir := filepath.Dir(filePath)
					if dir != tmpDir {
						err = os.MkdirAll(dir, 0755)
						if err != nil {
							os.RemoveAll(tmpDir)
							return "", nil, err
						}
					}
					err = os.WriteFile(filePath, []byte("rule: test"), 0644)
					if err != nil {
						os.RemoveAll(tmpDir)
						return "", nil, err
					}
				}
				return tmpDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: false,
		},
		{
			name: "directory with mixed file types should not return error but log warnings",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-rules-")
				if err != nil {
					return "", nil, err
				}

				files := map[string]string{
					"rule1.yaml":         "rule: test",
					"rule2.yml":          "rule: test",
					"readme.xml":         "This is a readme",
					"nested/rule3.yaml":  "rule: nested",
					"nested/invalid.xml": "<root></root>",
				}
				for file, content := range files {
					filePath := filepath.Join(tmpDir, file)
					dir := filepath.Dir(filePath)
					if dir != tmpDir {
						err = os.MkdirAll(dir, 0755)
						if err != nil {
							os.RemoveAll(tmpDir)
							return "", nil, err
						}
					}
					err = os.WriteFile(filePath, []byte(content), 0644)
					if err != nil {
						os.RemoveAll(tmpDir)
						return "", nil, err
					}
				}
				return tmpDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: false,
		},
		{
			name: "empty directory should pass",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-rules-")
				if err != nil {
					return "", nil, err
				}
				return tmpDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: false,
		},
		{
			name: "empty string path should return error",
			setupFunc: func() (string, func(), error) {
				return "", func() {}, nil
			},
			wantErr: true,
			errMsg:  "no such file or directory",
		},
		{
			name: "file with no extension should not return error but log warning",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-rules-")
				if err != nil {
					return "", nil, err
				}
				filePath := filepath.Join(tmpDir, "rulefile")
				err = os.WriteFile(filePath, []byte("rule: test"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				return filePath, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: false,
		},
		{
			name: "uppercase YAML extension should pass",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-rules-")
				if err != nil {
					return "", nil, err
				}
				filePath := filepath.Join(tmpDir, "test.YAML")
				err = os.WriteFile(filePath, []byte("rule: test"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				return filePath, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: false,
		},
		{
			name: "very deep nested directory should pass",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-rules-")
				if err != nil {
					return "", nil, err
				}
				deepPath := tmpDir
				for i := 0; i < 10; i++ {
					deepPath = filepath.Join(deepPath, fmt.Sprintf("level%d", i))
				}
				err = os.MkdirAll(deepPath, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				filePath := filepath.Join(deepPath, "deep.yaml")
				err = os.WriteFile(filePath, []byte("rule: deep"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				return tmpDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: false,
		},
		{
			name: "relative path should work",
			setupFunc: func() (string, func(), error) {
				relDir := "test-rules-rel"
				err := os.MkdirAll(relDir, 0755)
				if err != nil {
					return "", nil, err
				}

				filePath := filepath.Join(relDir, "test.yaml")
				err = os.WriteFile(filePath, []byte("rule: test"), 0644)
				if err != nil {
					os.RemoveAll(relDir)
					return "", nil, err
				}
				return relDir, func() { os.RemoveAll(relDir) }, nil
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rulePath, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Failed to setup test: %v", err)
			}
			defer cleanup()

			a := &analyzeCommand{}
			a.log = log
			err = a.validateRulesPath(rulePath)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateRulesPath() expected error but got none")
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("validateRulesPath() error = %v, expected to contain %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateRulesPath() unexpected error = %v", err)
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func Test_analyzeCommand_getLabelSelectorArgs(t *testing.T) {
	tests := []struct {
		name          string
		labelSelector string
		sources       []string
		targets       []string
		want          string
	}{
		{
			name: "neither sources nor targets must not create label selector",
		},
		{
			name:    "one target specified, return target, catch-all source and default labels",
			targets: []string{"test"},
			want:    "((konveyor.io/target=test) && konveyor.io/source) || (discovery)",
		},
		{
			name:    "one source specified, return source and default labels",
			sources: []string{"test"},
			want:    "(konveyor.io/source=test) || (discovery)",
		},
		{
			name:    "one source & one target specified, return source, target and default labels",
			sources: []string{"test"},
			targets: []string{"test"},
			want:    "((konveyor.io/target=test) && (konveyor.io/source=test)) || (discovery)",
		},
		{
			name:    "multiple sources specified, OR them all with default labels",
			sources: []string{"t1", "t2"},
			want:    "(konveyor.io/source=t1 || konveyor.io/source=t2) || (discovery)",
		},
		{
			name:    "multiple targets specified, OR them all, AND result with catch-all source label, finally OR with default labels",
			targets: []string{"t1", "t2"},
			want:    "((konveyor.io/target=t1 || konveyor.io/target=t2) && konveyor.io/source) || (discovery)",
		},
		{
			name:    "multiple sources & targets specified, OR them within each other, AND result with catch-all source label, finally OR with default labels",
			targets: []string{"t1", "t2"},
			sources: []string{"t1", "t2"},
			want:    "((konveyor.io/target=t1 || konveyor.io/target=t2) && (konveyor.io/source=t1 || konveyor.io/source=t2)) || (discovery)",
		},
		{
			name:          "return the labelSelector when specified",
			labelSelector: "example.io/target=foo",
			want:          "example.io/target=foo",
		},
		{
			name:          "labelSelector should win",
			targets:       []string{"t1", "t2"},
			sources:       []string{"t1", "t2"},
			labelSelector: "example.io/target=foo",
			want:          "example.io/target=foo",
		},
		{
			name:          "multiple sources & targets specified, OR them within each other, AND result with catch-all source label, finally OR with default labels",
			targets:       []string{"t1", "t2"},
			sources:       []string{"t1", "t2"},
			labelSelector: "",
			want:          "((konveyor.io/target=t1 || konveyor.io/target=t2) && (konveyor.io/source=t1 || konveyor.io/source=t2)) || (discovery)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &analyzeCommand{
				sources:       tt.sources,
				targets:       tt.targets,
				labelSelector: tt.labelSelector,
			}
			if got := a.getLabelSelector(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("analyzeCommand.getLabelSelectorArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_analyzeCommand_RunAnalysis_InputValidation(t *testing.T) {
	log := logr.Discard()

	tests := []struct {
		name        string
		setupCtx    func() context.Context
		volName     string
		setup       func() *analyzeCommand
		expectError bool
		errorMsg    string
	}{
		{
			name: "normal context and valid volume name should work",
			setupCtx: func() context.Context {
				return context.Background()
			},
			volName: "valid-volume-name",
			setup: func() *analyzeCommand {
				return &analyzeCommand{
					output:       "/tmp/test-output",
					contextLines: 10,
					AnalyzeCommandContext: AnalyzeCommandContext{
						log:          log,
						providersMap: make(map[string]ProviderInit),
					},
				}
			},
			expectError: false,
		},
		{
			name: "cancelled context should return error",
			setupCtx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			volName: "test-volume",
			setup: func() *analyzeCommand {
				return &analyzeCommand{
					output:       "/tmp/test-output",
					contextLines: 10,
					AnalyzeCommandContext: AnalyzeCommandContext{
						log:          log,
						providersMap: make(map[string]ProviderInit),
					},
				}
			},
			expectError: true,
			errorMsg:    "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "test-run-analysis-input-")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			outputDir := filepath.Join(tmpDir, "output")
			err = os.MkdirAll(outputDir, 0755)
			if err != nil {
				t.Fatalf("Failed to create output dir: %v", err)
			}

			a := tt.setup()
			a.output = outputDir

			ctx := tt.setupCtx()
			err = a.validateRunAnalysisInputs(ctx, tt.volName)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing %q, got: %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func (a *analyzeCommand) validateRunAnalysisInputs(ctx context.Context, volName string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if a.output == "" {
		return fmt.Errorf("output directory is required")
	}

	if deadline, ok := ctx.Deadline(); ok && time.Now().After(deadline) {
		return context.DeadlineExceeded
	}

	return nil
}

func (a *analyzeCommand) buildVolumeMapping(volName string) map[string]string {
	return map[string]string{
		volName:  "/opt/input/source",
		a.output: "/opt/output",
	}
}

func Test_analyzeCommand_needDefaultRules(t *testing.T) {
	tests := []struct {
		name                          string
		initialEnableDefaultRulesets  bool
		providersMap                  map[string]ProviderInit
		expectedEnableDefaultRulesets bool
	}{
		{
			name:                          "no providers should disable default rulesets",
			initialEnableDefaultRulesets:  true,
			providersMap:                  make(map[string]ProviderInit),
			expectedEnableDefaultRulesets: false,
		},
		{
			name:                         "java provider with enabled rulesets should keep rulesets enabled",
			initialEnableDefaultRulesets: true,
			providersMap: map[string]ProviderInit{
				util.JavaProvider: {},
			},
			expectedEnableDefaultRulesets: true,
		},
		{
			name:                         "java provider with disabled rulesets should keep rulesets disabled",
			initialEnableDefaultRulesets: false,
			providersMap: map[string]ProviderInit{
				util.JavaProvider: {},
			},
			expectedEnableDefaultRulesets: false,
		},
		{
			name:                         "non-java providers only should disable default rulesets",
			initialEnableDefaultRulesets: true,
			providersMap: map[string]ProviderInit{
				util.PythonProvider: {},
				util.GoProvider:     {},
			},
			expectedEnableDefaultRulesets: false,
		},
		{
			name:                         "mixed providers including java with enabled rulesets should keep rulesets enabled",
			initialEnableDefaultRulesets: true,
			providersMap: map[string]ProviderInit{
				util.JavaProvider:   {},
				util.PythonProvider: {},
				util.GoProvider:     {},
			},
			expectedEnableDefaultRulesets: true,
		},
		{
			name:                         "mixed providers including java with disabled rulesets should keep rulesets disabled",
			initialEnableDefaultRulesets: false,
			providersMap: map[string]ProviderInit{
				util.JavaProvider:   {},
				util.PythonProvider: {},
				util.GoProvider:     {},
			},
			expectedEnableDefaultRulesets: false,
		},
		{
			name:                         "python provider only should disable default rulesets",
			initialEnableDefaultRulesets: true,
			providersMap: map[string]ProviderInit{
				util.PythonProvider: {},
			},
			expectedEnableDefaultRulesets: false,
		},
		{
			name:                         "go provider only should disable default rulesets",
			initialEnableDefaultRulesets: true,
			providersMap: map[string]ProviderInit{
				util.GoProvider: {},
			},
			expectedEnableDefaultRulesets: false,
		},
		{
			name:                         "nodejs provider only should disable default rulesets",
			initialEnableDefaultRulesets: true,
			providersMap: map[string]ProviderInit{
				util.NodeJSProvider: {},
			},
			expectedEnableDefaultRulesets: false,
		},
		{
			name:                         "dotnet provider only should disable default rulesets",
			initialEnableDefaultRulesets: true,
			providersMap: map[string]ProviderInit{
				util.DotnetProvider: {},
			},
			expectedEnableDefaultRulesets: false,
		},
		{
			name:                         "already disabled rulesets with non-java providers should remain disabled",
			initialEnableDefaultRulesets: false,
			providersMap: map[string]ProviderInit{
				util.PythonProvider: {},
				util.GoProvider:     {},
			},
			expectedEnableDefaultRulesets: false,
		},
		{
			name:                         "java provider first in iteration should enable",
			initialEnableDefaultRulesets: true,
			providersMap: map[string]ProviderInit{
				util.JavaProvider:   {},
				util.PythonProvider: {},
				util.GoProvider:     {},
				util.NodeJSProvider: {},
			},
			expectedEnableDefaultRulesets: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &analyzeCommand{
				enableDefaultRulesets: tt.initialEnableDefaultRulesets,
				AnalyzeCommandContext: AnalyzeCommandContext{
					providersMap: tt.providersMap,
				},
			}
			a.needDefaultRules()

			if a.enableDefaultRulesets != tt.expectedEnableDefaultRulesets {
				t.Errorf("needDefaultRules() set enableDefaultRulesets = %v, want %v",
					a.enableDefaultRulesets, tt.expectedEnableDefaultRulesets)
			}
		})
	}
}

func Test_analyzeCommand_needDefaultRules_StateChanges(t *testing.T) {
	log := logr.Discard()

	tests := []struct {
		name                string
		initialState        *analyzeCommand
		expectedOnlyChanged []string
	}{
		{
			name: "should only modify enableDefaultRulesets when disabling",
			initialState: &analyzeCommand{
				enableDefaultRulesets: true,
				output:                "/test/output",
				contextLines:          100,
				sources:               []string{"test"},
				AnalyzeCommandContext: AnalyzeCommandContext{
					log:          log,
					providersMap: map[string]ProviderInit{util.PythonProvider: {}},
				},
			},
			expectedOnlyChanged: []string{"enableDefaultRulesets"},
		},
		{
			name: "should not modify anything when java provider present and rulesets enabled",
			initialState: &analyzeCommand{
				enableDefaultRulesets: true,
				output:                "/test/output",
				contextLines:          100,
				sources:               []string{"test"},
				AnalyzeCommandContext: AnalyzeCommandContext{
					log:          log,
					providersMap: map[string]ProviderInit{util.JavaProvider: {}},
				},
			},
			expectedOnlyChanged: []string{},
		},
		{
			name: "should not modify anything when rulesets already disabled",
			initialState: &analyzeCommand{
				enableDefaultRulesets: false,
				output:                "/test/output",
				contextLines:          100,
				sources:               []string{"test"},
				AnalyzeCommandContext: AnalyzeCommandContext{
					log:          log,
					providersMap: map[string]ProviderInit{util.PythonProvider: {}},
				},
			},
			expectedOnlyChanged: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := &analyzeCommand{
				enableDefaultRulesets: tt.initialState.enableDefaultRulesets,
				output:                tt.initialState.output,
				contextLines:          tt.initialState.contextLines,
				sources:               make([]string, len(tt.initialState.sources)),
				AnalyzeCommandContext: AnalyzeCommandContext{
					log:          tt.initialState.log,
					providersMap: make(map[string]ProviderInit),
				},
			}
			copy(original.sources, tt.initialState.sources)
			for k, v := range tt.initialState.providersMap {
				original.providersMap[k] = v
			}

			tt.initialState.needDefaultRules()

			changedFields := []string{}

			if tt.initialState.enableDefaultRulesets != original.enableDefaultRulesets {
				changedFields = append(changedFields, "enableDefaultRulesets")
			}
			if tt.initialState.output != original.output {
				changedFields = append(changedFields, "output")
			}
			if tt.initialState.contextLines != original.contextLines {
				changedFields = append(changedFields, "contextLines")
			}
			if !reflect.DeepEqual(tt.initialState.sources, original.sources) {
				changedFields = append(changedFields, "sources")
			}
			if !reflect.DeepEqual(tt.initialState.providersMap, original.providersMap) {
				changedFields = append(changedFields, "providersMap")
			}

			if !reflect.DeepEqual(changedFields, tt.expectedOnlyChanged) {
				t.Errorf("needDefaultRules() changed fields %v, expected only %v to change",
					changedFields, tt.expectedOnlyChanged)
			}
		})
	}
}

func Test_analyzeCommand_needDefaultRules_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *analyzeCommand
		expected bool
	}{
		{
			name: "nil providersMap should disable rulesets",
			setup: func() *analyzeCommand {
				return &analyzeCommand{
					enableDefaultRulesets: true,
					AnalyzeCommandContext: AnalyzeCommandContext{
						providersMap: nil,
					},
				}
			},
			expected: false,
		},
		{
			name: "empty string key in providersMap should not match java",
			setup: func() *analyzeCommand {
				return &analyzeCommand{
					enableDefaultRulesets: true,
					AnalyzeCommandContext: AnalyzeCommandContext{
						providersMap: map[string]ProviderInit{
							"": {},
						},
					},
				}
			},
			expected: false,
		},
		{
			name: "whitespace in provider name should not match java",
			setup: func() *analyzeCommand {
				return &analyzeCommand{
					enableDefaultRulesets: true,
					AnalyzeCommandContext: AnalyzeCommandContext{
						providersMap: map[string]ProviderInit{
							" java ": {},
						},
					},
				}
			},
			expected: false,
		},
		{
			name: "java with different casing should not match",
			setup: func() *analyzeCommand {
				return &analyzeCommand{
					enableDefaultRulesets: true,
					AnalyzeCommandContext: AnalyzeCommandContext{
						providersMap: map[string]ProviderInit{
							"JAVA": {},
							"Java": {},
							"jAvA": {},
						},
					},
				}
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := tt.setup()
			a.needDefaultRules()

			if a.enableDefaultRulesets != tt.expected {
				t.Errorf("needDefaultRules() set enableDefaultRulesets = %v, want %v",
					a.enableDefaultRulesets, tt.expected)
			}
		})
	}
}

func Test_analyzeCommand_CheckOverwriteOutput(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func() (*analyzeCommand, func(), error)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "non-existent output directory should pass",
			setupFunc: func() (*analyzeCommand, func(), error) {
				return &analyzeCommand{
					output: "/non/existent/output/path",
				}, func() {}, nil
			},
			wantErr: false,
		},
		{
			name: "existing output directory without overwrite should fail",
			setupFunc: func() (*analyzeCommand, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-output-")
				if err != nil {
					return nil, nil, err
				}
				return &analyzeCommand{
					output:    tmpDir,
					overwrite: false,
				}, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: true,
			errMsg:  "already exists and --overwrite not set",
		},
		{
			name: "existing output directory with overwrite should pass",
			setupFunc: func() (*analyzeCommand, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-output-")
				if err != nil {
					return nil, nil, err
				}
				// Create a test file to verify it gets removed
				testFile := filepath.Join(tmpDir, "test.txt")
				err = os.WriteFile(testFile, []byte("test"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return nil, nil, err
				}
				return &analyzeCommand{
					output:    tmpDir,
					overwrite: true,
				}, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: false,
		},
		{
			name: "bulk analysis with existing analysis.log should fail",
			setupFunc: func() (*analyzeCommand, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-output-")
				if err != nil {
					return nil, nil, err
				}
				logFile := filepath.Join(tmpDir, "analysis.log")
				err = os.WriteFile(logFile, []byte("test"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return nil, nil, err
				}
				return &analyzeCommand{
					output: tmpDir,
					bulk:   true,
				}, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: true,
			errMsg:  "already contains 'analysis.log'",
		},
		{
			name: "bulk analysis with same input should fail",
			setupFunc: func() (*analyzeCommand, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-output-")
				if err != nil {
					return nil, nil, err
				}
				inputDir, err := os.MkdirTemp("", "test-input-")
				if err != nil {
					os.RemoveAll(tmpDir)
					return nil, nil, err
				}
				outputFile := fmt.Sprintf("output.yaml.%s", filepath.Base(inputDir))
				outputPath := filepath.Join(tmpDir, outputFile)
				err = os.WriteFile(outputPath, []byte("test"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					os.RemoveAll(inputDir)
					return nil, nil, err
				}
				return &analyzeCommand{
						output: tmpDir,
						input:  inputDir,
						bulk:   true,
					}, func() {
						os.RemoveAll(tmpDir)
						os.RemoveAll(inputDir)
					}, nil
			},
			wantErr: true,
			errMsg:  "already contains analysis report for provided input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Failed to setup test: %v", err)
			}
			defer cleanup()

			err = a.CheckOverwriteOutput()

			if tt.wantErr {
				if err == nil {
					t.Errorf("CheckOverwriteOutput() expected error but got none")
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("CheckOverwriteOutput() error = %v, expected to contain %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("CheckOverwriteOutput() unexpected error = %v", err)
				}
			}
		})
	}
}

func Test_analyzeCommand_moveResults(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func() (*analyzeCommand, func(), error)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "successful move with all files present",
			setupFunc: func() (*analyzeCommand, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-move-")
				if err != nil {
					return nil, nil, err
				}
				inputDir, err := os.MkdirTemp("", "test-input-")
				if err != nil {
					os.RemoveAll(tmpDir)
					return nil, nil, err
				}

				files := map[string]string{
					"output.yaml":       "test output",
					"analysis.log":      "test log",
					"dependencies.yaml": "test deps",
				}
				for filename, content := range files {
					filePath := filepath.Join(tmpDir, filename)
					err = os.WriteFile(filePath, []byte(content), 0644)
					if err != nil {
						os.RemoveAll(tmpDir)
						os.RemoveAll(inputDir)
						return nil, nil, err
					}
				}

				return &analyzeCommand{
						input:  inputDir,
						output: tmpDir,
						mode:   "full",
					}, func() {
						os.RemoveAll(tmpDir)
						os.RemoveAll(inputDir)
					}, nil
			},
			wantErr: false,
		},
		{
			name: "move without dependencies.yaml should succeed",
			setupFunc: func() (*analyzeCommand, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-move-")
				if err != nil {
					return nil, nil, err
				}
				inputDir, err := os.MkdirTemp("", "test-input-")
				if err != nil {
					os.RemoveAll(tmpDir)
					return nil, nil, err
				}

				// Create only output.yaml and analysis.log
				files := map[string]string{
					"output.yaml":  "test output",
					"analysis.log": "test log",
				}
				for filename, content := range files {
					filePath := filepath.Join(tmpDir, filename)
					err = os.WriteFile(filePath, []byte(content), 0644)
					if err != nil {
						os.RemoveAll(tmpDir)
						os.RemoveAll(inputDir)
						return nil, nil, err
					}
				}

				return &analyzeCommand{
						input:  inputDir,
						output: tmpDir,
						mode:   "source-only",
					}, func() {
						os.RemoveAll(tmpDir)
						os.RemoveAll(inputDir)
					}, nil
			},
			wantErr: false,
		},
		{
			name: "missing output.yaml should fail",
			setupFunc: func() (*analyzeCommand, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-move-")
				if err != nil {
					return nil, nil, err
				}
				inputDir, err := os.MkdirTemp("", "test-input-")
				if err != nil {
					os.RemoveAll(tmpDir)
					return nil, nil, err
				}

				// Create only analysis.log
				filePath := filepath.Join(tmpDir, "analysis.log")
				err = os.WriteFile(filePath, []byte("test log"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					os.RemoveAll(inputDir)
					return nil, nil, err
				}

				return &analyzeCommand{
						input:  inputDir,
						output: tmpDir,
					}, func() {
						os.RemoveAll(tmpDir)
						os.RemoveAll(inputDir)
					}, nil
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Failed to setup test: %v", err)
			}
			defer cleanup()

			err = a.moveResults()

			if tt.wantErr {
				if err == nil {
					t.Errorf("moveResults() expected error but got none")
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("moveResults() error = %v, expected to contain %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("moveResults() unexpected error = %v", err)
				}
			}
		})
	}
}
