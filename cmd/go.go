package cmd

import (
	"fmt"
	"github.com/konveyor/analyzer-lsp/provider"
)

type GoProvider struct {
	config provider.Config
}

func (p *GoProvider) GetConfigVolume(a *analyzeCommand, tmpDir string) (provider.Config, error) {
	p.config = provider.Config{
		Name:    goProvider,
		Address: fmt.Sprintf("0.0.0.0:%v", a.providersMap[goProvider].port),
		InitConfig: []provider.InitConfig{
			{
				AnalysisMode: provider.FullAnalysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					"lspServerName":                 "generic",
					"workspaceFolders":              []string{fmt.Sprintf("file://%s", SourceMountPath)},
					"dependencyProviderPath":        "/usr/local/bin/golang-dependency-provider",
					provider.LspServerPathConfigKey: "/root/go/bin/gopls",
				},
			},
		},
	}
	return p.config, nil
}
