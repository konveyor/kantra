package provider

import (
	"strings"

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

	var excludedDirs []interface{}
	if excludedDir := util.GetProfilesExcludedDir(c.InputPath, true); excludedDir != "" {
		excludedDirs = append(excludedDirs, excludedDir)
	}
	excludedPaths := getBuiltinTargetConfigs(c)
	excludedDirs = append(excludedDirs, excludedPaths...)
	if len(excludedDirs) > 0 {
		providerSpecificConfig["excludedDirs"] = excludedDirs
	}

	p.config = provider.Config{
		Name: "builtin",
		InitConfig: []provider.InitConfig{
			{
				Location:               util.SourceMountPath,
				AnalysisMode:           provider.AnalysisMode(c.Mode),
				ProviderSpecificConfig: providerSpecificConfig,
			},
		},
	}

	return p.config, nil
}

func getBuiltinTargetConfigs(c ConfigInput) []interface{} {
	var volumePaths []interface{}

	for _, path := range c.JavaExcludedTargetPaths {
		ns := strings.Replace(path.(string), c.InputPath, util.SourceMountPath, 1)
		volumePaths = append(volumePaths, ns)
	}

	return volumePaths
}
