package cmd

import (
	"fmt"

	"github.com/konveyor/analyzer-lsp/provider"
)

type NodeJsProvider struct {
	config provider.Config
}

func (p *NodeJsProvider) GetConfigVolume(a *analyzeCommand, tmpDir string) (provider.Config, error) {
	p.config = provider.Config{
		Name:    nodeJSProvider,
		Address: fmt.Sprintf("0.0.0.0:%v", a.providersMap[nodeJSProvider].port),
		InitConfig: []provider.InitConfig{
			{
				AnalysisMode: provider.SourceOnlyAnalysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					"lspServerName":                 "nodejs",
					"workspaceFolders":              []string{fmt.Sprintf("file://%s", SourceMountPath)},
					provider.LspServerPathConfigKey: "/usr/local/bin/typescript-language-server",
					"lspServerArgs":                 []string{"--stdio"},
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
