package provider

import (
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
)

type BuiltinProvider struct {
	config provider.Config
}

func (p *BuiltinProvider) GetConfigVolume(c ConfigInput) (provider.Config, error) {
	providerSpecificConfig := map[string]interface{}{
		// Don't set excludedDirs - let analyzer-lsp use default exclusions
		// (node_modules, vendor, dist, build, target, .git, .venv, venv)
		// Java target paths are already included in the defaults (target/)
	}

	if excludedDir := util.GetProfilesExcludedDir(c.InputPath, util.SourceMountPath, true); excludedDir != "" {
		providerSpecificConfig["excludedDirs"] = []interface{}{excludedDir}
	}

	p.config = provider.Config{
		Name: "builtin",
		InitConfig: []provider.InitConfig{
			{
				Location:               c.ContainerSourcePath,
				AnalysisMode:           provider.AnalysisMode(c.Mode),
				ProviderSpecificConfig: providerSpecificConfig,
			},
		},
	}

	return p.config, nil
}
