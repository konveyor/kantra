package provider

import (
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
)

type BuiltinProvider struct {
	config provider.Config
}

func (p *BuiltinProvider) GetConfigVolume(c ConfigInput) (provider.Config, error) {
	p.config = provider.Config{
		Name: "builtin",
		InitConfig: []provider.InitConfig{
			{
				Location:     util.SourceMountPath,
				AnalysisMode: provider.AnalysisMode(c.Mode),
			},
		},
	}
	return p.config, nil
}
