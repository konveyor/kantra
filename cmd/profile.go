package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/tackle2-hub/api"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

const Profiles = ".konveyor/profiles"

func (a *analyzeCommand) unmarshalProfile(path string) (*api.AnalysisProfile, error) {
	if a.profile == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var profile api.AnalysisProfile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return nil, err
	}

	return &profile, nil
}

func (a *analyzeCommand) setSettingsFromProfile(path string, cmd *cobra.Command) error {
	profile, err := a.unmarshalProfile(path)
	if err != nil {
		return err
	}
	konveyorIndex := strings.Index(a.profile, ".konveyor")
	if konveyorIndex == -1 {
		return fmt.Errorf("profile path does not contain .konveyor directory: %s", a.profile)
	}
	// get dir before .konveyor/
	locationDir := a.profile[:konveyorIndex-1]

	if !cmd.Flags().Lookup("input").Changed {
		a.input = locationDir
	}
	if !cmd.Flags().Lookup("mode").Changed {
		if !profile.Mode.WithDeps {
			a.mode = string(provider.SourceOnlyAnalysisMode)
		} else {
			a.mode = string(provider.FullAnalysisMode)
		}
	}
	if !cmd.Flags().Lookup("analyze-known-libraries").Changed && profile.Scope.WithKnownLibs {
		a.analyzeKnownLibraries = true
	}
	if !cmd.Flags().Lookup("incident-selector").Changed && len(profile.Scope.Packages.Included) > 0 {
		a.incidentSelector = strings.Join(profile.Scope.Packages.Included, " || ")
	}
	if !cmd.Flags().Lookup("label-selector").Changed && len(profile.Rules.Labels.Included) > 0 {
		a.labelSelector = strings.Join(profile.Rules.Labels.Included, " || ")
	}

	// add rules from profile directory if not explicitly set via command line
	if !cmd.Flags().Lookup("rules").Changed {
		profileDir := filepath.Dir(path)
		profileRules, err := a.getRulesInProfile(profileDir)
		if err != nil {
			a.log.Error(err, "failed to get rules from profile directory")
			return err
		}
		if len(profileRules) > 0 {
			a.rules = append(a.rules, profileRules...)

			a.enableDefaultRulesets = false
		}
	}

	return nil
}

func (a *analyzeCommand) getRulesInProfile(profile string) ([]string, error) {
	if profile == "" {
		return nil, nil
	}
	rulesDir := filepath.Join(profile, "rules")
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

func (a *analyzeCommand) findSingleProfile(profilesDir string) (string, error) {
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
