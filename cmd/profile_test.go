package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/tackle2-hub/api"
	"github.com/spf13/cobra"
)

func TestAnalyzeCommand_unmarshalProfile(t *testing.T) {
	tests := []struct {
		name        string
		profile     string
		setupFunc   func() (string, func(), error)
		wantProfile *api.AnalysisProfile
		wantErr     bool
		errMsg      string
	}{
		{
			name:    "empty profile should return nil",
			profile: "",
			setupFunc: func() (string, func(), error) {
				return "", func() {}, nil
			},
			wantProfile: nil,
			wantErr:     false,
		},
		{
			name:    "valid profile file",
			profile: "test-profile",
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
			wantProfile: &api.AnalysisProfile{
				Name: "Test Profile",
			},
			wantErr: false,
		},
		{
			name:    "non-existent file should fail",
			profile: "test-profile",
			setupFunc: func() (string, func(), error) {
				return "/non/existent/profile.yaml", func() {}, nil
			},
			wantProfile: nil,
			wantErr:     true,
			errMsg:      "no such file or directory",
		},
		{
			name:    "invalid yaml should fail",
			profile: "test-profile",
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

			cmd := &analyzeCommand{
				profile: tt.profile,
			}

			got, err := cmd.unmarshalProfile(path)

			if tt.wantErr {
				if err == nil {
					t.Errorf("unmarshalProfile() expected error but got none")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("unmarshalProfile() error = %v, want error containing %v", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("unmarshalProfile() unexpected error = %v", err)
				return
			}

			if tt.wantProfile == nil && got != nil {
				t.Errorf("unmarshalProfile() = %v, want nil", got)
				return
			}

			if tt.wantProfile != nil && got == nil {
				t.Errorf("unmarshalProfile() = nil, want %v", tt.wantProfile)
				return
			}

			if tt.wantProfile != nil && got != nil {
				if got.Name != tt.wantProfile.Name {
					t.Errorf("unmarshalProfile() Name = %v, want %v", got.Name, tt.wantProfile.Name)
				}
				if got.Mode.WithDeps != true {
					t.Errorf("unmarshalProfile() Mode.WithDeps = %v, want true", got.Mode.WithDeps)
				}
				if got.Scope.WithKnownLibs != true {
					t.Errorf("unmarshalProfile() Scope.WithKnownLibs = %v, want true", got.Scope.WithKnownLibs)
				}
				if len(got.Scope.Packages.Included) == 0 || got.Scope.Packages.Included[0] != "com.example" {
					t.Errorf("unmarshalProfile() Scope.Packages.Included = %v, want [com.example]", got.Scope.Packages.Included)
				}
				if len(got.Rules.Labels.Included) == 0 || got.Rules.Labels.Included[0] != "test-label" {
					t.Errorf("unmarshalProfile() Rules.Labels.Included = %v, want [test-label]", got.Rules.Labels.Included)
				}
			}
		})
	}
}

func TestAnalyzeCommand_setSettingsFromProfile(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func() (*analyzeCommand, *cobra.Command, string, func(), error)
		wantErr   bool
		errMsg    string
		validate  func(*analyzeCommand, *testing.T)
	}{
		{
			name: "profile with all settings",
			setupFunc: func() (*analyzeCommand, *cobra.Command, string, func(), error) {
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

				rulesDir := filepath.Join(konveyorDir, "rules", "test-rule")
				err = os.MkdirAll(rulesDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return nil, nil, "", nil, err
				}

				profilePath := filepath.Join(konveyorDir, "profile.yaml")
				profileData := `
name: "Test Profile"
mode:
  withDeps: true
scope:
  withKnownLibs: true
  packages:
    included:
      - "com.example"
      - "org.test"
rules:
  labels:
    included:
      - "test-label"
      - "another-label"
`

				err = os.WriteFile(profilePath, []byte(profileData), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return nil, nil, "", nil, err
				}

				cmd := &analyzeCommand{
					profile: profilePath,
				}
				cmd.log = logr.Discard()

				cobraCmd := &cobra.Command{}
				cobraCmd.Flags().String("input", "", "")
				cobraCmd.Flags().String("mode", "", "")
				cobraCmd.Flags().Bool("analyze-known-libraries", false, "")
				cobraCmd.Flags().String("incident-selector", "", "")
				cobraCmd.Flags().String("label-selector", "", "")
				cobraCmd.Flags().StringSlice("rules", []string{}, "")

				return cmd, cobraCmd, profilePath, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: false,
			validate: func(cmd *analyzeCommand, t *testing.T) {
				expectedInput := strings.Split(cmd.profile, ".konveyor")[0]
				expectedInput = strings.TrimSuffix(expectedInput, "/")
				if cmd.input != expectedInput {
					t.Errorf("Expected input to be %s, got %s", expectedInput, cmd.input)
				}

				if cmd.mode != string(provider.FullAnalysisMode) {
					t.Errorf("Expected mode to be %s, got %s", provider.FullAnalysisMode, cmd.mode)
				}

				if !cmd.analyzeKnownLibraries {
					t.Errorf("Expected analyzeKnownLibraries to be true")
				}

				expectedIncidentSelector := "com.example || org.test"
				if cmd.incidentSelector != expectedIncidentSelector {
					t.Errorf("Expected incidentSelector to be %s, got %s", expectedIncidentSelector, cmd.incidentSelector)
				}

				expectedLabelSelector := "test-label || another-label"
				if cmd.labelSelector != expectedLabelSelector {
					t.Errorf("Expected labelSelector to be %s, got %s", expectedLabelSelector, cmd.labelSelector)
				}

				if len(cmd.rules) == 0 {
					t.Errorf("Expected rules to be added from profile")
				}
				if cmd.enableDefaultRulesets {
					t.Errorf("Expected enableDefaultRulesets to be false when profile rules are found")
				}
			},
		},
		{
			name: "profile with source-only mode",
			setupFunc: func() (*analyzeCommand, *cobra.Command, string, func(), error) {
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
name: "Source Only Profile"
mode:
  withDeps: false
`

				err = os.WriteFile(profilePath, []byte(profileData), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return nil, nil, "", nil, err
				}

				cmd := &analyzeCommand{
					profile: profilePath,
				}
				cmd.log = logr.Discard()

				cobraCmd := &cobra.Command{}
				cobraCmd.Flags().String("input", "", "")
				cobraCmd.Flags().String("mode", "", "")
				cobraCmd.Flags().Bool("analyze-known-libraries", false, "")
				cobraCmd.Flags().String("incident-selector", "", "")
				cobraCmd.Flags().String("label-selector", "", "")
				cobraCmd.Flags().StringSlice("rules", []string{}, "")

				return cmd, cobraCmd, profilePath, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: false,
			validate: func(cmd *analyzeCommand, t *testing.T) {
				if cmd.mode != string(provider.SourceOnlyAnalysisMode) {
					t.Errorf("Expected mode to be %s, got %s", provider.SourceOnlyAnalysisMode, cmd.mode)
				}
			},
		},
		{
			name: "invalid profile path without .konveyor",
			setupFunc: func() (*analyzeCommand, *cobra.Command, string, func(), error) {
				cmd := &analyzeCommand{
					profile: "/invalid/path/profile.yaml",
				}

				cobraCmd := &cobra.Command{}
				cobraCmd.Flags().String("input", "", "")
				cobraCmd.Flags().String("mode", "", "")
				cobraCmd.Flags().Bool("analyze-known-libraries", false, "")
				cobraCmd.Flags().String("incident-selector", "", "")
				cobraCmd.Flags().String("label-selector", "", "")
				cobraCmd.Flags().StringSlice("rules", []string{}, "")

				return cmd, cobraCmd, "/invalid/path/profile.yaml", func() {}, nil
			},
			wantErr: true,
			errMsg:  "no such file or directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cobraCmd, path, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Setup failed: %v", err)
			}
			defer cleanup()

			err = cmd.setSettingsFromProfile(path, cobraCmd)

			if tt.wantErr {
				if err == nil {
					t.Errorf("setSettingsFromProfile() expected error but got none")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("setSettingsFromProfile() error = %v, want error containing %v", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("setSettingsFromProfile() unexpected error = %v", err)
				return
			}

			if tt.validate != nil {
				tt.validate(cmd, t)
			}
		})
	}
}

func TestAnalyzeCommand_getRulesInProfile(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func() (string, func(), error)
		wantRules []string
		wantErr   bool
		errMsg    string
	}{
		{
			name: "empty profile should return nil",
			setupFunc: func() (string, func(), error) {
				return "", func() {}, nil
			},
			wantRules: nil,
			wantErr:   false,
		},
		{
			name: "profile with multiple rule directories",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-rules-")
				if err != nil {
					return "", nil, err
				}

				profileDir := filepath.Join(tmpDir, "profile")
				rulesDir := filepath.Join(profileDir, "rules")

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

				err = os.WriteFile(filepath.Join(rulesDir, "not-a-rule.txt"), []byte("test"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				return profileDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantRules: []string{},
			wantErr:   false,
		},
		{
			name: "profile with no rules directory",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-rules-")
				if err != nil {
					return "", nil, err
				}

				profileDir := filepath.Join(tmpDir, "profile")
				err = os.MkdirAll(profileDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				return profileDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantRules: nil,
			wantErr:   false,
		},
		{
			name: "rules path is a file not directory",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-rules-")
				if err != nil {
					return "", nil, err
				}

				profileDir := filepath.Join(tmpDir, "profile")
				err = os.MkdirAll(profileDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				rulesFile := filepath.Join(profileDir, "rules")
				err = os.WriteFile(rulesFile, []byte("test"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				return profileDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantRules: nil,
			wantErr:   true,
			errMsg:    "is not a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profileDir, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Setup failed: %v", err)
			}
			defer cleanup()

			cmd := &analyzeCommand{}
			got, err := cmd.getRulesInProfile(profileDir)

			if tt.wantErr {
				if err == nil {
					t.Errorf("getRulesInProfile() expected error but got none")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("getRulesInProfile() error = %v, want error containing %v", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("getRulesInProfile() unexpected error = %v", err)
				return
			}

			if tt.name == "profile with multiple rule directories" {
				if len(got) != 2 {
					t.Errorf("getRulesInProfile() returned %d rules, expected 2", len(got))
					return
				}

				expectedRules := []string{
					filepath.Join(profileDir, "rules", "rule1"),
					filepath.Join(profileDir, "rules", "rule2"),
				}

				for _, expectedRule := range expectedRules {
					found := false
					for _, gotRule := range got {
						if gotRule == expectedRule {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("getRulesInProfile() missing expected rule %s", expectedRule)
					}
				}
				return
			}

			if len(got) != len(tt.wantRules) {
				t.Errorf("getRulesInProfile() returned %d rules, expected %d", len(got), len(tt.wantRules))
			}
		})
	}
}

func TestAnalyzeCommand_findSingleProfile(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func() (string, func(), error)
		wantProfile string
		wantErr     bool
		errMsg      string
	}{
		{
			name: "single valid profile",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profiles-")
				if err != nil {
					return "", nil, err
				}

				profilesDir := filepath.Join(tmpDir, "profiles")
				profileDir := filepath.Join(profilesDir, "test-profile")
				err = os.MkdirAll(profileDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				profilePath := filepath.Join(profileDir, "profile.yaml")
				err = os.WriteFile(profilePath, []byte("name: test"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				return profilesDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantProfile: "",
			wantErr:     false,
		},
		{
			name: "multiple profiles should return empty",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profiles-")
				if err != nil {
					return "", nil, err
				}

				profilesDir := filepath.Join(tmpDir, "profiles")

				profile1Dir := filepath.Join(profilesDir, "profile1")
				err = os.MkdirAll(profile1Dir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				profile1Path := filepath.Join(profile1Dir, "profile.yaml")
				err = os.WriteFile(profile1Path, []byte("name: profile1"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				profile2Dir := filepath.Join(profilesDir, "profile2")
				err = os.MkdirAll(profile2Dir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				profile2Path := filepath.Join(profile2Dir, "profile.yaml")
				err = os.WriteFile(profile2Path, []byte("name: profile2"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				return profilesDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantProfile: "",
			wantErr:     false,
		},
		{
			name: "no profiles should return empty",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profiles-")
				if err != nil {
					return "", nil, err
				}

				profilesDir := filepath.Join(tmpDir, "profiles")
				err = os.MkdirAll(profilesDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				return profilesDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantProfile: "",
			wantErr:     false,
		},
		{
			name: "directory with profile dir but no profile.yaml",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-profiles-")
				if err != nil {
					return "", nil, err
				}

				profilesDir := filepath.Join(tmpDir, "profiles")
				profileDir := filepath.Join(profilesDir, "test-profile")
				err = os.MkdirAll(profileDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				// Don't create profile.yaml

				return profilesDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantProfile: "",
			wantErr:     false,
		},
		{
			name: "non-existent directory",
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
				err = os.WriteFile(profilesFile, []byte("test"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				return profilesFile, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantProfile: "",
			wantErr:     true,
			errMsg:      "is not a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profilesDir, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Setup failed: %v", err)
			}
			defer cleanup()

			cmd := &analyzeCommand{}
			got, err := cmd.findSingleProfile(profilesDir)

			if tt.wantErr {
				if err == nil {
					t.Errorf("findSingleProfile() expected error but got none")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("findSingleProfile() error = %v, want error containing %v", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("findSingleProfile() unexpected error = %v", err)
				return
			}

			if tt.name == "single valid profile" {
				expectedPath := filepath.Join(profilesDir, "test-profile", "profile.yaml")
				if got != expectedPath {
					t.Errorf("findSingleProfile() = %v, want %v", got, expectedPath)
				}
				return
			}

			if got != tt.wantProfile {
				t.Errorf("findSingleProfile() = %v, want %v", got, tt.wantProfile)
			}
		})
	}
}
