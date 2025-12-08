package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

const Profiles = ".konveyor/profiles"

type AnalysisProfile struct {
	ID    uint          `json:"id" yaml:"id"`
	Name  string        `json:"name" yaml:"name"`
	Mode  AnalysisMode  `json:"mode,omitempty" yaml:"mode,omitempty"`
	Scope AnalysisScope `json:"scope,omitempty" yaml:"scope,omitempty"`
	Rules AnalysisRules `json:"rules,omitempty" yaml:"rules,omitempty"`
}

type AnalysisMode struct {
	WithDeps bool `json:"withDeps" yaml:"withDeps"`
}

type PackageSelector struct {
	Included []string `json:"included,omitempty" yaml:"included,omitempty"`
	Excluded []string `json:"excluded,omitempty" yaml:"excluded,omitempty"`
}

type AnalysisScope struct {
	WithKnownLibs bool            `json:"withKnownLibs" yaml:"withKnownLibs"`
	Packages      PackageSelector `json:"packages,omitempty" yaml:"packages,omitempty"`
}

type LabelSelector struct {
	Included []string `json:"included,omitempty" yaml:"included,omitempty"`
	Excluded []string `json:"excluded,omitempty" yaml:"excluded,omitempty"`
}

type AnalysisRules struct {
	Labels LabelSelector `json:"labels,omitempty" yaml:"labels,omitempty"`
}

type ProfileSettings struct {
	Input                 string
	Mode                  string
	AnalyzeKnownLibraries bool
	IncidentSelector      string
	LabelSelector         string
	Rules                 []string
	EnableDefaultRulesets bool
}

func UnmarshalProfile(path string) (*AnalysisProfile, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var profile AnalysisProfile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return nil, err
	}

	return &profile, nil
}

func SetSettingsFromProfile(path string, cmd *cobra.Command, settings *ProfileSettings) error {
	profile, err := UnmarshalProfile(path)
	if err != nil {
		return err
	}
	konveyorIndex := strings.Index(path, ".konveyor")
	if konveyorIndex == -1 {
		return fmt.Errorf("profile path does not contain .konveyor directory: %s", path)
	}
	// get dir before .konveyor/
	locationDir := path[:konveyorIndex-1]

	if !cmd.Flags().Lookup("input").Changed {
		settings.Input = locationDir
	}
	if !cmd.Flags().Lookup("mode").Changed {
		if !profile.Mode.WithDeps {
			settings.Mode = string(provider.SourceOnlyAnalysisMode)
		} else {
			settings.Mode = string(provider.FullAnalysisMode)
		}
	}
	if !cmd.Flags().Lookup("analyze-known-libraries").Changed && profile.Scope.WithKnownLibs {
		settings.AnalyzeKnownLibraries = true
	}
	if !cmd.Flags().Lookup("incident-selector").Changed && len(profile.Scope.Packages.Included) > 0 {
		settings.IncidentSelector = strings.Join(profile.Scope.Packages.Included, " || ")
	}
	if !cmd.Flags().Lookup("label-selector").Changed && len(profile.Rules.Labels.Included) > 0 {
		settings.LabelSelector = strings.Join(profile.Rules.Labels.Included, " || ")
	}

	// add rules from profile directory if not explicitly set via command line
	if !cmd.Flags().Lookup("rules").Changed {
		profileDir := filepath.Dir(path)
		profileRules, err := GetRulesInProfile(profileDir)
		if err != nil {
			return err
		}
		if len(profileRules) > 0 {
			settings.Rules = append(settings.Rules, profileRules...)
			settings.EnableDefaultRulesets = false
		}
	}

	return nil
}

func GetRulesInProfile(profileDir string) ([]string, error) {
	if profileDir == "" {
		return nil, nil
	}
	rulesDir := filepath.Join(profileDir, "rules")
	stat, err := os.Stat(rulesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	if !stat.IsDir() {
		return nil, fmt.Errorf("rules path %s is not a directory", rulesDir)
	}
	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		return nil, err
	}

	var rulePaths []string
	for _, entry := range entries {
		if entry.IsDir() {
			rulePath := filepath.Join(rulesDir, entry.Name())
			rulePaths = append(rulePaths, rulePath)
		}
	}

	return rulePaths, nil
}

func FindSingleProfile(profilesDir string) (string, error) {
	// check for a profile dir to use as default
	stat, err := os.Stat(profilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	if !stat.IsDir() {
		return "", fmt.Errorf("found profiles path %s is not a directory", profilesDir)
	}
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		return "", err
	}

	var profileDirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			profilePath := filepath.Join(profilesDir, entry.Name(), "profile.yaml")
			if _, err := os.Stat(profilePath); err == nil {
				profileDirs = append(profileDirs, entry.Name())
			}
		}
	}
	// do not error
	if len(profileDirs) == 0 {
		return "", nil
	}
	// do not error - only use as default if 1 found
	if len(profileDirs) > 1 {
		return "", nil
	}

	profilePath := filepath.Join(profilesDir, profileDirs[0], "profile.yaml")
	return profilePath, nil
}
