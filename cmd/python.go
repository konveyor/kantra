package cmd

import (
	"fmt"
	"github.com/konveyor/analyzer-lsp/provider"
)

type PythonProvider struct {
	config provider.Config
}

func (p *PythonProvider) GetConfigVolume(a *analyzeCommand, tmpDir string) (provider.Config, error) {
	p.config = provider.Config{
		Name:    pythonProvider,
		Address: fmt.Sprintf("0.0.0.0:%v", a.providersMap[pythonProvider].port),
		InitConfig: []provider.InitConfig{
			{
				AnalysisMode: provider.SourceOnlyAnalysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					"lspServerName":                 "generic",
					"workspaceFolders":              []string{fmt.Sprintf("file://%s", SourceMountPath)},
					provider.LspServerPathConfigKey: "/usr/local/bin/pylsp",
				},
			},
		},
	}
	_, dependencyFolders := a.getDepsFolders()
	if len(dependencyFolders) != 0 {
		p.config.InitConfig[0].ProviderSpecificConfig["dependencyFolders"] = dependencyFolders
	}
	return p.config, nil
}
