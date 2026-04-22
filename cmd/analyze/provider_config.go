package analyze

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/konveyor-ecosystem/kantra/pkg/util"
	outputv1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/spf13/cobra"
)

func labelSelectorSetFromCLI(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	f := cmd.Flags().Lookup("label-selector")
	if f == nil {
		return false
	}
	return f.Changed
}

func (a *analyzeCommand) getLabelSelector(cmd *cobra.Command) string {
	if labelSelectorSetFromCLI(cmd) && a.labelSelector != "" {
		return a.labelSelector
	}
	hasSourceOrTarget := len(a.sources) > 0 || len(a.targets) > 0
	if hasSourceOrTarget {
		cliExpr := a.labelSelectorFromSourcesTargets()
		if a.profilePath != "" && a.labelSelector != "" && !labelSelectorSetFromCLI(cmd) {
			return fmt.Sprintf("(%s) && (%s)", cliExpr, a.labelSelector)
		}
		return cliExpr
	}
	if a.labelSelector != "" {
		return a.labelSelector
	}
	return ""
}

func (a *analyzeCommand) labelSelectorFromSourcesTargets() string {
	if len(a.sources) == 0 && len(a.targets) == 0 {
		return ""
	}
	// default labels are applied everytime either a source or target is specified
	defaultLabels := []string{"discovery"}
	targets := []string{}
	for _, target := range a.targets {
		targets = append(targets,
			fmt.Sprintf("%s=%s", outputv1.TargetTechnologyLabel, target))
	}
	sources := []string{}
	for _, source := range a.sources {
		sources = append(sources,
			fmt.Sprintf("%s=%s", outputv1.SourceTechnologyLabel, source))
	}
	targetExpr := ""
	if len(targets) > 0 {
		targetExpr = fmt.Sprintf("(%s)", strings.Join(targets, " || "))
	}
	sourceExpr := ""
	if len(sources) > 0 {
		sourceExpr = fmt.Sprintf("(%s)", strings.Join(sources, " || "))
	}
	if targetExpr != "" {
		if sourceExpr != "" {
			// when both targets and sources are present, AND them
			return fmt.Sprintf("(%s && %s) || (%s)",
				targetExpr, sourceExpr, strings.Join(defaultLabels, " || "))
		}
		// when target is specified, but source is not
		// return target expression OR'd with default labels
		return fmt.Sprintf("%s || (%s)",
			targetExpr, strings.Join(defaultLabels, " || "))
	}
	if sourceExpr != "" {
		// when only source is specified, OR them all
		return fmt.Sprintf("%s || (%s)",
			sourceExpr, strings.Join(defaultLabels, " || "))
	}
	return ""
}

// loadOverrideProviderSettings loads provider configuration overrides from a JSON file.
// Returns a slice of provider.Config objects that will be merged with default configs.
func (a *analyzeCommand) loadOverrideProviderSettings() ([]provider.Config, error) {
	if a.overrideProviderSettings == "" {
		return nil, nil
	}

	a.log.V(1).Info("loading override provider settings", "file", a.overrideProviderSettings)

	data, err := os.ReadFile(a.overrideProviderSettings)
	if err != nil {
		return nil, fmt.Errorf("failed to read override provider settings file: %w", err)
	}

	var overrideConfigs []provider.Config
	if err := json.Unmarshal(data, &overrideConfigs); err != nil {
		return nil, fmt.Errorf("failed to parse override provider settings JSON: %w", err)
	}

	a.log.V(1).Info("loaded override provider settings", "providers", len(overrideConfigs))
	return overrideConfigs, nil
}

// mergeProviderSpecificConfig merges override config into base config.
// Override values take precedence over base values.
func mergeProviderSpecificConfig(base, override map[string]interface{}) map[string]interface{} {
	if override == nil {
		return base
	}

	result := make(map[string]interface{})
	// Copy base config
	for k, v := range base {
		result[k] = v
	}
	// Apply overrides
	for k, v := range override {
		result[k] = v
	}
	return result
}

// externalProviderNames returns the names of providers in the override configs
// that have an Address set, indicating they are externally managed by the user.
// These providers don't need kantra to start any infrastructure for them.
func externalProviderNames(overrides []provider.Config) []string {
	var names []string
	for _, cfg := range overrides {
		if cfg.Address != "" {
			names = append(names, cfg.Name)
		}
	}
	return names
}

// overrideProviderNameSet returns a set of all provider names in the override configs.
func overrideProviderNameSet(overrides []provider.Config) map[string]bool {
	names := make(map[string]bool, len(overrides))
	for _, cfg := range overrides {
		names[cfg.Name] = true
	}
	return names
}

// standardProviderNames lists the provider names that kantra manages natively.
var standardProviderNames = map[string]bool{
	util.JavaProvider:   true,
	"builtin":           true,
	util.GoProvider:     true,
	util.PythonProvider: true,
	util.NodeJSProvider: true,
	util.CsharpProvider: true,
}

// hasNonStandardExternalProviders returns true if the override configs contain
// at least one provider with an Address set whose name is NOT a standard
// kantra-managed provider. This signals that the user is introducing a
// completely new external provider rather than just tweaking an existing one.
func hasNonStandardExternalProviders(overrides []provider.Config) bool {
	for _, cfg := range overrides {
		if cfg.Address != "" && !standardProviderNames[cfg.Name] {
			return true
		}
	}
	return false
}

// isExternalOnly returns true when all detected providers are externally
// managed (appear in externalNames) or no providers were detected but
// external overrides exist. This determines whether kantra can skip
// Java-specific validation and use only builtin + external providers.
func isExternalOnly(foundProviders []string, externalNames []string) bool {
	if len(externalNames) == 0 {
		return false
	}
	if len(foundProviders) == 0 {
		return true
	}
	externalSet := make(map[string]bool, len(externalNames))
	for _, name := range externalNames {
		externalSet[name] = true
	}
	for _, p := range foundProviders {
		if !externalSet[p] {
			return false
		}
	}
	return true
}

// applyAllProviderOverrides merges override configs into the base provider config list.
// Overrides matching an existing provider name are merged into that config.
// Overrides for providers not in the base list (e.g., user-managed external providers)
// are appended as-is — the user is responsible for starting those providers and
// setting the Address field so the analyzer can connect.
func applyAllProviderOverrides(baseConfigs []provider.Config, overrideConfigs []provider.Config) []provider.Config {
	if overrideConfigs == nil {
		return baseConfigs
	}

	// Merge overrides into matching base configs
	knownProviders := make(map[string]bool, len(baseConfigs))
	for i := range baseConfigs {
		knownProviders[baseConfigs[i].Name] = true
		baseConfigs[i] = applyProviderOverrides(baseConfigs[i], overrideConfigs)
	}

	// Append override configs for providers not already in the base list.
	// These are external providers the user is managing themselves.
	for _, override := range overrideConfigs {
		if !knownProviders[override.Name] {
			baseConfigs = append(baseConfigs, override)
		}
	}

	return baseConfigs
}

// applyProviderOverrides applies override settings to a provider config.
// Returns the modified config with overrides applied.
func applyProviderOverrides(baseConfig provider.Config, overrideConfigs []provider.Config) provider.Config {
	if overrideConfigs == nil {
		return baseConfig
	}

	// Find matching override config by provider name
	for _, override := range overrideConfigs {
		if override.Name != baseConfig.Name {
			continue
		}

		// Apply top-level config overrides
		if override.ContextLines != 0 {
			baseConfig.ContextLines = override.ContextLines
		}
		if override.Proxy != nil {
			baseConfig.Proxy = override.Proxy
		}

		// Merge InitConfig settings
		if len(override.InitConfig) > 0 && len(baseConfig.InitConfig) > 0 {
			// Merge ProviderSpecificConfig from first InitConfig
			if override.InitConfig[0].ProviderSpecificConfig != nil {
				baseConfig.InitConfig[0].ProviderSpecificConfig = mergeProviderSpecificConfig(
					baseConfig.InitConfig[0].ProviderSpecificConfig,
					override.InitConfig[0].ProviderSpecificConfig,
				)
			}
			// Apply other InitConfig fields
			if override.InitConfig[0].AnalysisMode != "" {
				baseConfig.InitConfig[0].AnalysisMode = override.InitConfig[0].AnalysisMode
			}
		}
		break
	}

	return baseConfig
}
