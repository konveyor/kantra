package provider

import (
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
)

type NodeJsProvider struct {
	baseProvider
}

func (p *NodeJsProvider) Name() string {
	return util.NodeJSProvider
}

func (p *NodeJsProvider) GetConfig(mode ExecutionMode, opts BaseOptions, extra ...ProviderOption) (provider.Config, error) {
	switch mode {
	case ModeContainer:
		opts.BinaryPath = ContainerGenericProviderBin
	}

	// Node.js defaults to source-only analysis
	if opts.AnalysisMode == "" {
		opts.AnalysisMode = string(provider.SourceOnlyAnalysisMode)
	}

	cfg := NewBaseConfig(util.NodeJSProvider, mode, opts)
	psc := cfg.InitConfig[0].ProviderSpecificConfig

	switch mode {
	case ModeContainer:
		psc["lspServerName"] = "nodejs"
		psc[provider.LspServerPathConfigKey] = ContainerTSLangServerPath
		psc["lspServerArgs"] = []string{"--stdio"}
		psc["workspaceFolders"] = []string{}
		psc["dependencyFolders"] = []string{}

	case ModeNetwork:
		psc["lspServerName"] = "nodejs"
		psc[provider.LspServerPathConfigKey] = ContainerTSLangServerPath
		psc["lspServerArgs"] = []string{"--stdio"}
	}

	return cfg, nil
}
