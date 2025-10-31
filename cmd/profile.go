package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

type Profile struct {
	APIVersion string          `yaml:"apiVersion" json:"apiVersion"`
	Kind       string          `yaml:"kind" json:"kind"`
	Metadata   ProfileMetadata `yaml:"metadata" json:"metadata"`
	Spec       ProfileSpec     `yaml:"spec" json:"spec"`
}

type ProfileMetadata struct {
	Name     string `yaml:"name" json:"name"`
	ID       string `yaml:"id,omitempty" json:"id,omitempty"`
	Source   string `yaml:"source,omitempty" json:"source,omitempty"`
	SyncedAt string `yaml:"syncedAt,omitempty" json:"syncedAt,omitempty"`
	Version  string `yaml:"version,omitempty" json:"version,omitempty"`
}

type ProfileSpec struct {
	Rules       ProfileRules        `yaml:"rules" json:"rules"`
	Scope       ProfileScope        `yaml:"scope" json:"scope"`
	HubMetadata *ProfileHubMetadata `yaml:"hubMetadata,omitempty" json:"hubMetadata,omitempty"`
}

type ProfileRules struct {
	LabelSelectors  []string `yaml:"labelSelectors,omitempty" json:"labelSelectors,omitempty"`
	Rulesets        []string `yaml:"rulesets,omitempty" json:"rulesets,omitempty"`
	UseDefaultRules bool     `yaml:"useDefaultRules" json:"useDefaultRules"`
	WithDepRules    bool     `yaml:"withDepRules" json:"withDepRules"`
}

type ProfileScope struct {
	DepAnalysis   bool   `yaml:"depAanlysis" json:"depAanlysis"`
	WithKnownLibs bool   `yaml:"withKnownLibs" json:"withKnownLibs"`
	Packages      string `yaml:"packages,omitempty" json:"packages,omitempty"`
}

type ProfileHubMetadata struct {
	ApplicationID string `yaml:"applicationId" json:"applicationId"`
	ProfileID     string `yaml:"profileId" json:"profileId"`
	Readonly      bool   `yaml:"readonly" json:"readonly"`
}

func (a *analyzeCommand) validateProfile(cmd *cobra.Command) error {
	stat, err := os.Stat(a.profile)
	if err != nil {
		return fmt.Errorf("%w failed to stat profile at path %s", err, a.profile)
	}
	if !stat.IsDir() {
		return fmt.Errorf("profile must be a directory")
	}
	if cmd.Flags().Lookup("input").Changed {
		return fmt.Errorf("input must not be set when profile is set")
	}
	if cmd.Flags().Lookup("mode").Changed {
		return fmt.Errorf("mode must not be set when profile is set")
	}
	if cmd.Flags().Lookup("analyze-known-libraries").Changed {
		return fmt.Errorf("analyzeKnownLibraries must not be set when profile is set")
	}
	if cmd.Flags().Lookup("label-selector").Changed {
		return fmt.Errorf("labelSelector must not be set when profile is set")
	}
	if cmd.Flags().Lookup("source").Changed {
		return fmt.Errorf("sources must not be set when profile is set")
	}
	if cmd.Flags().Lookup("target").Changed {
		return fmt.Errorf("targets must not be set when profile is set")
	}
	if cmd.Flags().Lookup("no-dependency-rules").Changed {
		return fmt.Errorf("noDepRules must not be set when profile is set")
	}
	if cmd.Flags().Lookup("rules").Changed {
		return fmt.Errorf("rules must not be set when profile is set")
	}
	if cmd.Flags().Lookup("enable-default-rulesets").Changed {
		return fmt.Errorf("enableDefaultRulesets must not be set when profile is set")
	}
	if cmd.Flags().Lookup("incident-selector").Changed {
		return fmt.Errorf("incidentSelector must not be set when profile is set")
	}

	return nil
}

func (a *analyzeCommand) unmarshalProfile() (*Profile, error) {
	if a.profile == "" {
		return nil, nil
	}
	profilePath := filepath.Join(a.profile, "profile.yaml")
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return nil, fmt.Errorf("%w failed to read profile file %s", err, profilePath)
	}

	var profile Profile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("%w failed to unmarshal profile file %s", err, profilePath)
	}

	return &profile, nil
}

func (a *analyzeCommand) getSettingsFromProfile(ctx context.Context, profile *Profile) error {
	var err error
	profile, err = a.unmarshalProfile()
	if err != nil {
		a.log.Error(err, "failed to unmarshal profile file")
		return err
	}
	// location is the dir before .konveyor
	konveyorProfileDir := filepath.Dir(a.profile)
	locationDir := filepath.Dir(konveyorProfileDir)

	a.input = locationDir
	if !profile.Spec.Scope.DepAnalysis {
		a.mode = string(provider.SourceOnlyAnalysisMode)
	} else {
		a.mode = string(provider.FullAnalysisMode)
	}
	// default is false
	if profile.Spec.Scope.WithKnownLibs {
		a.analyzeKnownLibraries = true
	}
	if profile.Spec.Rules.LabelSelectors != nil {
		a.labelSelector = strings.Join(profile.Spec.Rules.LabelSelectors, " || ")
	}
	if profile.Spec.Rules.Rulesets != nil {
		a.rules = profile.Spec.Rules.Rulesets
	}
	if !profile.Spec.Rules.UseDefaultRules {
		a.enableDefaultRulesets = false
	}
	if profile.Spec.Rules.WithDepRules {
		a.noDepRules = true
	}
	if profile.Spec.Scope.Packages != "" {
		a.incidentSelector = profile.Spec.Scope.Packages
	}
	return nil
}
