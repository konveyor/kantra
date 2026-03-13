package analyze

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	outputv1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
)

func (a *analyzeCommand) getLabelSelector() string {
	if a.labelSelector != "" {
		return a.labelSelector
	}
	if (a.sources == nil || len(a.sources) == 0) &&
		(a.targets == nil || len(a.targets) == 0) {
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
		} else {
			// when target is specified, but source is not
			// return target expression OR'd with default labels
			return fmt.Sprintf("%s || (%s)",
				targetExpr, strings.Join(defaultLabels, " || "))
		}
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
