package provider

import (
	"fmt"

	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
)

type NodeJsProvider struct {
	config provider.Config
}

func (p *NodeJsProvider) GetConfigVolume(c ConfigInput) (provider.Config, error) {
	providerSpecificConfig := map[string]interface{}{
		"lspServerName":                 "nodejs",
		"workspaceFolders":              []interface{}{fmt.Sprintf("file://%s", util.SourceMountPath)},
		provider.LspServerPathConfigKey: "/usr/local/bin/typescript-language-server",
		"lspServerArgs":                 []interface{}{"--stdio"},
	}

	if excludedDir := util.GetProfilesExcludedDir(c.InputPath, true); excludedDir != "" {
		providerSpecificConfig["excludedDirs"] = []interface{}{excludedDir}
	}

	p.config = provider.Config{
		Name:    util.NodeJSProvider,
		Address: fmt.Sprintf("0.0.0.0:%v", c.Port),
		InitConfig: []provider.InitConfig{
			{
				AnalysisMode:           provider.SourceOnlyAnalysisMode,
				ProviderSpecificConfig: providerSpecificConfig,
			},
		},
	}
	if len(c.DepsFolders) != 0 {
		p.config.InitConfig[0].ProviderSpecificConfig["dependencyFolders"] = c.DepsFolders
	}
	return p.config, nil
}
