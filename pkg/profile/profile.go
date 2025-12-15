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
	if !cmd.Flags().Lookup("incident-selector").Changed {
		settings.IncidentSelector = buildIncidentSelector(profile.Scope.Packages)
	}
	if !cmd.Flags().Lookup("label-selector").Changed {
		settings.LabelSelector = buildLabelSelector(profile.Rules.Labels)
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

func buildIncidentSelector(packages PackageSelector) string {
	incidentParts := []string{}
	if len(packages.Included) > 0 {
		for _, pkg := range packages.Included {
			pkg = strings.TrimSpace(pkg)
			if pkg != "" {
				incidentParts = append(incidentParts, fmt.Sprintf("package=%s", pkg))
			}
		}
	}
	if len(packages.Excluded) > 0 {
		for _, pkg := range packages.Excluded {
			pkg = strings.TrimSpace(pkg)
			if pkg != "" {
				incidentParts = append(incidentParts, fmt.Sprintf("!package=%s", pkg))
			}
		}
	}
	if len(incidentParts) == 0 {
		return ""
	}
	includedParts := []string{}
	excludedParts := []string{}
	for _, part := range incidentParts {
		if strings.HasPrefix(part, "!") {
			excludedParts = append(excludedParts, part)
		} else {
			includedParts = append(includedParts, part)
		}
	}
	selector := ""
	if len(includedParts) > 0 {
		selector = strings.Join(includedParts, " || ")
	}

	if len(excludedParts) > 0 {
		if selector != "" {
			selector = fmt.Sprintf("(%s) && %s", selector, strings.Join(excludedParts, " && "))
		} else {
			selector = strings.Join(excludedParts, " && ")
		}
	}

	return selector
}

func buildLabelSelector(labels LabelSelector) string {
	includedParts := []string{}
	excludedParts := []string{}
	if len(labels.Included) > 0 {
		for _, label := range labels.Included {
			label = strings.TrimSpace(label)
			if label != "" {
				includedParts = append(includedParts, label)
			}
		}
	}
	if len(labels.Excluded) > 0 {
		for _, label := range labels.Excluded {
			label = strings.TrimSpace(label)
			if label != "" {
				excludedParts = append(excludedParts, fmt.Sprintf("!%s", label))
			}
		}
	}
	if len(includedParts) == 0 && len(excludedParts) == 0 {
		return ""
	}
	selector := ""
	if len(includedParts) > 0 {
		selector = fmt.Sprintf("(%s)", strings.Join(includedParts, " || "))
	}
	if len(excludedParts) > 0 {
		if selector != "" {
			selector = fmt.Sprintf("%s && %s", selector, strings.Join(excludedParts, " && "))
		} else {
			selector = strings.Join(excludedParts, " && ")
		}
	}

	return selector
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
