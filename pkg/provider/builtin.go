package provider

import (
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
	"strings"
)

type BuiltinProvider struct {
	config provider.Config
}

func (p *BuiltinProvider) GetConfigVolume(c ConfigInput) (provider.Config, error) {

	p.config = provider.Config{
		Name: "builtin",
		InitConfig: []provider.InitConfig{
			{
				Location:               util.SourceMountPath,
				AnalysisMode:           provider.AnalysisMode(c.Mode),
				ProviderSpecificConfig: map[string]interface{}{},
			},
		},
	}

	excludedPaths := getBuiltinTargetConfigs(c)
	if len(excludedPaths) > 0 {
		p.config.InitConfig[0].ProviderSpecificConfig["excludedDirs"] = excludedPaths
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
