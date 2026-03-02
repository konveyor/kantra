package analyze

import (
	"github.com/konveyor-ecosystem/kantra/pkg/profile"
	"github.com/spf13/cobra"
)

func (a *analyzeCommand) createProfileSettings() *profile.ProfileSettings {
	return &profile.ProfileSettings{
		Input:                 a.input,
		Mode:                  a.mode,
		AnalyzeKnownLibraries: a.analyzeKnownLibraries,
		IncidentSelector:      a.incidentSelector,
		LabelSelector:         a.labelSelector,
		Rules:                 a.rules,
		EnableDefaultRulesets: a.enableDefaultRulesets,
	}
}

func (a *analyzeCommand) applyProfileSettings(profilePath string, cmd *cobra.Command) error {
	settings := a.createProfileSettings()
	err := profile.SetSettingsFromProfile(profilePath, cmd, settings)
	if err != nil {
		return err
	}
	a.input = settings.Input
	a.mode = settings.Mode
	a.analyzeKnownLibraries = settings.AnalyzeKnownLibraries
	a.incidentSelector = settings.IncidentSelector
	a.labelSelector = settings.LabelSelector
	a.rules = settings.Rules
	a.enableDefaultRulesets = settings.EnableDefaultRulesets

	return nil
}
