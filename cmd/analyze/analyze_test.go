package analyze

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

func Test_analyzeCommand_Validate_sourceAndTargetLabels(t *testing.T) {
	ctx := context.Background()
	cmd := &cobra.Command{}

	// Setup: temp kantra dir with rulesets containing source and target labels
	mkKantraDirWithRules := func(t *testing.T, sourceLabel, targetLabel string) string {
		t.Helper()
		tmpDir, err := os.MkdirTemp("", "kantra-validate-labels-")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(tmpDir) })
		rulesetsDir := filepath.Join(tmpDir, "rulesets", "java")
		if err := os.MkdirAll(rulesetsDir, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		ruleContent := fmt.Sprintf(`
- category: mandatory
  labels:
  - konveyor.io/source=%s
  - konveyor.io/target=%s
  ruleID: test-rule-01
`, sourceLabel, targetLabel)
		if err := os.WriteFile(filepath.Join(rulesetsDir, "rules.yaml"), []byte(ruleContent), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		return tmpDir
	}

	t.Run("runLocal true with valid source and target passes", func(t *testing.T) {
		kantraDir := mkKantraDirWithRules(t, "java-ee", "quarkus")
		outDir := filepath.Join(os.TempDir(), fmt.Sprintf("kantra-validate-out-%d", time.Now().UnixNano()))
		t.Cleanup(func() { _ = os.RemoveAll(outDir) })
		if err := os.MkdirAll(outDir, 0755); err != nil {
			t.Fatalf("MkdirAll output: %v", err)
		}

		a := &analyzeCommand{
			output:                outDir,
			input:                 "",
			mode:                  string(provider.FullAnalysisMode),
			runLocal:              true,
			sources:               []string{"java-ee"},
			targets:               []string{"quarkus"},
			enableDefaultRulesets: true,
			overwrite:             true,
			AnalyzeCommandContext: AnalyzeCommandContext{
				log:       logr.Discard(),
				kantraDir: kantraDir,
			},
		}
		err := a.Validate(ctx, cmd, nil)
		if err != nil {
			t.Errorf("Validate with valid source and target: %v", err)
		}
	})

	t.Run("runLocal true with unknown source returns error", func(t *testing.T) {
		kantraDir := mkKantraDirWithRules(t, "java-ee", "quarkus")
		outDir := filepath.Join(os.TempDir(), fmt.Sprintf("kantra-validate-out-%d", time.Now().UnixNano()))
		t.Cleanup(func() { _ = os.RemoveAll(outDir) })
		if err := os.MkdirAll(outDir, 0755); err != nil {
			t.Fatalf("MkdirAll output: %v", err)
		}

		a := &analyzeCommand{
			output:                outDir,
			input:                 "",
			mode:                  string(provider.FullAnalysisMode),
			runLocal:              true,
			sources:               []string{"unknown-source"},
			enableDefaultRulesets: true,
			overwrite:             true,
			AnalyzeCommandContext: AnalyzeCommandContext{
				log:       logr.Discard(),
				kantraDir: kantraDir,
			},
		}
		err := a.Validate(ctx, cmd, nil)
		if err == nil {
			t.Fatal("Validate expected error for unknown source")
		}
		if !strings.Contains(err.Error(), "unknown source") || !strings.Contains(err.Error(), "unknown-source") {
			t.Errorf("error = %v, want 'unknown source' and 'unknown-source'", err)
		}
	})

	t.Run("runLocal true with unknown target returns error", func(t *testing.T) {
		kantraDir := mkKantraDirWithRules(t, "java-ee", "quarkus")
		outDir := filepath.Join(os.TempDir(), fmt.Sprintf("kantra-validate-out-%d", time.Now().UnixNano()))
		t.Cleanup(func() { _ = os.RemoveAll(outDir) })
		if err := os.MkdirAll(outDir, 0755); err != nil {
			t.Fatalf("MkdirAll output: %v", err)
		}

		a := &analyzeCommand{
			output:                outDir,
			input:                 "",
			mode:                  string(provider.FullAnalysisMode),
			runLocal:              true,
			targets:               []string{"unknown-target"},
			enableDefaultRulesets: true,
			overwrite:             true,
			AnalyzeCommandContext: AnalyzeCommandContext{
				log:       logr.Discard(),
				kantraDir: kantraDir,
			},
		}
		err := a.Validate(ctx, cmd, nil)
		if err == nil {
			t.Fatal("Validate expected error for unknown target")
		}
		if !strings.Contains(err.Error(), "unknown target") || !strings.Contains(err.Error(), "unknown-target") {
			t.Errorf("error = %v, want 'unknown target' and 'unknown-target'", err)
		}
	})

}

func TestParseLabelLines(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		expected []string
	}{
		{
			name:     "empty string returns empty slice",
			raw:      "",
			expected: []string{},
		},
		{
			name:     "single line no newline",
			raw:      "java-ee",
			expected: []string{"java-ee"},
		},
		{
			name:     "multiple lines",
			raw:      "available source technologies:\njava-ee\nquarkus\n",
			expected: []string{"available source technologies:", "java-ee", "quarkus"},
		},
		{
			name:     "trims leading and trailing whitespace",
			raw:      "  java-ee  \n  quarkus  ",
			expected: []string{"java-ee", "quarkus"},
		},
		{
			name:     "skips empty lines",
			raw:      "java-ee\n\nquarkus\n\n",
			expected: []string{"java-ee", "quarkus"},
		},
		{
			name:     "handles Windows line endings",
			raw:      "java-ee\r\nquarkus\r\n",
			expected: []string{"java-ee", "quarkus"},
		},
		{
			name:     "mixed content with header",
			raw:      "available target technologies:\ncloud-readiness\nquarkus\n",
			expected: []string{"available target technologies:", "cloud-readiness", "quarkus"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLabelLines(tt.raw)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("parseLabelLines() = %v, want %v", got, tt.expected)
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
func Test_JavaProvider_GetConfig_disableMavenSearch(t *testing.T) {
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
			javaProvider := &kantraProvider.JavaProvider{}

			// disableMavenSearch is only set in local mode
			config, err := javaProvider.GetConfig(kantraProvider.ModeLocal, kantraProvider.BaseOptions{
				Location:     "/tmp/project",
				AnalysisMode: "source-only",
				KantraDir:    "/home/user/.kantra",
			}, kantraProvider.JavaOptions{
				DisableMavenSearch: tt.disableMavenSearch,
			})
			if err != nil {
				t.Fatalf("JavaProvider.GetConfig() error = %v", err)
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

func Test_analyzeCommand_Validate_staticReportPath(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T) (string, func())
		wantErr     bool
		errContains string
	}{
		{
			name: "empty staticReportPath is valid",
			setupFunc: func(t *testing.T) (string, func()) {
				return "", func() {}
			},
			wantErr: false,
		},
		{
			name: "valid directory path is accepted",
			setupFunc: func(t *testing.T) (string, func()) {
				tmpDir, err := os.MkdirTemp("", "test-report-")
				if err != nil {
					t.Fatal(err)
				}
				return tmpDir, func() { os.RemoveAll(tmpDir) }
			},
			wantErr: false,
		},
		{
			name: "non-existent path returns error",
			setupFunc: func(t *testing.T) (string, func()) {
				return "/nonexistent/path/to/report", func() {}
			},
			wantErr:     true,
			errContains: "failed to stat static report path",
		},
		{
			name: "file path (not directory) returns error",
			setupFunc: func(t *testing.T) (string, func()) {
				tmpDir, err := os.MkdirTemp("", "test-report-")
				if err != nil {
					t.Fatal(err)
				}
				filePath := filepath.Join(tmpDir, "not-a-dir.txt")
				err = os.WriteFile(filePath, []byte("test"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					t.Fatal(err)
				}
				return filePath, func() { os.RemoveAll(tmpDir) }
			},
			wantErr:     true,
			errContains: "is not a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reportPath, cleanup := tt.setupFunc(t)
			defer cleanup()

			// Create minimal valid analyze command setup
			tmpInput, err := os.MkdirTemp("", "test-input-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(tmpInput)

			tmpOutput := filepath.Join(os.TempDir(), fmt.Sprintf("test-output-%d", time.Now().UnixNano()))

			tmpKantraDir, err := os.MkdirTemp("", "test-kantra-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(tmpKantraDir)

			// Create required kantra subdirectories
			for _, sub := range []string{"rulesets/default/generated", "jdtls/bin", "java-bundles"} {
				os.MkdirAll(filepath.Join(tmpKantraDir, sub), 0755)
			}
			os.WriteFile(filepath.Join(tmpKantraDir, "fernflower.jar"), []byte(""), 0644)

			a := &analyzeCommand{
				staticReportPath:     reportPath,
				input:                tmpInput,
				output:               tmpOutput,
				mode:                 "full",
				enableDefaultRulesets: true,
				AnalyzeCommandContext: AnalyzeCommandContext{
					log:       logr.Discard(),
					kantraDir: tmpKantraDir,
				},
			}

			cmd := &cobra.Command{}
			err = a.Validate(context.Background(), cmd, nil)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q but got nil", tt.errContains)
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			// Cleanup output dir if created
			os.RemoveAll(tmpOutput)
		})
	}
}

func Test_staticReportPathFlagParsing(t *testing.T) {
	log := logr.Discard()
	cmd := NewAnalyzeCmd(log)

	// Verify the flag exists and has the correct default
	flag := cmd.Flags().Lookup("static-report-path")
	if flag == nil {
		t.Fatal("expected --static-report-path flag to exist")
	}
	if flag.DefValue != "" {
		t.Errorf("expected default value to be empty, got %q", flag.DefValue)
	}
	if flag.Usage != "override the default static report template location" {
		t.Errorf("unexpected usage text: %q", flag.Usage)
	}
}
