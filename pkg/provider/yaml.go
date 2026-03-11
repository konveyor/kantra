package provider

import (
	"github.com/konveyor/analyzer-lsp/provider"
)

// YamlProvider generates configuration for the YAML/yq analyzer provider.
// Only supported in container mode.
type YamlProvider struct {
	baseProvider
}

func (p *YamlProvider) Name() string {
	return "yaml"
}

func (p *YamlProvider) GetConfig(mode ExecutionMode, opts BaseOptions, extra ...ProviderOption) (provider.Config, error) {
	switch mode {
	case ModeContainer:
		opts.BinaryPath = ContainerYqProviderBin
	}

	cfg := NewBaseConfig("yaml", mode, opts)
	psc := cfg.InitConfig[0].ProviderSpecificConfig

	switch mode {
	case ModeContainer:
		psc["name"] = "yq"
		psc[provider.LspServerPathConfigKey] = ContainerYqPath
	}

	return cfg, nil
}
