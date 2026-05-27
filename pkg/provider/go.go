package provider

import (
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
)

type GoProvider struct {
	baseProvider
}

func (p *GoProvider) Name() string {
	return util.GoProvider
}

func (p *GoProvider) GetConfig(mode ExecutionMode, opts BaseOptions, extra ...ProviderOption) (provider.Config, error) {
	switch mode {
	case ModeContainer:
		opts.BinaryPath = ContainerGoProviderBin
	}

	cfg := NewBaseConfig(util.GoProvider, mode, opts)
	psc := cfg.InitConfig[0].ProviderSpecificConfig

	switch mode {
	case ModeContainer:
		psc["lspServerName"] = "gopls"
		psc[provider.LspServerPathConfigKey] = ContainerGoplsPath
		psc["lspServerArgs"] = []string{}

	case ModeNetwork:
		psc["lspServerName"] = "gopls"
		psc[provider.LspServerPathConfigKey] = ContainerGoplsPath
	}

	return cfg, nil
}
