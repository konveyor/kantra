package cmd

import (
	"strings"

	"github.com/konveyor/analyzer-lsp/provider"
)

type BuiltinProvider struct {
	config provider.Config
}

func (p *BuiltinProvider) GetConfigVolume(a *analyzeCommand, tmpDir string, excludedTargetPaths []interface{}) (provider.Config, error) {
	p.config = provider.Config{
		Name: "builtin",
		InitConfig: []provider.InitConfig{
			{
				Location:               SourceMountPath,
				AnalysisMode:           provider.AnalysisMode(a.mode),
				ProviderSpecificConfig: map[string]interface{}{},
			},
		},
	}

	excludedPaths := getBuiltinTargetConfigs(a, excludedTargetPaths)
	if len(excludedPaths) > 0 {
		p.config.InitConfig[0].ProviderSpecificConfig["excludedDirs"] = excludedPaths
	}

	return p.config, nil
}

func getBuiltinTargetConfigs(a *analyzeCommand, excludedTargetPaths []interface{}) []interface{} {
	var volumePaths []interface{}

	for _, path := range excludedTargetPaths {
		ns := strings.Replace(path.(string), a.input, SourceMountPath, 1)
		volumePaths = append(volumePaths, ns)
	}

	return volumePaths
}
