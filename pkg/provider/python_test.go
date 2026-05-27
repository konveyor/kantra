package provider

import (
	"testing"

	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPythonProvider_Name(t *testing.T) {
	p := &PythonProvider{}
	assert.Equal(t, util.PythonProvider, p.Name())
}

func TestPythonProvider_GetConfig_ModeContainer(t *testing.T) {
	p := &PythonProvider{}
	opts := BaseOptions{
		Location: "/opt/input/source",
	}

	cfg, err := p.GetConfig(ModeContainer, opts)

	require.NoError(t, err)
	assert.Equal(t, util.PythonProvider, cfg.Name)
	assert.Equal(t, ContainerPythonProviderBin, cfg.BinaryPath)
	assert.Empty(t, cfg.Address)
	require.Len(t, cfg.InitConfig, 1)
	assert.Equal(t, "/opt/input/source", cfg.InitConfig[0].Location)

	// Python defaults to source-only analysis
	assert.Equal(t, provider.SourceOnlyAnalysisMode, cfg.InitConfig[0].AnalysisMode)

	psc := cfg.InitConfig[0].ProviderSpecificConfig
	assert.Equal(t, "pylsp", psc["lspServerName"])
	assert.Equal(t, ContainerPylspPath, psc[provider.LspServerPathConfigKey])
	assert.Contains(t, psc, "lspServerArgs")
	assert.Contains(t, psc, "workspaceFolders")
	assert.Contains(t, psc, "dependencyFolders")
}

func TestPythonProvider_GetConfig_ModeNetwork(t *testing.T) {
	p := &PythonProvider{}
	opts := BaseOptions{
		Location: "/opt/input/source",
		Address:  "localhost:12347",
	}

	cfg, err := p.GetConfig(ModeNetwork, opts)

	require.NoError(t, err)
	assert.Equal(t, util.PythonProvider, cfg.Name)
	assert.Empty(t, cfg.BinaryPath)
	assert.Equal(t, "localhost:12347", cfg.Address)
	require.Len(t, cfg.InitConfig, 1)
	assert.Equal(t, "/opt/input/source", cfg.InitConfig[0].Location)

	psc := cfg.InitConfig[0].ProviderSpecificConfig
	assert.Equal(t, "pylsp", psc["lspServerName"])
	assert.Equal(t, ContainerPylspPath, psc[provider.LspServerPathConfigKey])
}

func TestPythonProvider_GetConfig_DefaultsToSourceOnly(t *testing.T) {
	p := &PythonProvider{}
	opts := BaseOptions{
		Location: "/opt/input/source",
		// Don't set AnalysisMode - should default to source-only
	}

	cfg, err := p.GetConfig(ModeContainer, opts)

	require.NoError(t, err)
	assert.Equal(t, provider.SourceOnlyAnalysisMode, cfg.InitConfig[0].AnalysisMode)
}

func TestPythonProvider_GetConfig_OverrideAnalysisMode(t *testing.T) {
	p := &PythonProvider{}
	opts := BaseOptions{
		Location:     "/opt/input/source",
		AnalysisMode: "full",
	}

	cfg, err := p.GetConfig(ModeContainer, opts)

	require.NoError(t, err)
	// Even though we try to set full, the provider should respect the override
	assert.Equal(t, provider.AnalysisMode("full"), cfg.InitConfig[0].AnalysisMode)
}

func TestPythonProvider_GetConfig_ModeLocal(t *testing.T) {
	p := &PythonProvider{}
	opts := BaseOptions{
		Location:  "/home/user/project",
		KantraDir: "/home/user/.kantra",
	}

	cfg, err := p.GetConfig(ModeLocal, opts)

	require.NoError(t, err)
	assert.Equal(t, util.PythonProvider, cfg.Name)
	require.Len(t, cfg.InitConfig, 1)
	assert.Equal(t, "/home/user/project", cfg.InitConfig[0].Location)
	assert.Equal(t, provider.SourceOnlyAnalysisMode, cfg.InitConfig[0].AnalysisMode)
}
