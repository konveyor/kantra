package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	outputv1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/spf13/cobra"
)

func TestUnmarshalProfile(t *testing.T) {
	tests := []struct {
		name        string
		profileDir  string
		setupFunc   func() (string, func(), error)
		wantProfile *AnalysisProfile
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "empty path",
			profileDir:  "",
			setupFunc:   func() (string, func(), error) { return "", func() {}, nil },
			wantProfile: nil,
			wantErr:     false,
		},
		{
			name: "valid profile",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, err
				}

				profileContent := `
mode:
  withDeps: true
scope:
  withKnownLibs: true
  packages:
    included:
      - "com.example"
rules:
  labels:
    included:
      - "test-label"
`
				profilePath := filepath.Join(tmpDir, "profile.yaml")
				err = os.WriteFile(profilePath, []byte(profileContent), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				cleanup := func() { os.RemoveAll(tmpDir) }
				return profilePath, cleanup, nil
			},
			wantProfile: &AnalysisProfile{
				Mode: AnalysisMode{WithDeps: true},
				Scope: AnalysisScope{
					WithKnownLibs: true,
					Packages: PackageSelector{
						Included: []string{"com.example"},
					},
				},
				Rules: AnalysisRules{
					Labels: LabelSelector{
						Included: []string{"test-label"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "profile with ID and all fields",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, err
				}

				profileContent := `
id: 123
name: "Complete Profile"
mode:
  withDeps: false
scope:
  withKnownLibs: false
  packages:
    included:
      - "com.example.included"
    excluded:
      - "com.example.excluded"
rules:
  labels:
    included:
      - "included-label"
    excluded:
      - "excluded-label"
`
				profilePath := filepath.Join(tmpDir, "profile.yaml")
				err = os.WriteFile(profilePath, []byte(profileContent), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				cleanup := func() { os.RemoveAll(tmpDir) }
				return profilePath, cleanup, nil
			},
			wantProfile: &AnalysisProfile{
				ID:   123,
				Name: "Complete Profile",
				Mode: AnalysisMode{WithDeps: false},
				Scope: AnalysisScope{
					WithKnownLibs: false,
					Packages: PackageSelector{
						Included: []string{"com.example.included"},
						Excluded: []string{"com.example.excluded"},
					},
				},
				Rules: AnalysisRules{
					Labels: LabelSelector{
						Included: []string{"included-label"},
						Excluded: []string{"excluded-label"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty profile file",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, err
				}

				profilePath := filepath.Join(tmpDir, "profile.yaml")
				err = os.WriteFile(profilePath, []byte(""), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				cleanup := func() { os.RemoveAll(tmpDir) }
				return profilePath, cleanup, nil
			},
			wantProfile: &AnalysisProfile{},
			wantErr:     false,
		},
		{
			name: "profile with only mode",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, err
				}

				profileContent := `
mode:
  withDeps: true
`
				profilePath := filepath.Join(tmpDir, "profile.yaml")
				err = os.WriteFile(profilePath, []byte(profileContent), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				cleanup := func() { os.RemoveAll(tmpDir) }
				return profilePath, cleanup, nil
			},
			wantProfile: &AnalysisProfile{
				Mode: AnalysisMode{WithDeps: true},
			},
			wantErr: false,
		},
		{
			name: "directory instead of file",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, err
				}

				profileDir := filepath.Join(tmpDir, "profile.yaml")
				err = os.Mkdir(profileDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				cleanup := func() { os.RemoveAll(tmpDir) }
				return profileDir, cleanup, nil
			},
			wantProfile: nil,
			wantErr:     true,
			errMsg:      "is a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Setup failed: %v", err)
			}
			defer cleanup()

			got, err := UnmarshalProfile(path)

			if tt.wantErr {
				if err == nil {
					t.Errorf("UnmarshalProfile() expected error but got none")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("UnmarshalProfile() error = %v, want error containing %v", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("UnmarshalProfile() unexpected error = %v", err)
				return
			}

			if tt.wantProfile == nil && got != nil {
				t.Errorf("UnmarshalProfile() = %v, want nil", got)
				return
			}

			if tt.wantProfile != nil && got == nil {
				t.Errorf("UnmarshalProfile() = nil, want %v", tt.wantProfile)
				return
			}

			if tt.wantProfile != nil && got != nil {
				if got.ID != tt.wantProfile.ID {
					t.Errorf("UnmarshalProfile() ID = %v, want %v", got.ID, tt.wantProfile.ID)
				}
				if got.Name != tt.wantProfile.Name {
					t.Errorf("UnmarshalProfile() Name = %v, want %v", got.Name, tt.wantProfile.Name)
				}
				if got.Mode.WithDeps != tt.wantProfile.Mode.WithDeps {
					t.Errorf("UnmarshalProfile() Mode.WithDeps = %v, want %v", got.Mode.WithDeps, tt.wantProfile.Mode.WithDeps)
				}
				if got.Scope.WithKnownLibs != tt.wantProfile.Scope.WithKnownLibs {
					t.Errorf("UnmarshalProfile() Scope.WithKnownLibs = %v, want %v", got.Scope.WithKnownLibs, tt.wantProfile.Scope.WithKnownLibs)
				}
				if len(got.Scope.Packages.Included) != len(tt.wantProfile.Scope.Packages.Included) {
					t.Errorf("UnmarshalProfile() Scope.Packages.Included length = %v, want %v", len(got.Scope.Packages.Included), len(tt.wantProfile.Scope.Packages.Included))
				}
				if len(got.Scope.Packages.Excluded) != len(tt.wantProfile.Scope.Packages.Excluded) {
					t.Errorf("UnmarshalProfile() Scope.Packages.Excluded length = %v, want %v", len(got.Scope.Packages.Excluded), len(tt.wantProfile.Scope.Packages.Excluded))
				}
				if len(got.Rules.Labels.Included) != len(tt.wantProfile.Rules.Labels.Included) {
					t.Errorf("UnmarshalProfile() Rules.Labels.Included length = %v, want %v", len(got.Rules.Labels.Included), len(tt.wantProfile.Rules.Labels.Included))
				}
				if len(got.Rules.Labels.Excluded) != len(tt.wantProfile.Rules.Labels.Excluded) {
					t.Errorf("UnmarshalProfile() Rules.Labels.Excluded length = %v, want %v", len(got.Rules.Labels.Excluded), len(tt.wantProfile.Rules.Labels.Excluded))
				}
			}
		})
	}
}

func TestSetSettingsFromProfile(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func() (string, *cobra.Command, func(), error)
		wantErr   bool
		errMsg    string
		validate  func(*ProfileSettings, *testing.T)
	}{
		{
			name: "profile with all settings",
			setupFunc: func() (string, *cobra.Command, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, nil, err
				}

				konveyorDir := filepath.Join(tmpDir, ".konveyor", "profiles", "test-profile")
				err = os.MkdirAll(konveyorDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, nil, err
				}

				rulesDir := filepath.Join(konveyorDir, "rules", "test-rule")
				err = os.MkdirAll(rulesDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, nil, err
				}
				if err := os.WriteFile(filepath.Join(rulesDir, "rule.yaml"), []byte("rule: test"), 0644); err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, nil, err
				}

				profileContent := `
mode:
  withDeps: true
scope:
  withKnownLibs: true
  packages:
    included:
      - "com.example"
rules:
  labels:
    included:
      - "test-label"
`
				profilePath := filepath.Join(konveyorDir, "profile.yaml")
				err = os.WriteFile(profilePath, []byte(profileContent), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, nil, err
				}

				cmd := &cobra.Command{}
				cmd.Flags().String("input", "", "input flag")
				cmd.Flags().String("mode", "", "mode flag")
				cmd.Flags().Bool("analyze-known-libraries", false, "analyze known libraries flag")
				cmd.Flags().String("incident-selector", "", "incident selector flag")
				cmd.Flags().String("label-selector", "", "label selector flag")
				cmd.Flags().StringSlice("rules", []string{}, "rules flag")

				cleanup := func() { os.RemoveAll(tmpDir) }
				return profilePath, cmd, cleanup, nil
			},
			wantErr: false,
			validate: func(settings *ProfileSettings, t *testing.T) {
				if settings.Mode != string(provider.FullAnalysisMode) {
					t.Errorf("Expected mode %s, got %s", provider.FullAnalysisMode, settings.Mode)
				}
				if !settings.AnalyzeKnownLibraries {
					t.Errorf("Expected AnalyzeKnownLibraries to be true")
				}
				if settings.IncidentSelector != "package=com.example" {
					t.Errorf("Expected IncidentSelector 'package=com.example', got '%s'", settings.IncidentSelector)
				}
				if settings.LabelSelector != "(test-label)" {
					t.Errorf("Expected LabelSelector '(test-label)', got '%s'", settings.LabelSelector)
				}
				if len(settings.Rules) == 0 {
					t.Errorf("Expected rules to be populated")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, cmd, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Setup failed: %v", err)
			}
			defer cleanup()

			settings := &ProfileSettings{
				EnableDefaultRulesets: true, // default value
			}
			err = SetSettingsFromProfile(path, cmd, settings)

			if tt.wantErr {
				if err == nil {
					t.Errorf("SetSettingsFromProfile() expected error but got none")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("SetSettingsFromProfile() error = %v, want error containing %v", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("SetSettingsFromProfile() unexpected error = %v", err)
				return
			}

			if tt.validate != nil {
				tt.validate(settings, t)
			}
		})
	}
}

func TestProfileHasRules(t *testing.T) {
	// ProfileHasRules(rulesDir) expects the rules directory path, not the profile file path.
	tests := []struct {
		name         string
		setupFunc    func() (rulesDir string, cleanup func(), err error)
		wantHasRules bool
	}{
		{
			name: "empty path returns false",
			setupFunc: func() (string, func(), error) {
				return "", func() {}, nil
			},
			wantHasRules: false,
		},
		{
			name: "path to profile with no rules dir returns false",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, err
				}
				_ = os.WriteFile(filepath.Join(tmpDir, "profile.yaml"), []byte("mode: {}"), 0644)
				rulesDir := filepath.Join(tmpDir, "rules") // does not exist
				return rulesDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantHasRules: false,
		},
		{
			name: "path to profile with rules dir but no yaml returns false",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, err
				}
				rulesDir := filepath.Join(tmpDir, "rules")
				if err := os.MkdirAll(rulesDir, 0755); err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				_ = os.WriteFile(filepath.Join(rulesDir, "readme.txt"), []byte("text"), 0644)
				return rulesDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantHasRules: false,
		},
		{
			name: "path to profile with rules dir containing yaml returns true",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, err
				}
				rulesDir := filepath.Join(tmpDir, "rules")
				if err := os.MkdirAll(rulesDir, 0755); err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				if err := os.WriteFile(filepath.Join(rulesDir, "rule.yaml"), []byte("rule: test"), 0644); err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				return rulesDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantHasRules: true,
		},
		{
			name: "path to profile with rules dir containing yml returns true",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, err
				}
				rulesDir := filepath.Join(tmpDir, "rules")
				if err := os.MkdirAll(rulesDir, 0755); err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				if err := os.WriteFile(filepath.Join(rulesDir, "rule.yml"), []byte("rule: test"), 0644); err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				return rulesDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantHasRules: true,
		},
		{
			name: "path to profile with rules subdir containing yaml returns true",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, err
				}
				subDir := filepath.Join(tmpDir, "rules", "files")
				if err := os.MkdirAll(subDir, 0755); err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				if err := os.WriteFile(filepath.Join(subDir, "rule.yaml"), []byte("rule: test"), 0644); err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				return filepath.Join(tmpDir, "rules"), func() { os.RemoveAll(tmpDir) }, nil
			},
			wantHasRules: true,
		},
		{
			name: "rules dir with subdir that causes WalkDir error returns false",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, err
				}
				rulesDir := filepath.Join(tmpDir, "rules")
				if err := os.MkdirAll(rulesDir, 0755); err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				noAccessDir := filepath.Join(rulesDir, "noaccess")
				if err := os.Mkdir(noAccessDir, 0000); err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				cleanup := func() {
					_ = os.Chmod(noAccessDir, 0755)
					os.RemoveAll(tmpDir)
				}
				return rulesDir, cleanup, nil
			},
			wantHasRules: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rulesDir, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Setup failed: %v", err)
			}
			defer cleanup()
			got := ProfileHasRules(rulesDir)
			if got != tt.wantHasRules {
				t.Errorf("ProfileHasRules(%q) = %v, want %v", rulesDir, got, tt.wantHasRules)
			}
		})
	}
}

func TestSetSettingsFromProfile_EnableDefaultRulesets(t *testing.T) {
	makeProfileWithKonveyorPath := func(t *testing.T, profileYAML string) (profilePath string, cleanup func()) {
		t.Helper()
		tmpDir, err := os.MkdirTemp("", "test-profile-")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		// Path must contain ".konveyor" for SetSettingsFromProfile to accept it
		konveyorDir := filepath.Join(tmpDir, "app", ".konveyor", "profiles", "p")
		if err := os.MkdirAll(konveyorDir, 0755); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("MkdirAll: %v", err)
		}
		profilePath = filepath.Join(konveyorDir, "profile.yaml")
		if err := os.WriteFile(profilePath, []byte(profileYAML), 0644); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("WriteFile: %v", err)
		}
		return profilePath, func() { os.RemoveAll(tmpDir) }
	}

	tests := []struct {
		name                      string
		profileYAML               string
		setupFlags                func(*cobra.Command)
		wantEnableDefaultRulesets bool
		wantErr                   bool
	}{
		{
			name:        "enable-default-rulesets flag set to true",
			profileYAML: "mode: {}\nrules:\n  labels:\n    included: []",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().Bool("enable-default-rulesets", false, "")
				_ = cmd.Flags().Set("enable-default-rulesets", "true")
				cmd.Flags().Lookup("enable-default-rulesets").Changed = true
			},
			wantEnableDefaultRulesets: true,
		},
		{
			name:        "enable-default-rulesets flag set to false",
			profileYAML: "mode: {}\nrules:\n  labels:\n    included: []",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().Bool("enable-default-rulesets", true, "")
				_ = cmd.Flags().Set("enable-default-rulesets", "false")
				cmd.Flags().Lookup("enable-default-rulesets").Changed = true
			},
			wantEnableDefaultRulesets: false,
		},
		{
			name:        "no enable-default-rulesets flag; target flag set",
			profileYAML: "mode: {}\nrules:\n  labels:\n    included: []",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().Bool("enable-default-rulesets", true, "")
				cmd.Flags().String("target", "", "")
				cmd.Flags().String("source", "", "")
				_ = cmd.Flags().Set("target", "eap8")
				cmd.Flags().Lookup("target").Changed = true
			},
			wantEnableDefaultRulesets: true,
		},
		{
			name:        "no enable-default-rulesets flag; source flag set",
			profileYAML: "mode: {}\nrules:\n  labels:\n    included: []",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().Bool("enable-default-rulesets", true, "")
				cmd.Flags().String("target", "", "")
				cmd.Flags().String("source", "", "")
				_ = cmd.Flags().Set("source", "java")
				cmd.Flags().Lookup("source").Changed = true
			},
			wantEnableDefaultRulesets: true,
		},
		{
			name:        "no enable-default-rulesets flag; profile has default konveyor target label",
			profileYAML: "mode: {}\nrules:\n  labels:\n    included:\n      - \"" + outputv1.TargetTechnologyLabel + "=eap8\"",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().Bool("enable-default-rulesets", true, "")
				cmd.Flags().String("target", "", "")
				cmd.Flags().String("source", "", "")
			},
			wantEnableDefaultRulesets: true,
		},
		{
			name:        "no enable-default-rulesets flag; profile has exact konveyor target label (no value)",
			profileYAML: "mode: {}\nrules:\n  labels:\n    included:\n      - \"" + outputv1.TargetTechnologyLabel + "\"",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().Bool("enable-default-rulesets", true, "")
				cmd.Flags().String("target", "", "")
				cmd.Flags().String("source", "", "")
			},
			wantEnableDefaultRulesets: true,
		},
		{
			name:        "no enable-default-rulesets flag; profile has default konveyor source label",
			profileYAML: "mode: {}\nrules:\n  labels:\n    included:\n      - \"" + outputv1.SourceTechnologyLabel + "=java\"",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().Bool("enable-default-rulesets", true, "")
				cmd.Flags().String("target", "", "")
				cmd.Flags().String("source", "", "")
			},
			wantEnableDefaultRulesets: true,
		},
		{
			name:        "no enable-default-rulesets flag; profile has exact konveyor source label (no value)",
			profileYAML: "mode: {}\nrules:\n  labels:\n    included:\n      - \"" + outputv1.SourceTechnologyLabel + "\"",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().Bool("enable-default-rulesets", true, "")
				cmd.Flags().String("target", "", "")
				cmd.Flags().String("source", "", "")
			},
			wantEnableDefaultRulesets: true,
		},
		{
			name:        "no enable-default-rulesets/target/source; profile has no default labels",
			profileYAML: "mode: {}\nrules:\n  labels:\n    included:\n      - \"custom-label\"",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().Bool("enable-default-rulesets", true, "")
				cmd.Flags().String("target", "", "")
				cmd.Flags().String("source", "", "")
			},
			wantEnableDefaultRulesets: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profilePath, cleanup := makeProfileWithKonveyorPath(t, tt.profileYAML)
			defer cleanup()

			cmd := &cobra.Command{}
			cmd.Flags().String("input", "", "")
			cmd.Flags().String("mode", "", "")
			cmd.Flags().Bool("analyze-known-libraries", false, "")
			cmd.Flags().String("incident-selector", "", "")
			cmd.Flags().String("label-selector", "", "")
			cmd.Flags().StringSlice("rules", nil, "")
			tt.setupFlags(cmd)

			settings := &ProfileSettings{
				EnableDefaultRulesets: true,
			}
			err := SetSettingsFromProfile(profilePath, cmd, settings)
			if tt.wantErr {
				if err == nil {
					t.Fatal("SetSettingsFromProfile expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("SetSettingsFromProfile: %v", err)
			}
			if settings.EnableDefaultRulesets != tt.wantEnableDefaultRulesets {
				t.Errorf("EnableDefaultRulesets = %v, want %v", settings.EnableDefaultRulesets, tt.wantEnableDefaultRulesets)
			}
		})
	}
}

func TestGetRulesInProfile(t *testing.T) {
	tests := []struct {
		name       string
		setupFunc  func() (string, func(), error)
		wantRules  []string
		wantErr    bool
		profileDir string
		errMsg     string
	}{
		{
			name:       "empty profile dir",
			profileDir: "",
			setupFunc:  func() (string, func(), error) { return "", func() {}, nil },
			wantRules:  nil,
			wantErr:    false,
		},
		{
			name: "profile with rules",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, err
				}

				rulesDir := filepath.Join(tmpDir, "rules")
				err = os.MkdirAll(rulesDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				// At least one .yaml file required for GetRulesInProfile to return rule paths
				if err := os.WriteFile(filepath.Join(rulesDir, "ruleset.yaml"), []byte("rules: []"), 0644); err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				rule1Dir := filepath.Join(rulesDir, "rule1")
				rule2Dir := filepath.Join(rulesDir, "rule2")
				err = os.MkdirAll(rule1Dir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				err = os.MkdirAll(rule2Dir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				cleanup := func() { os.RemoveAll(tmpDir) }
				return tmpDir, cleanup, nil
			},
			wantRules: []string{"rule1", "rule2"}, // Should return the rule directories
			wantErr:   false,
		},
		{
			name: "rules directory with subdirs but no yaml returns nil",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, err
				}
				rulesDir := filepath.Join(tmpDir, "rules")
				if err := os.MkdirAll(rulesDir, 0755); err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				if err := os.Mkdir(filepath.Join(rulesDir, "rule1"), 0755); err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				if err := os.WriteFile(filepath.Join(rulesDir, "readme.txt"), []byte("no yaml"), 0644); err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				return tmpDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantRules: nil,
			wantErr:   false,
		},
		{
			name: "rules directory with files (not directories)",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, err
				}

				rulesDir := filepath.Join(tmpDir, "rules")
				err = os.Mkdir(rulesDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				// Create files (not directories) in rules dir
				err = os.WriteFile(filepath.Join(rulesDir, "rule1.yaml"), []byte("content"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				cleanup := func() { os.RemoveAll(tmpDir) }
				return tmpDir, cleanup, nil
			},
			wantRules: []string{}, // Should return empty slice since no directories
			wantErr:   false,
		},
		{
			name: "rules directory with mixed files and directories",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, err
				}

				rulesDir := filepath.Join(tmpDir, "rules")
				err = os.Mkdir(rulesDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				// Create a file
				err = os.WriteFile(filepath.Join(rulesDir, "rule1.yaml"), []byte("content"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				// Create directories
				err = os.Mkdir(filepath.Join(rulesDir, "rule-dir1"), 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				err = os.Mkdir(filepath.Join(rulesDir, "rule-dir2"), 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				cleanup := func() { os.RemoveAll(tmpDir) }
				return tmpDir, cleanup, nil
			},
			wantRules: []string{"rule-dir1", "rule-dir2"}, // Should only return directories
			wantErr:   false,
		},
		{
			name: "rules path is a file not directory",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, err
				}

				// Create a file named "rules" instead of directory
				rulesFile := filepath.Join(tmpDir, "rules")
				err = os.WriteFile(rulesFile, []byte("content"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				cleanup := func() { os.RemoveAll(tmpDir) }
				return tmpDir, cleanup, nil
			},
			wantRules: nil,
			wantErr:   true,
			errMsg:    "is not a directory",
		},
		{
			name: "permission denied on rules directory",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, err
				}

				rulesDir := filepath.Join(tmpDir, "rules")
				err = os.Mkdir(rulesDir, 0000) // No permissions
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				cleanup := func() {
					os.Chmod(rulesDir, 0755) // Restore permissions for cleanup
					os.RemoveAll(tmpDir)
				}
				return tmpDir, cleanup, nil
			},
			// When rules dir has no read permission, ProfileHasRules fails and GetRulesInProfile returns nil, nil
			wantRules: nil,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profileDir := tt.profileDir
			cleanup := func() {}
			var err error

			if tt.setupFunc != nil {
				profileDir, cleanup, err = tt.setupFunc()
				if err != nil {
					t.Fatalf("Setup failed: %v", err)
				}
			}
			defer cleanup()

			got, err := GetRulesInProfile(profileDir)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetRulesInProfile() expected error but got none")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("GetRulesInProfile() error = %v, want error containing %v", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("GetRulesInProfile() unexpected error = %v", err)
				return
			}

			if tt.name == "rules directory with mixed files and directories" {
				if len(got) != 2 {
					t.Errorf("GetRulesInProfile() returned %d rules, want 2", len(got))
					return
				}
				// Check that we got the expected directory names (order may vary)
				foundDirs := make(map[string]bool)
				for _, rule := range got {
					dir := filepath.Base(rule)
					foundDirs[dir] = true
				}
				if !foundDirs["rule-dir1"] || !foundDirs["rule-dir2"] {
					t.Errorf("GetRulesInProfile() = %v, want directories rule-dir1 and rule-dir2", got)
				}
			} else if tt.name == "profile with rules" {
				if len(got) != 2 {
					t.Errorf("GetRulesInProfile() returned %d rules, want 2", len(got))
					return
				}
				// Check that we got the expected directory names (order may vary)
				foundDirs := make(map[string]bool)
				for _, rule := range got {
					dir := filepath.Base(rule)
					foundDirs[dir] = true
				}
				if !foundDirs["rule1"] || !foundDirs["rule2"] {
					t.Errorf("GetRulesInProfile() = %v, want directories rule1 and rule2", got)
				}
			} else if tt.wantRules == nil && got != nil {
				t.Errorf("GetRulesInProfile() = %v, want nil", got)
			} else if tt.wantRules != nil && len(got) != len(tt.wantRules) {
				t.Errorf("GetRulesInProfile() returned %d rules, want %d", len(got), len(tt.wantRules))
			}
		})
	}
}

func TestFindSingleProfile(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func() (string, func(), error)
		wantProfile string
		wantErr     bool
		errMsg      string
	}{
		{
			name: "single profile",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profiles-")
				if err != nil {
					return "", nil, err
				}

				profileDir := filepath.Join(tmpDir, "test-profile")
				err = os.MkdirAll(profileDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				profilePath := filepath.Join(profileDir, "profile.yaml")
				err = os.WriteFile(profilePath, []byte("mode:\n  withDeps: true"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				cleanup := func() { os.RemoveAll(tmpDir) }
				return tmpDir, cleanup, nil
			},
			wantProfile: "profile.yaml", // Should return the profile path
			wantErr:     false,
		},
		{
			name: "no profiles directory",
			setupFunc: func() (string, func(), error) {
				return "/non/existent/profiles", func() {}, nil
			},
			wantProfile: "",
			wantErr:     false,
		},
		{
			name: "profiles path is a file not directory",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profiles-")
				if err != nil {
					return "", nil, err
				}

				profilesFile := filepath.Join(tmpDir, "profiles")
				err = os.WriteFile(profilesFile, []byte("content"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				cleanup := func() { os.RemoveAll(tmpDir) }
				return profilesFile, cleanup, nil
			},
			wantProfile: "",
			wantErr:     true,
			errMsg:      "is not a directory",
		},
		{
			name: "multiple profiles found",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profiles-")
				if err != nil {
					return "", nil, err
				}

				// Create multiple profile directories
				profile1Dir := filepath.Join(tmpDir, "profile1")
				profile2Dir := filepath.Join(tmpDir, "profile2")

				err = os.Mkdir(profile1Dir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				err = os.Mkdir(profile2Dir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				// Create profile.yaml in both
				err = os.WriteFile(filepath.Join(profile1Dir, "profile.yaml"), []byte("name: profile1"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				err = os.WriteFile(filepath.Join(profile2Dir, "profile.yaml"), []byte("name: profile2"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				cleanup := func() { os.RemoveAll(tmpDir) }
				return tmpDir, cleanup, nil
			},
			wantProfile: "", // Should return empty string when multiple profiles found
			wantErr:     false,
		},
		{
			name: "directory without profile.yaml",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profiles-")
				if err != nil {
					return "", nil, err
				}

				// Create directory but no profile.yaml
				profileDir := filepath.Join(tmpDir, "profile1")
				err = os.Mkdir(profileDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				cleanup := func() { os.RemoveAll(tmpDir) }
				return tmpDir, cleanup, nil
			},
			wantProfile: "",
			wantErr:     false,
		},
		{
			name: "empty profiles directory",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profiles-")
				if err != nil {
					return "", nil, err
				}

				cleanup := func() { os.RemoveAll(tmpDir) }
				return tmpDir, cleanup, nil
			},
			wantProfile: "",
			wantErr:     false,
		},
		{
			name: "profiles directory with files only",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profiles-")
				if err != nil {
					return "", nil, err
				}

				// Create files (not directories)
				err = os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				cleanup := func() { os.RemoveAll(tmpDir) }
				return tmpDir, cleanup, nil
			},
			wantProfile: "",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profilesDir, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Setup failed: %v", err)
			}
			defer cleanup()

			got, err := FindSingleProfile(profilesDir)

			if tt.wantErr {
				if err == nil {
					t.Errorf("FindSingleProfile() expected error but got none")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("FindSingleProfile() error = %v, want error containing %v", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("FindSingleProfile() unexpected error = %v", err)
				return
			}

			if tt.name == "single profile" {
				if got == "" {
					t.Errorf("FindSingleProfile() expected profile path but got empty string")
				} else if !strings.HasSuffix(got, "test-profile/profile.yaml") {
					t.Errorf("FindSingleProfile() = %v, want path ending with test-profile/profile.yaml", got)
				}
			} else if got != tt.wantProfile {
				t.Errorf("FindSingleProfile() = %v, want %v", got, tt.wantProfile)
			}
		})
	}
}

// Additional comprehensive tests from cmd/config/profile_test.go

func TestUnmarshalProfile_Comprehensive(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func() (string, func(), error)
		wantProfile *AnalysisProfile
		wantErr     bool
		errMsg      string
	}{
		{
			name: "valid profile file with name",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, err
				}

				profilePath := filepath.Join(tmpDir, "profile.yaml")
				profileData := `
name: "Test Profile"
mode:
  withDeps: true
scope:
  withKnownLibs: true
  packages:
    included:
      - "com.example"
rules:
  labels:
    included:
      - "test-label"
`

				err = os.WriteFile(profilePath, []byte(profileData), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				return profilePath, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantProfile: &AnalysisProfile{
				Name: "Test Profile",
			},
			wantErr: false,
		},
		{
			name: "non-existent file should fail",
			setupFunc: func() (string, func(), error) {
				return "/non/existent/profile.yaml", func() {}, nil
			},
			wantProfile: nil,
			wantErr:     true,
			errMsg:      "no such file or directory",
		},
		{
			name: "invalid yaml should fail",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, err
				}

				profilePath := filepath.Join(tmpDir, "profile.yaml")
				invalidYaml := "invalid: yaml: content: ["

				err = os.WriteFile(profilePath, []byte(invalidYaml), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				return profilePath, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantProfile: nil,
			wantErr:     true,
			errMsg:      "mapping values are not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Setup failed: %v", err)
			}
			defer cleanup()

			got, err := UnmarshalProfile(path)

			if tt.wantErr {
				if err == nil {
					t.Errorf("UnmarshalProfile() expected error but got none")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("UnmarshalProfile() error = %v, want error containing %v", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("UnmarshalProfile() unexpected error = %v", err)
				return
			}

			if tt.wantProfile != nil && got != nil {
				if got.Name != tt.wantProfile.Name {
					t.Errorf("UnmarshalProfile() Name = %v, want %v", got.Name, tt.wantProfile.Name)
				}
			}
		})
	}
}

func TestSetSettingsFromProfile_Comprehensive(t *testing.T) {
	log := logr.Discard()

	tests := []struct {
		name      string
		setupFunc func() (*mockAnalyzeCommand, *cobra.Command, string, func(), error)
		wantErr   bool
		errMsg    string
		validate  func(*mockAnalyzeCommand, *testing.T)
	}{
		{
			name: "profile without .konveyor should fail",
			setupFunc: func() (*mockAnalyzeCommand, *cobra.Command, string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return nil, nil, "", nil, err
				}

				profilePath := filepath.Join(tmpDir, "profile.yaml")
				profileData := `
mode:
  withDeps: false
`
				err = os.WriteFile(profilePath, []byte(profileData), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return nil, nil, "", nil, err
				}

				cmd := &mockAnalyzeCommand{log: log}
				cobraCmd := &cobra.Command{}
				cobraCmd.Flags().String("input", "", "input flag")
				cobraCmd.Flags().String("mode", "", "mode flag")
				cobraCmd.Flags().Bool("analyze-known-libraries", false, "analyze known libraries flag")
				cobraCmd.Flags().String("incident-selector", "", "incident selector flag")
				cobraCmd.Flags().String("label-selector", "", "label selector flag")
				cobraCmd.Flags().StringSlice("rules", []string{}, "rules flag")

				cleanup := func() { os.RemoveAll(tmpDir) }
				return cmd, cobraCmd, profilePath, cleanup, nil
			},
			wantErr: true,
			errMsg:  "profile path does not contain .konveyor directory",
		},
		{
			name: "profile with source only mode",
			setupFunc: func() (*mockAnalyzeCommand, *cobra.Command, string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return nil, nil, "", nil, err
				}

				konveyorDir := filepath.Join(tmpDir, ".konveyor", "profiles", "test-profile")
				err = os.MkdirAll(konveyorDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return nil, nil, "", nil, err
				}

				profilePath := filepath.Join(konveyorDir, "profile.yaml")
				profileData := `
mode:
  withDeps: false
scope:
  withKnownLibs: false
`
				err = os.WriteFile(profilePath, []byte(profileData), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return nil, nil, "", nil, err
				}

				cmd := &mockAnalyzeCommand{log: log}
				cobraCmd := &cobra.Command{}
				cobraCmd.Flags().String("input", "", "input flag")
				cobraCmd.Flags().String("mode", "", "mode flag")
				cobraCmd.Flags().Bool("analyze-known-libraries", false, "analyze known libraries flag")
				cobraCmd.Flags().String("incident-selector", "", "incident selector flag")
				cobraCmd.Flags().String("label-selector", "", "label selector flag")
				cobraCmd.Flags().StringSlice("rules", []string{}, "rules flag")

				cleanup := func() { os.RemoveAll(tmpDir) }
				return cmd, cobraCmd, profilePath, cleanup, nil
			},
			wantErr: false,
			validate: func(cmd *mockAnalyzeCommand, t *testing.T) {
				if cmd.mode != string(provider.SourceOnlyAnalysisMode) {
					t.Errorf("Expected mode %s, got %s", provider.SourceOnlyAnalysisMode, cmd.mode)
				}
				if cmd.analyzeKnownLibraries {
					t.Errorf("Expected AnalyzeKnownLibraries to be false")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cobraCmd, path, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Setup failed: %v", err)
			}
			defer cleanup()

			settings := &ProfileSettings{
				Input:                 cmd.input,
				Mode:                  cmd.mode,
				AnalyzeKnownLibraries: cmd.analyzeKnownLibraries,
				IncidentSelector:      cmd.incidentSelector,
				LabelSelector:         cmd.labelSelector,
				Rules:                 cmd.rules,
				EnableDefaultRulesets: cmd.enableDefaultRulesets,
			}
			err = SetSettingsFromProfile(path, cobraCmd, settings)

			if tt.wantErr {
				if err == nil {
					t.Errorf("SetSettingsFromProfile() expected error but got none")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("SetSettingsFromProfile() error = %v, want error containing %v", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("SetSettingsFromProfile() unexpected error = %v", err)
				return
			}

			// Apply settings back to mock command for validation
			cmd.input = settings.Input
			cmd.mode = settings.Mode
			cmd.analyzeKnownLibraries = settings.AnalyzeKnownLibraries
			cmd.incidentSelector = settings.IncidentSelector
			cmd.labelSelector = settings.LabelSelector
			cmd.rules = settings.Rules
			cmd.enableDefaultRulesets = settings.EnableDefaultRulesets

			if tt.validate != nil {
				tt.validate(cmd, t)
			}
		})
	}
}

// mockAnalyzeCommand simulates the analyzeCommand struct for testing
type mockAnalyzeCommand struct {
	input                 string
	mode                  string
	analyzeKnownLibraries bool
	incidentSelector      string
	labelSelector         string
	rules                 []string
	enableDefaultRulesets bool
	log                   logr.Logger
}

// Additional comprehensive tests for better coverage

func TestSetSettingsFromProfile_ErrorHandling(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func() (string, *cobra.Command, func(), error)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "invalid profile file should fail",
			setupFunc: func() (string, *cobra.Command, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, nil, err
				}

				konveyorDir := filepath.Join(tmpDir, ".konveyor", "profiles", "test-profile")
				err = os.MkdirAll(konveyorDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, nil, err
				}

				// Create invalid YAML file
				profilePath := filepath.Join(konveyorDir, "profile.yaml")
				invalidYaml := "invalid: yaml: content: ["
				err = os.WriteFile(profilePath, []byte(invalidYaml), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, nil, err
				}

				cmd := &cobra.Command{}
				cmd.Flags().String("input", "", "input flag")
				cmd.Flags().String("mode", "", "mode flag")
				cmd.Flags().Bool("analyze-known-libraries", false, "analyze known libraries flag")
				cmd.Flags().String("incident-selector", "", "incident selector flag")
				cmd.Flags().String("label-selector", "", "label selector flag")
				cmd.Flags().StringSlice("rules", []string{}, "rules flag")

				cleanup := func() { os.RemoveAll(tmpDir) }
				return profilePath, cmd, cleanup, nil
			},
			wantErr: true,
			errMsg:  "mapping values are not allowed",
		},
		{
			name: "GetRulesInProfile with rules as file returns error",
			setupFunc: func() (string, *cobra.Command, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, nil, err
				}

				konveyorDir := filepath.Join(tmpDir, ".konveyor", "profiles", "test-profile")
				err = os.MkdirAll(konveyorDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, nil, err
				}

				profileContent := `
mode:
  withDeps: true
`
				profilePath := filepath.Join(konveyorDir, "profile.yaml")
				err = os.WriteFile(profilePath, []byte(profileContent), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, nil, err
				}

				// Create a file named "rules" instead of directory; GetRulesInProfile returns error
				rulesFile := filepath.Join(konveyorDir, "rules")
				err = os.WriteFile(rulesFile, []byte("content"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, nil, err
				}

				cmd := &cobra.Command{}
				cmd.Flags().String("input", "", "input flag")
				cmd.Flags().String("mode", "", "mode flag")
				cmd.Flags().Bool("analyze-known-libraries", false, "analyze known libraries flag")
				cmd.Flags().String("incident-selector", "", "incident selector flag")
				cmd.Flags().String("label-selector", "", "label selector flag")
				cmd.Flags().StringSlice("rules", []string{}, "rules flag")

				cleanup := func() { os.RemoveAll(tmpDir) }
				return profilePath, cmd, cleanup, nil
			},
			wantErr: true,
			errMsg:  "is not a directory",
		},
		{
			name: "enable-default-rulesets GetBool error when flag has wrong type",
			setupFunc: func() (string, *cobra.Command, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, nil, err
				}
				konveyorDir := filepath.Join(tmpDir, ".konveyor", "profiles", "test-profile")
				if err := os.MkdirAll(konveyorDir, 0755); err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, nil, err
				}
				profilePath := filepath.Join(konveyorDir, "profile.yaml")
				if err := os.WriteFile(profilePath, []byte("mode: {}\nrules:\n  labels:\n    included: []"), 0644); err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, nil, err
				}
				cmd := &cobra.Command{}
				cmd.Flags().String("input", "", "")
				cmd.Flags().String("mode", "", "")
				cmd.Flags().Bool("analyze-known-libraries", false, "")
				cmd.Flags().String("incident-selector", "", "")
				cmd.Flags().String("label-selector", "", "")
				cmd.Flags().StringSlice("rules", []string{}, "")
				// Use String flag so GetBool("enable-default-rulesets") returns an error
				cmd.Flags().String("enable-default-rulesets", "", "")
				_ = cmd.Flags().Set("enable-default-rulesets", "true")
				cmd.Flags().Lookup("enable-default-rulesets").Changed = true
				return profilePath, cmd, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: true,
			errMsg:  "error reading enable-default-rulesets",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, cmd, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Setup failed: %v", err)
			}
			defer cleanup()

			settings := &ProfileSettings{
				EnableDefaultRulesets: true,
			}
			err = SetSettingsFromProfile(path, cmd, settings)

			if tt.wantErr {
				if err == nil {
					t.Errorf("SetSettingsFromProfile() expected error but got none")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("SetSettingsFromProfile() error = %v, want error containing %v", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("SetSettingsFromProfile() unexpected error = %v", err)
				return
			}
		})
	}
}

func TestGetRulesInProfile_ErrorHandling(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func() (string, func(), error)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "stat error on rules directory should be handled",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profile-")
				if err != nil {
					return "", nil, err
				}

				// Create a directory with no permissions to cause stat error
				rulesDir := filepath.Join(tmpDir, "rules")
				err = os.Mkdir(rulesDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				// Remove read permissions from parent directory to cause stat error
				err = os.Chmod(tmpDir, 0000)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				cleanup := func() {
					os.Chmod(tmpDir, 0755) // Restore permissions for cleanup
					os.RemoveAll(tmpDir)
				}
				return tmpDir, cleanup, nil
			},
			wantErr: true,
			errMsg:  "permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profileDir, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Setup failed: %v", err)
			}
			defer cleanup()

			_, err = GetRulesInProfile(profileDir)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetRulesInProfile() expected error but got none")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("GetRulesInProfile() error = %v, want error containing %v", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("GetRulesInProfile() unexpected error = %v", err)
				return
			}
		})
	}
}

func TestFindSingleProfile_ErrorHandling(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func() (string, func(), error)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "stat error on profiles directory should be handled",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profiles-")
				if err != nil {
					return "", nil, err
				}

				profilesDir := filepath.Join(tmpDir, "profiles")
				err = os.Mkdir(profilesDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				// Remove read permissions from parent directory to cause stat error
				err = os.Chmod(tmpDir, 0000)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				cleanup := func() {
					os.Chmod(tmpDir, 0755) // Restore permissions for cleanup
					os.RemoveAll(tmpDir)
				}
				return profilesDir, cleanup, nil
			},
			wantErr: true,
			errMsg:  "permission denied",
		},
		{
			name: "readdir error should be handled",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profiles-")
				if err != nil {
					return "", nil, err
				}

				// Remove read permissions to cause ReadDir error
				err = os.Chmod(tmpDir, 0000)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				cleanup := func() {
					os.Chmod(tmpDir, 0755) // Restore permissions for cleanup
					os.RemoveAll(tmpDir)
				}
				return tmpDir, cleanup, nil
			},
			wantErr: true,
			errMsg:  "permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profilesDir, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Setup failed: %v", err)
			}
			defer cleanup()

			_, err = FindSingleProfile(profilesDir)

			if tt.wantErr {
				if err == nil {
					t.Errorf("FindSingleProfile() expected error but got none")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("FindSingleProfile() error = %v, want error containing %v", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("FindSingleProfile() unexpected error = %v", err)
				return
			}
		})
	}
}
