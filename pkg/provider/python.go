package provider

import (
	"fmt"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
)

type PythonProvider struct {
	config provider.Config
}

func (p *PythonProvider) GetConfigVolume(c ConfigInput) (provider.Config, error) {
	p.config = provider.Config{
		Name:    util.PythonProvider,
		Address: fmt.Sprintf("0.0.0.0:%v", c.Port),
		InitConfig: []provider.InitConfig{
			{
				AnalysisMode: provider.SourceOnlyAnalysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					"lspServerName":                 "generic",
					"workspaceFolders":              []string{fmt.Sprintf("file://%s", util.SourceMountPath)},
					provider.LspServerPathConfigKey: "/usr/local/bin/pylsp",
				},
			},
		},
	}
	if len(c.DepsFolders) != 0 {
		p.config.InitConfig[0].ProviderSpecificConfig["dependencyFolders"] = c.DepsFolders
	}
	return p.config, nil
}
