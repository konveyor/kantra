package provider

import (
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
)

type PythonProvider struct {
	baseProvider
}

func (p *PythonProvider) Name() string {
	return util.PythonProvider
}

func (p *PythonProvider) GetConfig(mode ExecutionMode, opts BaseOptions, extra ...ProviderOption) (provider.Config, error) {
	switch mode {
	case ModeContainer:
		opts.BinaryPath = ContainerGenericProviderBin
	}

	// Python defaults to source-only analysis
	if opts.AnalysisMode == "" {
		opts.AnalysisMode = string(provider.SourceOnlyAnalysisMode)
	}

	cfg := NewBaseConfig(util.PythonProvider, mode, opts)
	psc := cfg.InitConfig[0].ProviderSpecificConfig

	switch mode {
	case ModeContainer:
		psc["lspServerName"] = "pylsp"
		psc[provider.LspServerPathConfigKey] = ContainerPylspPath
		psc["lspServerArgs"] = []string{}
		psc["workspaceFolders"] = []string{}
		psc["dependencyFolders"] = []string{}

	case ModeNetwork:
		psc["lspServerName"] = "pylsp"
		psc[provider.LspServerPathConfigKey] = ContainerPylspPath
	}

	return cfg, nil
}
