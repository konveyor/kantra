package cmd

import (
	"fmt"
	"github.com/konveyor/analyzer-lsp/provider"
)

type DotNetProvider struct {
	config provider.Config
}

func (p *DotNetProvider) GetConfigVolume(a *analyzeCommand, tmpDir string) (provider.Config, error) {
	p.config = provider.Config{
		Name:    dotnetProvider,
		Address: fmt.Sprintf("0.0.0.0:%v", a.providersMap[dotnetProvider].port),
		InitConfig: []provider.InitConfig{
			{
				Location:     SourceMountPath,
				AnalysisMode: provider.SourceOnlyAnalysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					provider.LspServerPathConfigKey: "/opt/app-root/.dotnet/tools/csharp-ls",
				},
			},
		},
	}
	return p.config, nil
}
