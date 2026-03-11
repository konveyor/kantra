package provider

import (
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
)

type CsharpProvider struct {
	baseProvider
}

func (p *CsharpProvider) Name() string {
	return util.CsharpProvider
}

func (p *CsharpProvider) SupportsLogLevel() bool {
	return false
}

func (p *CsharpProvider) GetConfig(mode ExecutionMode, opts BaseOptions, extra ...ProviderOption) (provider.Config, error) {
	// C# always uses source-only analysis
	opts.AnalysisMode = string(provider.SourceOnlyAnalysisMode)

	cfg := NewBaseConfig(util.CsharpProvider, mode, opts)
	psc := cfg.InitConfig[0].ProviderSpecificConfig

	switch mode {
	case ModeContainer, ModeNetwork:
		psc["ilspy_cmd"] = ContainerIlspyCmdPath
		psc["paket_cmd"] = ContainerPaketCmdPath
	}

	return cfg, nil
}
