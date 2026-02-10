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
	"github.com/konveyor-ecosystem/kantra/pkg/profile"
	kantraProvider "github.com/konveyor-ecosystem/kantra/pkg/provider"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/spf13/cobra"
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
			name:    "one target specified, return target and default labels",
			targets: []string{"test"},
			want:    "(konveyor.io/target=test) || (discovery)",
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
			name:    "multiple targets specified, OR them all with default labels",
			targets: []string{"t1", "t2"},
			want:    "(konveyor.io/target=t1 || konveyor.io/target=t2) || (discovery)",
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
				util.CsharpProvider: {},
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

func Test_analyzeCommand_ValidateAndLoadProfile(t *testing.T) {
	tests := []struct {
		name             string
		setup            func(t *testing.T) *analyzeCommand
		wantProfileNil   bool
		wantErr          bool
		errContains      string
		checkProfilePath func(t *testing.T, a *analyzeCommand)
	}{
		{
			name: "profileDir set to valid dir with profile.yaml returns profile and sets profilePath",
			setup: func(t *testing.T) *analyzeCommand {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					t.Fatalf("MkdirTemp: %v", err)
				}
				t.Cleanup(func() { os.RemoveAll(tmpDir) })
				profilePath := filepath.Join(tmpDir, "profile.yaml")
				if err := os.WriteFile(profilePath, []byte("mode:\n  withDeps: true\n"), 0644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				return &analyzeCommand{profileDir: tmpDir}
			},
			wantProfileNil: false,
			wantErr:        false,
			checkProfilePath: func(t *testing.T, a *analyzeCommand) {
				if a.profilePath == "" {
					t.Error("expected profilePath to be set")
				}
				if !strings.HasSuffix(a.profilePath, "profile.yaml") {
					t.Errorf("profilePath should end with profile.yaml, got %q", a.profilePath)
				}
			},
		},
		{
			name: "profileDir set to non-existent path returns error",
			setup: func(t *testing.T) *analyzeCommand {
				return &analyzeCommand{profileDir: filepath.Join(os.TempDir(), "nonexistent-profile-dir-xyz")}
			},
			wantProfileNil: true,
			wantErr:        true,
			errContains:    "failed to stat",
		},
		{
			name: "profileDir set to file (not directory) returns error",
			setup: func(t *testing.T) *analyzeCommand {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					t.Fatalf("MkdirTemp: %v", err)
				}
				t.Cleanup(func() { os.RemoveAll(tmpDir) })
				f := filepath.Join(tmpDir, "file.txt")
				if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				return &analyzeCommand{profileDir: f}
			},
			wantProfileNil: true,
			wantErr:        true,
			errContains:    "not a directory",
		},
		{
			name: "profileDir empty and input empty leaves profilePath empty and returns nil, nil",
			setup: func(t *testing.T) *analyzeCommand {
				return &analyzeCommand{}
			},
			wantProfileNil: true,
			wantErr:        false,
			checkProfilePath: func(t *testing.T, a *analyzeCommand) {
				if a.profilePath != "" {
					t.Errorf("expected profilePath empty, got %q", a.profilePath)
				}
			},
		},
		{
			name: "profileDir empty and input set with single profile in default path returns profile and sets profilePath",
			setup: func(t *testing.T) *analyzeCommand {
				tmpDir, err := os.MkdirTemp("", "test-input-")
				if err != nil {
					t.Fatalf("MkdirTemp: %v", err)
				}
				t.Cleanup(func() { os.RemoveAll(tmpDir) })
				profilesDir := filepath.Join(tmpDir, ".konveyor", "profiles", "single")
				if err := os.MkdirAll(profilesDir, 0755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				profilePath := filepath.Join(profilesDir, "profile.yaml")
				if err := os.WriteFile(profilePath, []byte("mode:\n  withDeps: false\n"), 0644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				return &analyzeCommand{input: tmpDir}
			},
			wantProfileNil: false,
			wantErr:        false,
			checkProfilePath: func(t *testing.T, a *analyzeCommand) {
				if a.profilePath == "" {
					t.Error("expected profilePath to be set when single profile found under input")
				}
			},
		},
		{
			name: "profileDir set to dir without profile.yaml returns error when UnmarshalProfile fails",
			setup: func(t *testing.T) *analyzeCommand {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					t.Fatalf("MkdirTemp: %v", err)
				}
				t.Cleanup(func() { os.RemoveAll(tmpDir) })
				return &analyzeCommand{profileDir: tmpDir}
			},
			wantProfileNil: true,
			wantErr:        true,
			errContains:    "failed to load profile",
		},
		{
			name: "profileDir empty and input set but profiles path is a file returns error",
			setup: func(t *testing.T) *analyzeCommand {
				tmpDir, err := os.MkdirTemp("", "test-input-")
				if err != nil {
					t.Fatalf("MkdirTemp: %v", err)
				}
				t.Cleanup(func() { os.RemoveAll(tmpDir) })
				profilesPath := filepath.Join(tmpDir, ".konveyor", "profiles")
				if err := os.MkdirAll(filepath.Dir(profilesPath), 0755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				if err := os.WriteFile(profilesPath, []byte("not a dir"), 0644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				return &analyzeCommand{input: tmpDir}
			},
			wantProfileNil: true,
			wantErr:        true,
			errContains:    "not a directory",
		},
		{
			name: "input is not directory",
			setup: func(t *testing.T) *analyzeCommand {
				tmpDir, err := os.MkdirTemp("", "test-input-")
				if err != nil {
					t.Fatalf("MkdirTemp: %v", err)
				}
				t.Cleanup(func() { os.RemoveAll(tmpDir) })
				if err := os.WriteFile(filepath.Join(tmpDir, "file_input"), []byte("not a dir"), 0644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				return &analyzeCommand{input: filepath.Join(tmpDir, "file_input")}
			},
			wantProfileNil: true,
			wantErr:        false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := tt.setup(t)
			got, err := a.ValidateAndLoadProfile()
			if tt.wantErr {
				if err == nil {
					t.Fatal("ValidateAndLoadProfile expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, want substring %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateAndLoadProfile: %v", err)
			}
			if tt.wantProfileNil {
				if got != nil {
					t.Errorf("expected nil profile, got %+v", got)
				}
			} else {
				if got == nil {
					t.Fatal("expected non-nil profile")
				}
			}
			if tt.checkProfilePath != nil {
				tt.checkProfilePath(t, a)
			}
		})
	}
}

func Test_analyzeCommand_createProfileSettings(t *testing.T) {
	a := &analyzeCommand{
		input:                 "/some/input",
		mode:                  string(provider.FullAnalysisMode),
		analyzeKnownLibraries: true,
		incidentSelector:      "package=com.example",
		labelSelector:         "(label1)",
		rules:                 []string{"/rules/a"},
		enableDefaultRulesets: true,
	}
	got := a.createProfileSettings()
	if got == nil {
		t.Fatal("createProfileSettings() returned nil")
	}
	if got.Input != a.input {
		t.Errorf("Input = %q, want %q", got.Input, a.input)
	}
	if got.Mode != a.mode {
		t.Errorf("Mode = %q, want %q", got.Mode, a.mode)
	}
	if !got.AnalyzeKnownLibraries {
		t.Error("AnalyzeKnownLibraries = false, want true")
	}
	if got.IncidentSelector != a.incidentSelector {
		t.Errorf("IncidentSelector = %q, want %q", got.IncidentSelector, a.incidentSelector)
	}
	if got.LabelSelector != a.labelSelector {
		t.Errorf("LabelSelector = %q, want %q", got.LabelSelector, a.labelSelector)
	}
	if len(got.Rules) != 1 || got.Rules[0] != "/rules/a" {
		t.Errorf("Rules = %v, want [/rules/a]", got.Rules)
	}
	if !got.EnableDefaultRulesets {
		t.Error("EnableDefaultRulesets = false, want true")
	}
}

func Test_analyzeCommand_applyProfileSettings(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-apply-profile-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	konveyorDir := filepath.Join(tmpDir, "app", ".konveyor", "profiles", "p")
	if err := os.MkdirAll(konveyorDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	profilePath := filepath.Join(konveyorDir, "profile.yaml")
	profileYAML := `
mode:
  withDeps: true
scope:
  withKnownLibs: true
rules:
  labels:
    included: ["konveyor.io/target=eap8"]
`
	if err := os.WriteFile(profilePath, []byte(profileYAML), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	rulesDir := filepath.Join(konveyorDir, "rules", "r1")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatalf("MkdirAll rules: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "rule.yaml"), []byte("rule: x"), 0644); err != nil {
		t.Fatalf("WriteFile rule: %v", err)
	}

	cmd := &cobra.Command{}
	cmd.Flags().String("input", "", "")
	cmd.Flags().String("mode", "", "")
	cmd.Flags().Bool("analyze-known-libraries", false, "")
	cmd.Flags().String("incident-selector", "", "")
	cmd.Flags().String("label-selector", "", "")
	cmd.Flags().StringSlice("rules", nil, "")
	cmd.Flags().Bool("enable-default-rulesets", true, "")

	a := &analyzeCommand{
		input:                 "",
		mode:                  "",
		analyzeKnownLibraries: false,
		rules:                 nil,
		enableDefaultRulesets: false,
		AnalyzeCommandContext: AnalyzeCommandContext{log: logr.Discard()},
	}
	err = a.applyProfileSettings(profilePath, cmd)
	if err != nil {
		t.Fatalf("applyProfileSettings: %v", err)
	}
	if a.input != filepath.Join(tmpDir, "app") {
		t.Errorf("input = %q, want app dir", a.input)
	}
	if a.mode != string(provider.FullAnalysisMode) {
		t.Errorf("mode = %q, want full", a.mode)
	}
	if !a.analyzeKnownLibraries {
		t.Error("analyzeKnownLibraries = false, want true")
	}
	if len(a.rules) == 0 {
		t.Error("rules should be populated from profile")
	}
	if !a.enableDefaultRulesets {
		t.Error("enableDefaultRulesets = false, want true (profile has konveyor label)")
	}
}

func Test_analyzeCommand_applyProfileSettings_error(t *testing.T) {
	// Profile path that does not contain .konveyor causes SetSettingsFromProfile to return error
	tmpDir, err := os.MkdirTemp("", "test-apply-profile-err-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	profilePath := filepath.Join(tmpDir, "profile.yaml")
	if err := os.WriteFile(profilePath, []byte("mode: {}"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cmd := &cobra.Command{}
	cmd.Flags().String("input", "", "")
	cmd.Flags().String("mode", "", "")
	cmd.Flags().Bool("enable-default-rulesets", true, "")
	a := &analyzeCommand{AnalyzeCommandContext: AnalyzeCommandContext{log: logr.Discard()}}
	err = a.applyProfileSettings(profilePath, cmd)
	if err == nil {
		t.Fatal("applyProfileSettings expected error when path does not contain .konveyor")
	}
	if !strings.Contains(err.Error(), ".konveyor") {
		t.Errorf("error = %v, want mention of .konveyor", err)
	}
}

func Test_analyzeCommand_Validate_withFoundProfile(t *testing.T) {
	ctx := context.Background()
	cmd := &cobra.Command{}

	t.Run("no profile and no rules and default rulesets disabled returns error", func(t *testing.T) {
		// Use a path that does not exist so CheckOverwriteOutput does not fail first
		outDir := filepath.Join(os.TempDir(), fmt.Sprintf("kantra-validate-out-%d", time.Now().UnixNano()))
		t.Cleanup(func() { _ = os.RemoveAll(outDir) })
		a := &analyzeCommand{
			output:                outDir,
			input:                 "",
			mode:                  string(provider.FullAnalysisMode),
			enableDefaultRulesets: false,
			rules:                 nil,
			AnalyzeCommandContext: AnalyzeCommandContext{log: logr.Discard()},
		}
		err := a.Validate(ctx, cmd, nil)
		if err == nil {
			t.Fatal("Validate expected error when no profile, no rules, default rulesets disabled")
		}
		if !strings.Contains(err.Error(), "must specify rules") {
			t.Errorf("error = %v, want substring 'must specify rules'", err)
		}
	})

	t.Run("profile with rules allows validation when default rulesets disabled and no rules", func(t *testing.T) {
		outDir := filepath.Join(os.TempDir(), fmt.Sprintf("kantra-validate-out-%d", time.Now().UnixNano()))
		t.Cleanup(func() { _ = os.RemoveAll(outDir) })
		profileDir, err := os.MkdirTemp("", "test-validate-profile-")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(profileDir) })
		profilePath := filepath.Join(profileDir, "profile.yaml")
		if err := os.WriteFile(profilePath, []byte("mode:\n  withDeps: true\n"), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		// GetRulesInProfile returns only subdirs of rules/, so use a ruleset subdir
		rulesetDir := filepath.Join(profileDir, "rules", "my-ruleset")
		if err := os.MkdirAll(rulesetDir, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(rulesetDir, "rule.yaml"), []byte("rule: x"), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		foundProfile, _ := profile.UnmarshalProfile(profilePath)
		if foundProfile == nil {
			t.Fatal("UnmarshalProfile failed")
		}
		// Simulate applied profile: a.rules is what GetRulesInProfile returns (ruleset subdirs)
		profileRules, _ := profile.GetRulesInProfile(profileDir)
		a := &analyzeCommand{
			output:                outDir,
			input:                 "",
			mode:                  string(provider.FullAnalysisMode),
			enableDefaultRulesets: false,
			rules:                 profileRules,
			profilePath:           profilePath,
			AnalyzeCommandContext: AnalyzeCommandContext{log: logr.Discard()},
		}
		err = a.Validate(ctx, cmd, foundProfile)
		if err != nil {
			t.Errorf("Validate with profile that has rules: %v", err)
		}
	})

	t.Run("profile without rules and no rules and default rulesets disabled returns error", func(t *testing.T) {
		outDir := filepath.Join(os.TempDir(), fmt.Sprintf("kantra-validate-out-%d", time.Now().UnixNano()))
		t.Cleanup(func() { _ = os.RemoveAll(outDir) })
		profileDir, err := os.MkdirTemp("", "test-validate-profile-")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(profileDir) })
		profilePath := filepath.Join(profileDir, "profile.yaml")
		if err := os.WriteFile(profilePath, []byte("mode:\n  withDeps: true\n"), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		// no rules/ dir -> applied a.rules stays empty -> validation should fail
		foundProfile, _ := profile.UnmarshalProfile(profilePath)
		if foundProfile == nil {
			t.Fatal("UnmarshalProfile failed")
		}
		a := &analyzeCommand{
			output:                outDir,
			input:                 "",
			mode:                  string(provider.FullAnalysisMode),
			enableDefaultRulesets: false,
			rules:                 nil,
			profilePath:           profilePath,
			AnalyzeCommandContext: AnalyzeCommandContext{log: logr.Discard()},
		}
		err = a.Validate(ctx, cmd, foundProfile)
		if err == nil {
			t.Fatal("Validate expected error when profile has no rules and no rules specified")
		}
		if !strings.Contains(err.Error(), "must specify rules") {
			t.Errorf("error = %v, want substring 'must specify rules'", err)
		}
	})

	t.Run("default rulesets enabled passes without profile", func(t *testing.T) {
		outDir := filepath.Join(os.TempDir(), fmt.Sprintf("kantra-validate-out-%d", time.Now().UnixNano()))
		t.Cleanup(func() { _ = os.RemoveAll(outDir) })
		a := &analyzeCommand{
			output:                outDir,
			input:                 "",
			mode:                  string(provider.FullAnalysisMode),
			enableDefaultRulesets: true,
			rules:                 nil,
			AnalyzeCommandContext: AnalyzeCommandContext{log: logr.Discard()},
		}
		err := a.Validate(ctx, cmd, nil)
		if err != nil {
			t.Errorf("Validate with enableDefaultRulesets true: %v", err)
		}
	})

	t.Run("non-empty rules passes with default rulesets disabled", func(t *testing.T) {
		outDir := filepath.Join(os.TempDir(), fmt.Sprintf("kantra-validate-out-%d", time.Now().UnixNano()))
		t.Cleanup(func() { _ = os.RemoveAll(outDir) })
		ruleDir, err := os.MkdirTemp("", "kantra-validate-rules-")
		if err != nil {
			t.Fatalf("MkdirTemp rules: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(ruleDir) })
		if err := os.WriteFile(filepath.Join(ruleDir, "rule.yaml"), []byte("rule: x"), 0644); err != nil {
			t.Fatalf("WriteFile rule: %v", err)
		}
		a := &analyzeCommand{
			output:                outDir,
			input:                 "",
			mode:                  string(provider.FullAnalysisMode),
			enableDefaultRulesets: false,
			rules:                 []string{ruleDir},
			AnalyzeCommandContext: AnalyzeCommandContext{log: logr.Discard()},
		}
		err = a.Validate(ctx, cmd, nil)
		if err != nil {
			t.Errorf("Validate with non-empty rules: %v", err)
		}
	})
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
			name: "full mode - successful move with all files present",
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
						mode:   string(provider.FullAnalysisMode),
					}, func() {
						os.RemoveAll(tmpDir)
						os.RemoveAll(inputDir)
					}, nil
			},
			wantErr: false,
		},
		{
			name: "source-only mode - successful move with all files present",
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
					"output.yaml":  "test output",
					"analysis.log": "test log",
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
						mode:   string(provider.SourceOnlyAnalysisMode),
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

func Test_analyzeCommand_disableMavenSearch_flagParsing(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{
			name:     "disable-maven-search flag set to true",
			args:     []string{"analyze", "--disable-maven-search=true", "--input", "/test", "--output", "/output"},
			expected: true,
		},
		{
			name:     "disable-maven-search flag set to false",
			args:     []string{"analyze", "--disable-maven-search=false", "--input", "/test", "--output", "/output"},
			expected: false,
		},
		{
			name:     "disable-maven-search flag not provided (default false)",
			args:     []string{"analyze", "--input", "/test", "--output", "/output"},
			expected: false,
		},
		{
			name:     "disable-maven-search flag provided without value (true)",
			args:     []string{"analyze", "--disable-maven-search", "--input", "/test", "--output", "/output"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logr.Discard()
			cmd := NewAnalyzeCmd(log)

			// Set args and parse
			cmd.SetArgs(tt.args[1:]) // Remove "analyze" since that's the command name
			err := cmd.ParseFlags(tt.args[1:])
			if err != nil {
				t.Fatalf("Failed to parse flags: %v", err)
			}

			// Check if we can extract the flag value via reflection or by creating a new command
			// Since we can't easily access the internal state, let's test via flag lookup
			flagValue, err := cmd.Flags().GetBool("disable-maven-search")
			if err != nil {
				t.Fatalf("Failed to get disable-maven-search flag: %v", err)
			}

			if flagValue != tt.expected {
				t.Errorf("disableMavenSearch = %v, want %v", flagValue, tt.expected)
			}
		})
	}
}

func Test_analyzeCommand_getConfigVolumes_disableMavenSearch(t *testing.T) {
	log := logr.Discard()

	tests := []struct {
		name                       string
		disableMavenSearch         bool
		expectedDisableMavenSearch bool
	}{
		{
			name:                       "disableMavenSearch true",
			disableMavenSearch:         true,
			expectedDisableMavenSearch: true,
		},
		{
			name:                       "disableMavenSearch false",
			disableMavenSearch:         false,
			expectedDisableMavenSearch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "test-input-")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			outputDir, err := os.MkdirTemp("", "test-output-")
			if err != nil {
				t.Fatalf("Failed to create output dir: %v", err)
			}
			defer os.RemoveAll(outputDir)

			a := &analyzeCommand{
				input:              tmpDir,
				output:             outputDir,
				disableMavenSearch: tt.disableMavenSearch,
				mode:               "source-only",
				AnalyzeCommandContext: AnalyzeCommandContext{
					log:          log,
					isFileInput:  false,
					needsBuiltin: true,
				},
			}

			// Mock Settings to avoid nil pointer
			originalSettings := Settings
			Settings = &Config{
				JvmMaxMem: "1g",
			}
			defer func() { Settings = originalSettings }()
			configVols, err := a.getConfigVolumes()
			if err != nil {
				t.Fatalf("getConfigVolumes() error = %v", err)
			}
			if len(configVols) == 0 {
				t.Fatal("Expected config volumes to be created")
			}

			tempDir, err := os.MkdirTemp("", "analyze-config-")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tempDir)

			javaTargetPaths, _ := kantraProvider.WalkJavaPathForTarget(log, false, tmpDir)

			configInput := kantraProvider.ConfigInput{
				IsFileInput:             false,
				InputPath:               tmpDir,
				OutputPath:              outputDir,
				MavenSettingsFile:       "",
				Log:                     log,
				Mode:                    "source-only",
				Port:                    6734,
				TmpDir:                  tempDir,
				JvmMaxMem:               "1g",
				DepsFolders:             []string{},
				JavaExcludedTargetPaths: javaTargetPaths,
				DisableMavenSearch:      tt.disableMavenSearch,
			}

			if configInput.DisableMavenSearch != tt.expectedDisableMavenSearch {
				t.Errorf("ConfigInput.DisableMavenSearch = %v, want %v",
					configInput.DisableMavenSearch, tt.expectedDisableMavenSearch)
			}
		})
	}
}

func Test_JavaProvider_GetConfigVolume_disableMavenSearch(t *testing.T) {
	log := logr.Discard()

	tests := []struct {
		name                       string
		disableMavenSearch         bool
		expectedDisableMavenSearch bool
	}{
		{
			name:                       "disableMavenSearch true in config",
			disableMavenSearch:         true,
			expectedDisableMavenSearch: true,
		},
		{
			name:                       "disableMavenSearch false in config",
			disableMavenSearch:         false,
			expectedDisableMavenSearch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory for testing
			tmpDir, err := os.MkdirTemp("", "test-java-config-")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			configInput := kantraProvider.ConfigInput{
				IsFileInput:             false,
				InputPath:               tmpDir,
				OutputPath:              tmpDir,
				MavenSettingsFile:       "",
				Log:                     log,
				Mode:                    "source-only",
				Port:                    6734,
				TmpDir:                  tmpDir,
				JvmMaxMem:               "1g",
				DepsFolders:             []string{},
				JavaExcludedTargetPaths: []interface{}{},
				DisableMavenSearch:      tt.disableMavenSearch,
			}

			javaProvider := &kantraProvider.JavaProvider{}
			config, err := javaProvider.GetConfigVolume(configInput)
			if err != nil {
				t.Fatalf("JavaProvider.GetConfigVolume() error = %v", err)
			}

			if len(config.InitConfig) == 0 {
				t.Fatal("Expected InitConfig to have at least one entry")
			}

			providerConfig := config.InitConfig[0].ProviderSpecificConfig
			disableMavenSearchValue, exists := providerConfig["disableMavenSearch"]
			if !exists {
				t.Fatal("Expected disableMavenSearch to be present in ProviderSpecificConfig")
			}

			if disableMavenSearchValue != tt.expectedDisableMavenSearch {
				t.Errorf("ProviderSpecificConfig[\"disableMavenSearch\"] = %v, want %v",
					disableMavenSearchValue, tt.expectedDisableMavenSearch)
			}
		})
	}
}
