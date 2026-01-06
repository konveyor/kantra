package provider

import (
	"fmt"

	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
)

type CsharpProvider struct {
	config provider.Config
}

func (p *CsharpProvider) GetConfigVolume(c ConfigInput) (provider.Config, error) {
	p.config = provider.Config{
		Name:    util.CsharpProvider,
		Address: fmt.Sprintf("0.0.0.0:%v", c.Port),
		InitConfig: []provider.InitConfig{
			{
				Location:     util.SourceMountPath,
				AnalysisMode: provider.SourceOnlyAnalysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					"ilspy_cmd": "/usr/local/bin/ilspycmd",
					"paket_cmd": "/usr/local/bin/paket",
				},
			},
		},
	}
	return p.config, nil
}
