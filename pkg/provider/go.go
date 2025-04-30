package provider

import (
	"fmt"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
)

type GoProvider struct {
	config provider.Config
}

func (p *GoProvider) GetConfigVolume(c ConfigInput) (provider.Config, error) {
	p.config = provider.Config{
		Name:    util.GoProvider,
		Address: fmt.Sprintf("0.0.0.0:%v", c.Port),
		InitConfig: []provider.InitConfig{
			{
				AnalysisMode: provider.FullAnalysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					"lspServerName":                 "generic",
					"workspaceFolders":              []string{fmt.Sprintf("file://%s", util.SourceMountPath)},
					"dependencyProviderPath":        "/usr/local/bin/golang-dependency-provider",
					provider.LspServerPathConfigKey: "/usr/local/bin/gopls",
				},
			},
		},
	}
	return p.config, nil
}
