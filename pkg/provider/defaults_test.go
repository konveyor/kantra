package provider

import (
	"testing"

	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultProviderConfig_ModeContainer_AllProviders(t *testing.T) {
	configs := DefaultProviderConfig(ModeContainer, DefaultOptions{})

	require.Len(t, configs, len(AllContainerProviders))

	byName := configsByName(configs)

	// Java provider
	java := byName[util.JavaProvider]
	assert.Equal(t, ContainerJavaProviderBin, java.BinaryPath)
	assert.Empty(t, java.Address)
	require.Len(t, java.InitConfig, 1)
	assert.Equal(t, provider.FullAnalysisMode, java.InitConfig[0].AnalysisMode)
	assert.Equal(t, ContainerJDTLSPath, java.InitConfig[0].ProviderSpecificConfig[provider.LspServerPathConfigKey])
	assert.Equal(t, ContainerJavaBundlePath, java.InitConfig[0].ProviderSpecificConfig["bundles"])
	assert.Equal(t, ContainerDepOpenSourceLabels, java.InitConfig[0].ProviderSpecificConfig["depOpenSourceLabelsFile"])

	// Builtin provider
	builtin := byName["builtin"]
	assert.Empty(t, builtin.BinaryPath)
	assert.Empty(t, builtin.Address)
	require.Len(t, builtin.InitConfig, 1)

	// Go provider
	goP := byName[util.GoProvider]
	assert.Equal(t, ContainerGenericProviderBin, goP.BinaryPath)
	require.Len(t, goP.InitConfig, 1)
	assert.Equal(t, "generic", goP.InitConfig[0].ProviderSpecificConfig["lspServerName"])
	assert.Equal(t, ContainerGoplsPath, goP.InitConfig[0].ProviderSpecificConfig[provider.LspServerPathConfigKey])
	assert.Equal(t, ContainerGolangDepPath, goP.InitConfig[0].ProviderSpecificConfig["dependencyProviderPath"])

	// Python provider
	python := byName[util.PythonProvider]
	assert.Equal(t, ContainerGenericProviderBin, python.BinaryPath)
	require.Len(t, python.InitConfig, 1)
	assert.Equal(t, "pylsp", python.InitConfig[0].ProviderSpecificConfig["lspServerName"])
	assert.Equal(t, ContainerPylspPath, python.InitConfig[0].ProviderSpecificConfig[provider.LspServerPathConfigKey])

	// NodeJS provider
	nodejs := byName[util.NodeJSProvider]
	assert.Equal(t, ContainerGenericProviderBin, nodejs.BinaryPath)
	require.Len(t, nodejs.InitConfig, 1)
	assert.Equal(t, "nodejs", nodejs.InitConfig[0].ProviderSpecificConfig["lspServerName"])
	assert.Equal(t, ContainerTSLangServerPath, nodejs.InitConfig[0].ProviderSpecificConfig[provider.LspServerPathConfigKey])

	// YAML provider
	yaml := byName["yaml"]
	assert.Equal(t, ContainerYqProviderBin, yaml.BinaryPath)
	require.Len(t, yaml.InitConfig, 1)
	assert.Equal(t, ContainerYqPath, yaml.InitConfig[0].ProviderSpecificConfig[provider.LspServerPathConfigKey])
}

func TestDefaultProviderConfig_ModeContainer_MatchesRunnerDefaults(t *testing.T) {
	// This test validates that ModeContainer produces configs equivalent
	// to the former defaultProviderConfig in pkg/testing/runner.go.
	configs := DefaultProviderConfig(ModeContainer, DefaultOptions{})
	byName := configsByName(configs)

	// Verify all 6 providers from the runner's defaultProviderConfig
	expectedProviders := []string{"java", "builtin", "go", "python", "nodejs", "yaml"}
	for _, name := range expectedProviders {
		_, ok := byName[name]
		assert.True(t, ok, "missing provider: %s", name)
	}

	// Verify Java-specific values match runner.go defaults
	java := byName["java"]
	assert.Equal(t, "/usr/local/bin/java-external-provider", java.BinaryPath)
	psc := java.InitConfig[0].ProviderSpecificConfig
	assert.Equal(t, "java", psc["lspServerName"])
	assert.Equal(t,
		"/jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar",
		psc["bundles"])
	assert.Equal(t, "/usr/local/etc/maven.default.index", psc["depOpenSourceLabelsFile"])
	assert.Equal(t, "/jdtls/bin/jdtls", psc[provider.LspServerPathConfigKey])
}

func TestDefaultProviderConfig_ModeLocal(t *testing.T) {
	kantraDir := "/home/user/.kantra"
	input := "/home/user/project"

	configs := DefaultProviderConfig(ModeLocal, DefaultOptions{
		KantraDir:          kantraDir,
		Location:           input,
		AnalysisMode:       "source-only",
		DisableMavenSearch: true,
	})

	require.Len(t, configs, len(AllLocalProviders))

	byName := configsByName(configs)

	// Java provider - local paths
	java := byName[util.JavaProvider]
	assert.Equal(t, kantraDir+"/java-external-provider", java.BinaryPath)
	assert.Empty(t, java.Address)
	require.Len(t, java.InitConfig, 1)
	assert.Equal(t, input, java.InitConfig[0].Location)
	assert.Equal(t, provider.AnalysisMode("source-only"), java.InitConfig[0].AnalysisMode)

	psc := java.InitConfig[0].ProviderSpecificConfig
	assert.Equal(t, kantraDir+ContainerJavaBundlePath, psc["bundles"])
	assert.Equal(t, kantraDir+ContainerJDTLSPath, psc[provider.LspServerPathConfigKey])
	assert.Equal(t, kantraDir+"/maven.default.index", psc["depOpenSourceLabelsFile"])
	assert.Equal(t, kantraDir, psc["mavenIndexPath"])
	assert.Equal(t, true, psc["cleanExplodedBin"])
	assert.Equal(t, kantraDir+"/fernflower.jar", psc["fernFlowerPath"])
	assert.Equal(t, kantraDir+"/task.gradle", psc["gradleSourcesTaskFile"])
	assert.Equal(t, true, psc["disableMavenSearch"])

	// Builtin provider - local paths
	builtin := byName["builtin"]
	assert.Empty(t, builtin.BinaryPath)
	require.Len(t, builtin.InitConfig, 1)
	assert.Equal(t, input, builtin.InitConfig[0].Location)
}

func TestDefaultProviderConfig_ModeNetwork(t *testing.T) {
	sourcePath := "/opt/input/source"
	addresses := map[string]string{
		util.JavaProvider:   "localhost:12345",
		util.GoProvider:     "localhost:12346",
		util.PythonProvider: "localhost:12347",
		util.NodeJSProvider: "localhost:12348",
		util.CsharpProvider: "localhost:12349",
	}

	configs := DefaultProviderConfig(ModeNetwork, DefaultOptions{
		Location:          sourcePath,
		ProviderAddresses: addresses,
	})

	require.Len(t, configs, len(AllNetworkProviders))

	byName := configsByName(configs)

	// Java provider - network mode
	java := byName[util.JavaProvider]
	assert.Empty(t, java.BinaryPath)
	assert.Equal(t, "localhost:12345", java.Address)
	require.Len(t, java.InitConfig, 1)
	assert.Equal(t, sourcePath, java.InitConfig[0].Location)
	// Network mode uses container-internal paths for ProviderSpecificConfig
	assert.Equal(t, ContainerJDTLSPath, java.InitConfig[0].ProviderSpecificConfig[provider.LspServerPathConfigKey])

	// Go provider - network mode
	goP := byName[util.GoProvider]
	assert.Equal(t, "localhost:12346", goP.Address)
	assert.Empty(t, goP.BinaryPath)

	// Builtin - always in-process (no address)
	builtin := byName["builtin"]
	assert.Empty(t, builtin.Address)
	assert.Empty(t, builtin.BinaryPath)

	// C# provider - network mode
	csharp := byName[util.CsharpProvider]
	assert.Equal(t, "localhost:12349", csharp.Address)
	assert.Equal(t, provider.SourceOnlyAnalysisMode, csharp.InitConfig[0].AnalysisMode)
}

func TestDefaultProviderConfig_ProviderFilter(t *testing.T) {
	// Only request java provider
	configs := DefaultProviderConfig(ModeContainer, DefaultOptions{
		Providers: []string{util.JavaProvider},
	})

	require.Len(t, configs, 1)
	assert.Equal(t, util.JavaProvider, configs[0].Name)
}

func TestDefaultProviderConfig_Proxy(t *testing.T) {
	configs := DefaultProviderConfig(ModeContainer, DefaultOptions{
		Providers:  []string{util.JavaProvider},
		HTTPProxy:  "http://proxy:8080",
		HTTPSProxy: "https://proxy:8443",
		NoProxy:    "localhost",
	})

	require.Len(t, configs, 1)
	require.NotNil(t, configs[0].Proxy)
	assert.Equal(t, "http://proxy:8080", configs[0].Proxy.HTTPProxy)
	assert.Equal(t, "https://proxy:8443", configs[0].Proxy.HTTPSProxy)
	assert.Equal(t, "localhost", configs[0].Proxy.NoProxy)
}

func TestDefaultProviderConfig_NoProxyWhenEmpty(t *testing.T) {
	configs := DefaultProviderConfig(ModeContainer, DefaultOptions{
		Providers: []string{util.JavaProvider},
	})

	require.Len(t, configs, 1)
	assert.Nil(t, configs[0].Proxy)
}

func TestDefaultProviderConfig_ContextLines(t *testing.T) {
	configs := DefaultProviderConfig(ModeContainer, DefaultOptions{
		Providers:    []string{util.JavaProvider, "builtin"},
		ContextLines: 5,
	})

	require.Len(t, configs, 2)
	for _, cfg := range configs {
		assert.Equal(t, 5, cfg.ContextLines, "provider %s", cfg.Name)
	}
}

func TestDefaultProviderConfig_JavaOverrides(t *testing.T) {
	configs := DefaultProviderConfig(ModeContainer, DefaultOptions{
		Providers:         []string{util.JavaProvider},
		MavenSettingsFile: "/path/to/settings.xml",
		JvmMaxMem:         "4096m",
	})

	require.Len(t, configs, 1)
	psc := configs[0].InitConfig[0].ProviderSpecificConfig
	assert.Equal(t, "/path/to/settings.xml", psc["mavenSettingsFile"])
	assert.Equal(t, "4096m", psc["jvmMaxMem"])
}

func TestDefaultProviderConfig_UnknownProviderSkipped(t *testing.T) {
	configs := DefaultProviderConfig(ModeContainer, DefaultOptions{
		Providers: []string{"unknown-provider"},
	})

	assert.Empty(t, configs)
}

// configsByName creates a lookup map from provider configs.
func configsByName(configs []provider.Config) map[string]provider.Config {
	m := make(map[string]provider.Config, len(configs))
	for _, c := range configs {
		m[c.Name] = c
	}
	return m
}
