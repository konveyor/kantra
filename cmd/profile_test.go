package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestProfile creates a test profile YAML file and returns the path
func createTestProfile(t *testing.T, content string) string {
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "profile.yaml")
	err := os.WriteFile(profilePath, []byte(content), 0644)
	require.NoError(t, err)
	return profilePath
}

// createTestAnalyzeCommand creates a test analyzeCommand instance
func createTestAnalyzeCommand() *analyzeCommand {
	return &analyzeCommand{
		enableDefaultRulesets: true, // default value as set by the flag
		AnalyzeCommandContext: AnalyzeCommandContext{
			log: logr.Discard(),
		},
	}
}

// createTestCobraCommand creates a test cobra command with flags
func createTestCobraCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "test",
	}

	// Add all the flags that validateProfile checks
	cmd.Flags().String("input", "", "input flag")
	cmd.Flags().String("mode", "", "mode flag")
	cmd.Flags().Bool("analyze-known-libraries", false, "analyze-known-libraries flag")
	cmd.Flags().String("label-selector", "", "label-selector flag")
	cmd.Flags().StringSlice("source", []string{}, "source flag")
	cmd.Flags().StringSlice("target", []string{}, "target flag")
	cmd.Flags().Bool("no-dependency-rules", false, "no-dependency-rules flag")
	cmd.Flags().StringSlice("rules", []string{}, "rules flag")
	cmd.Flags().Bool("enable-default-rulesets", false, "enable-default-rulesets flag")
	cmd.Flags().String("incident-selector", "", "incident-selector flag")

	return cmd
}

func TestValidateProfile(t *testing.T) {
	tests := []struct {
		name          string
		setupProfile  func(t *testing.T) string
		setupCommand  func() *cobra.Command
		expectedError string
	}{
		{
			name: "valid profile directory",
			setupProfile: func(t *testing.T) string {
				return t.TempDir()
			},
			setupCommand: func() *cobra.Command {
				return createTestCobraCommand()
			},
			expectedError: "",
		},
		{
			name: "profile path does not exist",
			setupProfile: func(t *testing.T) string {
				return "/nonexistent/path"
			},
			setupCommand: func() *cobra.Command {
				return createTestCobraCommand()
			},
			expectedError: "failed to stat profile at path /nonexistent/path",
		},
		{
			name: "profile is a file not directory",
			setupProfile: func(t *testing.T) string {
				tmpDir := t.TempDir()
				filePath := filepath.Join(tmpDir, "profile.yaml")
				err := os.WriteFile(filePath, []byte("test"), 0644)
				require.NoError(t, err)
				return filePath
			},
			setupCommand: func() *cobra.Command {
				return createTestCobraCommand()
			},
			expectedError: "profile must be a directory",
		},
		{
			name: "input flag is set",
			setupProfile: func(t *testing.T) string {
				return t.TempDir()
			},
			setupCommand: func() *cobra.Command {
				cmd := createTestCobraCommand()
				cmd.Flags().Set("input", "/some/path")
				return cmd
			},
			expectedError: "input must not be set when profile is set",
		},
		{
			name: "mode flag is set",
			setupProfile: func(t *testing.T) string {
				return t.TempDir()
			},
			setupCommand: func() *cobra.Command {
				cmd := createTestCobraCommand()
				cmd.Flags().Set("mode", "source-only")
				return cmd
			},
			expectedError: "mode must not be set when profile is set",
		},
		{
			name: "analyze-known-libraries flag is set",
			setupProfile: func(t *testing.T) string {
				return t.TempDir()
			},
			setupCommand: func() *cobra.Command {
				cmd := createTestCobraCommand()
				cmd.Flags().Set("analyze-known-libraries", "true")
				return cmd
			},
			expectedError: "analyzeKnownLibraries must not be set when profile is set",
		},
		{
			name: "label-selector flag is set",
			setupProfile: func(t *testing.T) string {
				return t.TempDir()
			},
			setupCommand: func() *cobra.Command {
				cmd := createTestCobraCommand()
				cmd.Flags().Set("label-selector", "app=test")
				return cmd
			},
			expectedError: "labelSelector must not be set when profile is set",
		},
		{
			name: "source flag is set",
			setupProfile: func(t *testing.T) string {
				return t.TempDir()
			},
			setupCommand: func() *cobra.Command {
				cmd := createTestCobraCommand()
				cmd.Flags().Set("source", "java")
				return cmd
			},
			expectedError: "sources must not be set when profile is set",
		},
		{
			name: "target flag is set",
			setupProfile: func(t *testing.T) string {
				return t.TempDir()
			},
			setupCommand: func() *cobra.Command {
				cmd := createTestCobraCommand()
				cmd.Flags().Set("target", "cloud-readiness")
				return cmd
			},
			expectedError: "targets must not be set when profile is set",
		},
		{
			name: "no-dependency-rules flag is set",
			setupProfile: func(t *testing.T) string {
				return t.TempDir()
			},
			setupCommand: func() *cobra.Command {
				cmd := createTestCobraCommand()
				cmd.Flags().Set("no-dependency-rules", "true")
				return cmd
			},
			expectedError: "noDepRules must not be set when profile is set",
		},
		{
			name: "rules flag is set",
			setupProfile: func(t *testing.T) string {
				return t.TempDir()
			},
			setupCommand: func() *cobra.Command {
				cmd := createTestCobraCommand()
				cmd.Flags().Set("rules", "rule1")
				return cmd
			},
			expectedError: "rules must not be set when profile is set",
		},
		{
			name: "enable-default-rulesets flag is set",
			setupProfile: func(t *testing.T) string {
				return t.TempDir()
			},
			setupCommand: func() *cobra.Command {
				cmd := createTestCobraCommand()
				cmd.Flags().Set("enable-default-rulesets", "true")
				return cmd
			},
			expectedError: "enableDefaultRulesets must not be set when profile is set",
		},
		{
			name: "incident-selector flag is set",
			setupProfile: func(t *testing.T) string {
				return t.TempDir()
			},
			setupCommand: func() *cobra.Command {
				cmd := createTestCobraCommand()
				cmd.Flags().Set("incident-selector", "package=com.example")
				return cmd
			},
			expectedError: "incidentSelector must not be set when profile is set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := createTestAnalyzeCommand()
			a.profile = tt.setupProfile(t)
			cmd := tt.setupCommand()

			err := a.validateProfile(cmd)

			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestUnmarshalProfile(t *testing.T) {
	tests := []struct {
		name           string
		profileContent string
		profilePath    string
		expectError    bool
		expectedResult *Profile
	}{
		{
			name:           "empty profile path returns nil",
			profilePath:    "",
			expectError:    false,
			expectedResult: nil,
		},
		{
			name: "valid profile YAML",
			profileContent: `apiVersion: v1
kind: Profile
metadata:
  name: test-profile
  id: test-id
  source: test-source
  syncedAt: "2023-01-01T00:00:00Z"
  version: "1.0.0"
spec:
  rules:
    labelSelectors:
      - "app=test"
      - "env=prod"
    rulesets:
      - "ruleset1"
      - "ruleset2"
    useDefaultRules: true
    withDepRules: false
  scope:
    depAanlysis: true
    withKnownLibs: false
    packages: "com.example"
  hubMetadata:
    applicationId: "app-123"
    profileId: "profile-456"
    readonly: true`,
			expectError: false,
			expectedResult: &Profile{
				APIVersion: "v1",
				Kind:       "Profile",
				Metadata: ProfileMetadata{
					Name:     "test-profile",
					ID:       "test-id",
					Source:   "test-source",
					SyncedAt: "2023-01-01T00:00:00Z",
					Version:  "1.0.0",
				},
				Spec: ProfileSpec{
					Rules: ProfileRules{
						LabelSelectors:  []string{"app=test", "env=prod"},
						Rulesets:        []string{"ruleset1", "ruleset2"},
						UseDefaultRules: true,
						WithDepRules:    false,
					},
					Scope: ProfileScope{
						DepAnalysis:   true,
						WithKnownLibs: false,
						Packages:      "com.example",
					},
					HubMetadata: &ProfileHubMetadata{
						ApplicationID: "app-123",
						ProfileID:     "profile-456",
						Readonly:      true,
					},
				},
			},
		},
		{
			name: "minimal valid profile",
			profileContent: `apiVersion: v1
kind: Profile
metadata:
  name: minimal-profile
spec:
  rules:
    useDefaultRules: false
    withDepRules: false
  scope:
    depAanlysis: false
    withKnownLibs: false`,
			expectError: false,
			expectedResult: &Profile{
				APIVersion: "v1",
				Kind:       "Profile",
				Metadata: ProfileMetadata{
					Name: "minimal-profile",
				},
				Spec: ProfileSpec{
					Rules: ProfileRules{
						UseDefaultRules: false,
						WithDepRules:    false,
					},
					Scope: ProfileScope{
						DepAnalysis:   false,
						WithKnownLibs: false,
					},
				},
			},
		},
		{
			name:           "invalid YAML",
			profileContent: "invalid: yaml: content: [",
			expectError:    true,
		},
		{
			name:        "nonexistent file",
			profilePath: "/nonexistent/file.yaml",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := createTestAnalyzeCommand()

			var profilePath string
			if tt.profilePath != "" {
				profilePath = tt.profilePath
				a.profile = "set" // non-empty to trigger processing
			} else if tt.profileContent != "" {
				profilePath = createTestProfile(t, tt.profileContent)
				a.profile = "set" // non-empty to trigger processing
			}

			result, err := a.unmarshalProfile(profilePath)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				if tt.expectedResult == nil {
					assert.Nil(t, result)
				} else {
					assert.Equal(t, tt.expectedResult, result)
				}
			}
		})
	}
}

func TestGetSettingsFromProfile(t *testing.T) {
	tests := []struct {
		name           string
		profileContent string
		profilePath    string
		expectError    bool
		validateFunc   func(t *testing.T, a *analyzeCommand)
	}{
		{
			name: "full profile settings applied correctly",
			profileContent: `apiVersion: v1
kind: Profile
metadata:
  name: full-profile
spec:
  rules:
    labelSelectors:
      - "app=test"
      - "env=prod"
    rulesets:
      - "ruleset1"
      - "ruleset2"
    useDefaultRules: false
    withDepRules: true
  scope:
    depAanlysis: true
    withKnownLibs: true
    packages: "com.example"`,
			validateFunc: func(t *testing.T, a *analyzeCommand) {
				// input should be set to the directory above .konveyor
				expectedInput := filepath.Dir(filepath.Dir(a.profile))
				assert.Equal(t, expectedInput, a.input)

				// mode should be FullAnalysisMode since depAnalysis is true
				assert.Equal(t, string(provider.FullAnalysisMode), a.mode)

				// analyzeKnownLibraries should be true
				assert.True(t, a.analyzeKnownLibraries)

				// labelSelector should be joined with " || "
				assert.Equal(t, "app=test || env=prod", a.labelSelector)

				// rules should be set
				assert.Equal(t, []string{"ruleset1", "ruleset2"}, a.rules)

				// enableDefaultRulesets should be false since useDefaultRules is false
				assert.False(t, a.enableDefaultRulesets)

				// noDepRules should be true since withDepRules is true
				assert.True(t, a.noDepRules)

				// incidentSelector should be set
				assert.Equal(t, "com.example", a.incidentSelector)
			},
		},
		{
			name: "minimal profile with source-only mode",
			profileContent: `apiVersion: v1
kind: Profile
metadata:
  name: minimal-profile
spec:
  rules:
    useDefaultRules: true
    withDepRules: false
  scope:
    depAanlysis: false
    withKnownLibs: false`,
			validateFunc: func(t *testing.T, a *analyzeCommand) {
				// mode should be SourceOnlyAnalysisMode since depAnalysis is false
				assert.Equal(t, string(provider.SourceOnlyAnalysisMode), a.mode)

				// analyzeKnownLibraries should be false (default)
				assert.False(t, a.analyzeKnownLibraries)

				// enableDefaultRulesets should be true since useDefaultRules is true
				assert.True(t, a.enableDefaultRulesets)

				// noDepRules should be false since withDepRules is false
				assert.False(t, a.noDepRules)
			},
		},
		{
			name: "profile with empty arrays and strings",
			profileContent: `apiVersion: v1
kind: Profile
metadata:
  name: empty-arrays-profile
spec:
  rules:
    labelSelectors: []
    rulesets: []
    useDefaultRules: true
    withDepRules: false
  scope:
    depAanlysis: true
    withKnownLibs: false
    packages: ""`,
			validateFunc: func(t *testing.T, a *analyzeCommand) {
				// labelSelector should be empty since labelSelectors is empty
				assert.Equal(t, "", a.labelSelector)

				// rules should be empty since rulesets is empty
				assert.Empty(t, a.rules)

				// incidentSelector should be empty since packages is empty
				assert.Equal(t, "", a.incidentSelector)
			},
		},
		{
			name:        "invalid profile file",
			profilePath: "/nonexistent/file.yaml",
			expectError: true,
			validateFunc: func(t *testing.T, a *analyzeCommand) {
				// No validation needed for error case
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := createTestAnalyzeCommand()

			var profilePath string
			if tt.profilePath != "" {
				profilePath = tt.profilePath
				a.profile = filepath.Dir(profilePath) // Set profile to directory containing the file
			} else {
				// Create a proper directory structure: /tmp/test/.konveyor/profiles/profile.yaml
				tmpDir := t.TempDir()
				konveyorDir := filepath.Join(tmpDir, ".konveyor")
				profilesDir := filepath.Join(konveyorDir, "profiles")
				err := os.MkdirAll(profilesDir, 0755)
				require.NoError(t, err)

				profilePath = filepath.Join(profilesDir, "profile.yaml")
				err = os.WriteFile(profilePath, []byte(tt.profileContent), 0644)
				require.NoError(t, err)

				a.profile = profilesDir
			}

			err := a.getSettingsFromProfile(profilePath)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.validateFunc != nil {
					tt.validateFunc(t, a)
				}
			}
		})
	}
}

func TestProfileStructs(t *testing.T) {
	t.Run("Profile struct fields", func(t *testing.T) {
		profile := Profile{
			APIVersion: "v1",
			Kind:       "Profile",
			Metadata: ProfileMetadata{
				Name:     "test",
				ID:       "id",
				Source:   "source",
				SyncedAt: "time",
				Version:  "1.0",
			},
			Spec: ProfileSpec{
				Rules: ProfileRules{
					LabelSelectors:  []string{"label1"},
					Rulesets:        []string{"ruleset1"},
					UseDefaultRules: true,
					WithDepRules:    false,
				},
				Scope: ProfileScope{
					DepAnalysis:   true,
					WithKnownLibs: false,
					Packages:      "com.example",
				},
				HubMetadata: &ProfileHubMetadata{
					ApplicationID: "app-id",
					ProfileID:     "profile-id",
					Readonly:      true,
				},
			},
		}

		// Test that all fields are accessible
		assert.Equal(t, "v1", profile.APIVersion)
		assert.Equal(t, "Profile", profile.Kind)
		assert.Equal(t, "test", profile.Metadata.Name)
		assert.Equal(t, "id", profile.Metadata.ID)
		assert.Equal(t, "source", profile.Metadata.Source)
		assert.Equal(t, "time", profile.Metadata.SyncedAt)
		assert.Equal(t, "1.0", profile.Metadata.Version)
		assert.Equal(t, []string{"label1"}, profile.Spec.Rules.LabelSelectors)
		assert.Equal(t, []string{"ruleset1"}, profile.Spec.Rules.Rulesets)
		assert.True(t, profile.Spec.Rules.UseDefaultRules)
		assert.False(t, profile.Spec.Rules.WithDepRules)
		assert.True(t, profile.Spec.Scope.DepAnalysis)
		assert.False(t, profile.Spec.Scope.WithKnownLibs)
		assert.Equal(t, "com.example", profile.Spec.Scope.Packages)
		assert.NotNil(t, profile.Spec.HubMetadata)
		assert.Equal(t, "app-id", profile.Spec.HubMetadata.ApplicationID)
		assert.Equal(t, "profile-id", profile.Spec.HubMetadata.ProfileID)
		assert.True(t, profile.Spec.HubMetadata.Readonly)
	})

	t.Run("ProfileHubMetadata can be nil", func(t *testing.T) {
		profile := Profile{
			Spec: ProfileSpec{
				HubMetadata: nil,
			},
		}
		assert.Nil(t, profile.Spec.HubMetadata)
	})
}
