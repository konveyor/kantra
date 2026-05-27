package provider

import (
	"testing"

	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodeJsProvider_Name(t *testing.T) {
	p := &NodeJsProvider{}
	assert.Equal(t, util.NodeJSProvider, p.Name())
}

func TestNodeJsProvider_GetConfig_ModeContainer(t *testing.T) {
	p := &NodeJsProvider{}
	opts := BaseOptions{
		Location: "/opt/input/source",
	}

	cfg, err := p.GetConfig(ModeContainer, opts)

	require.NoError(t, err)
	assert.Equal(t, util.NodeJSProvider, cfg.Name)
	assert.Equal(t, ContainerNodeJSProviderBin, cfg.BinaryPath)
	assert.Empty(t, cfg.Address)
	require.Len(t, cfg.InitConfig, 1)

	// Node.js defaults to source-only analysis
	assert.Equal(t, provider.SourceOnlyAnalysisMode, cfg.InitConfig[0].AnalysisMode)

	psc := cfg.InitConfig[0].ProviderSpecificConfig
	assert.Equal(t, "nodejs", psc["lspServerName"])
	assert.Equal(t, ContainerTSLangServerPath, psc[provider.LspServerPathConfigKey])
	assert.Equal(t, []interface{}{"--stdio"}, psc["lspServerArgs"])
	assert.Equal(t, []interface{}{}, psc["workspaceFolders"])
	assert.Equal(t, []interface{}{}, psc["dependencyFolders"])
}

func TestNodeJsProvider_GetConfig_ModeNetwork(t *testing.T) {
	p := &NodeJsProvider{}
	opts := BaseOptions{
		Location: "/opt/input/source",
		Address:  "localhost:12348",
	}

	cfg, err := p.GetConfig(ModeNetwork, opts)

	require.NoError(t, err)
	assert.Equal(t, util.NodeJSProvider, cfg.Name)
	assert.Empty(t, cfg.BinaryPath)
	assert.Equal(t, "localhost:12348", cfg.Address)
	require.Len(t, cfg.InitConfig, 1)

	psc := cfg.InitConfig[0].ProviderSpecificConfig
	assert.Equal(t, "nodejs", psc["lspServerName"])
	assert.Equal(t, ContainerTSLangServerPath, psc[provider.LspServerPathConfigKey])
	assert.Equal(t, []interface{}{"--stdio"}, psc["lspServerArgs"])
}

func TestNodeJsProvider_GetConfig_DefaultsToSourceOnly(t *testing.T) {
	p := &NodeJsProvider{}
	opts := BaseOptions{
		Location: "/opt/input/source",
		// Don't set AnalysisMode - should default to source-only
	}

	cfg, err := p.GetConfig(ModeContainer, opts)

	require.NoError(t, err)
	assert.Equal(t, provider.SourceOnlyAnalysisMode, cfg.InitConfig[0].AnalysisMode)
}

func TestNodeJsProvider_GetConfig_OverrideAnalysisMode(t *testing.T) {
	p := &NodeJsProvider{}
	opts := BaseOptions{
		Location:     "/opt/input/source",
		AnalysisMode: "full",
	}

	cfg, err := p.GetConfig(ModeContainer, opts)

	require.NoError(t, err)
	assert.Equal(t, provider.AnalysisMode("full"), cfg.InitConfig[0].AnalysisMode)
}

func TestNodeJsProvider_GetConfig_ModeLocal(t *testing.T) {
	p := &NodeJsProvider{}
	opts := BaseOptions{
		Location:  "/home/user/project",
		KantraDir: "/home/user/.kantra",
	}

	cfg, err := p.GetConfig(ModeLocal, opts)

	require.NoError(t, err)
	assert.Equal(t, util.NodeJSProvider, cfg.Name)
	require.Len(t, cfg.InitConfig, 1)
	assert.Equal(t, "/home/user/project", cfg.InitConfig[0].Location)
	assert.Equal(t, provider.SourceOnlyAnalysisMode, cfg.InitConfig[0].AnalysisMode)
}
