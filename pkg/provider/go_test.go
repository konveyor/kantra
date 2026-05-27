package provider

import (
	"testing"

	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoProvider_Name(t *testing.T) {
	p := &GoProvider{}
	assert.Equal(t, util.GoProvider, p.Name())
}

func TestGoProvider_GetConfig_ModeContainer(t *testing.T) {
	p := &GoProvider{}
	opts := BaseOptions{
		Location: "/opt/input/source",
	}

	cfg, err := p.GetConfig(ModeContainer, opts)

	require.NoError(t, err)
	assert.Equal(t, util.GoProvider, cfg.Name)
	assert.Equal(t, ContainerGoProviderBin, cfg.BinaryPath)
	assert.Empty(t, cfg.Address)
	require.Len(t, cfg.InitConfig, 1)
	assert.Equal(t, "/opt/input/source", cfg.InitConfig[0].Location)

	psc := cfg.InitConfig[0].ProviderSpecificConfig
	assert.Equal(t, "gopls", psc["lspServerName"])
	assert.Equal(t, ContainerGoplsPath, psc[provider.LspServerPathConfigKey])
	assert.Contains(t, psc, "lspServerArgs")
}

func TestGoProvider_GetConfig_ModeNetwork(t *testing.T) {
	p := &GoProvider{}
	opts := BaseOptions{
		Location: "/opt/input/source",
		Address:  "localhost:12346",
	}

	cfg, err := p.GetConfig(ModeNetwork, opts)

	require.NoError(t, err)
	assert.Equal(t, util.GoProvider, cfg.Name)
	assert.Empty(t, cfg.BinaryPath)
	assert.Equal(t, "localhost:12346", cfg.Address)
	require.Len(t, cfg.InitConfig, 1)
	assert.Equal(t, "/opt/input/source", cfg.InitConfig[0].Location)

	psc := cfg.InitConfig[0].ProviderSpecificConfig
	assert.Equal(t, "gopls", psc["lspServerName"])
	assert.Equal(t, ContainerGoplsPath, psc[provider.LspServerPathConfigKey])
}

func TestGoProvider_GetConfig_ModeLocal(t *testing.T) {
	p := &GoProvider{}
	opts := BaseOptions{
		Location:  "/home/user/project",
		KantraDir: "/home/user/.kantra",
	}

	cfg, err := p.GetConfig(ModeLocal, opts)

	require.NoError(t, err)
	assert.Equal(t, util.GoProvider, cfg.Name)
	// ModeLocal doesn't set BinaryPath in the switch statement, but NewBaseConfig might
	require.Len(t, cfg.InitConfig, 1)
	assert.Equal(t, "/home/user/project", cfg.InitConfig[0].Location)
}

func TestGoProvider_GetConfig_AnalysisMode(t *testing.T) {
	p := &GoProvider{}
	opts := BaseOptions{
		Location:     "/opt/input/source",
		AnalysisMode: "source-only",
	}

	cfg, err := p.GetConfig(ModeContainer, opts)

	require.NoError(t, err)
	assert.Equal(t, provider.AnalysisMode("source-only"), cfg.InitConfig[0].AnalysisMode)
}
