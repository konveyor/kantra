package cmd

import (
	"github.com/konveyor/analyzer-lsp/provider"
)

type BuiltinProvider struct {
	config provider.Config
}

func (p *BuiltinProvider) GetConfigVolume(a *analyzeCommand, tmpDir string) (provider.Config, error) {
	p.config = provider.Config{
		Name: "builtin",
		InitConfig: []provider.InitConfig{
			{
				Location:     SourceMountPath,
				AnalysisMode: provider.AnalysisMode(a.mode),
			},
		},
	}
	return p.config, nil
}
