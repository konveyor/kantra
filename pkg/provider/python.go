package provider

import (
	"fmt"
	"path/filepath"

	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
)

type PythonProvider struct {
	config provider.Config
}

func (p *PythonProvider) GetConfigVolume(c ConfigInput) (provider.Config, error) {
	providerSpecificConfig := map[string]interface{}{
		"lspServerName":                 "pylsp",
		"workspaceFolders":              []interface{}{fmt.Sprintf("file://%s", util.SourceMountPath)},
		provider.LspServerPathConfigKey: "/usr/local/bin/pylsp",
		"dependencyProviderPath":        "",
	}

	if excludedDir := util.GetProfilesExcludedDir(c.InputPath, true); excludedDir != "" {
		providerSpecificConfig["excludedDirs"] = []interface{}{excludedDir}
	}
	depFolders := []string{filepath.Join(util.SourceMountPath, "_pycache_")}
	if len(c.DepsFolders) != 0 {
		if len(c.DepsFolders) != 0 {
			depFolders = append(depFolders, c.DepsFolders...)
		}
	}
	providerSpecificConfig["dependencyFolders"] = depFolders

	p.config = provider.Config{
		Name:    util.PythonProvider,
		Address: fmt.Sprintf("0.0.0.0:%v", c.Port),
		InitConfig: []provider.InitConfig{
			{
				AnalysisMode:           provider.SourceOnlyAnalysisMode,
				ProviderSpecificConfig: providerSpecificConfig,
			},
		},
	}
	return p.config, nil
}
