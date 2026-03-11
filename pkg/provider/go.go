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
		opts.BinaryPath = ContainerGenericProviderBin
	}

	cfg := NewBaseConfig(util.GoProvider, mode, opts)
	psc := cfg.InitConfig[0].ProviderSpecificConfig

	switch mode {
	case ModeContainer:
		psc["lspServerName"] = "generic"
		psc[provider.LspServerPathConfigKey] = ContainerGoplsPath
		psc["lspServerArgs"] = []string{}
		psc["dependencyProviderPath"] = ContainerGolangDepPath

	case ModeNetwork:
		psc["lspServerName"] = "generic"
		psc[provider.LspServerPathConfigKey] = ContainerGoplsPath
		psc["dependencyProviderPath"] = ContainerGolangDepPath
	}

	return cfg, nil
}
