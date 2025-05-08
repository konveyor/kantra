package provider

import (
	"fmt"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
)

type DotNetProvider struct {
	config provider.Config
}

func (p *DotNetProvider) GetConfigVolume(c ConfigInput) (provider.Config, error) {
	p.config = provider.Config{
		Name:    util.DotnetProvider,
		Address: fmt.Sprintf("0.0.0.0:%v", c.Port),
		InitConfig: []provider.InitConfig{
			{
				Location:     util.SourceMountPath,
				AnalysisMode: provider.SourceOnlyAnalysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					provider.LspServerPathConfigKey: "/opt/app-root/.dotnet/tools/csharp-ls",
				},
			},
		},
	}
	return p.config, nil
}
