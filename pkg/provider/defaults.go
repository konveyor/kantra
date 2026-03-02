package provider

import (
	"path/filepath"

	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
)

// ExecutionMode determines how providers are located and connected.
type ExecutionMode int

const (
	// ModeContainer uses in-container paths. Providers run as in-process
	// binaries started via BinaryPath. Used by the test runner for
	// in-container analysis.
	ModeContainer ExecutionMode = iota

	// ModeLocal uses local filesystem paths relative to KantraDir.
	// Providers run as in-process binaries on the host.
	// Used by containerless mode.
	ModeLocal

	// ModeNetwork connects to providers via network addresses. Providers
	// run in separate containers and are reached via Address (e.g.,
	// "localhost:PORT"). Used by hybrid mode.
	ModeNetwork
)

// Container-internal paths -- single source of truth.
// These are the canonical locations of provider binaries and resources
// inside the kantra/provider container images.
const (
	// Provider binaries
	ContainerJavaProviderBin    = "/usr/local/bin/java-external-provider"
	ContainerGenericProviderBin = "/usr/local/bin/generic-external-provider"
	ContainerYqProviderBin      = "/usr/local/bin/yq-external-provider"

	// Java provider resources
	ContainerJDTLSPath           = "/jdtls/bin/jdtls"
	ContainerJavaBundlePath      = "/jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar"
	ContainerMavenIndexPath      = "/usr/local/etc/maven-index.txt"
	ContainerDepOpenSourceLabels = "/usr/local/etc/maven.default.index"

	// Go provider resources
	ContainerGoplsPath     = "/usr/local/bin/gopls"
	ContainerGolangDepPath = "/usr/local/bin/golang-dependency-provider"

	// Python provider resources
	ContainerPylspPath = "/usr/local/bin/pylsp"

	// Node.js provider resources
	ContainerTSLangServerPath = "/usr/local/bin/typescript-language-server"

	// YAML provider resources
	ContainerYqPath = "/usr/bin/yq"

	// C# provider resources
	ContainerIlspyCmdPath = "/usr/local/bin/ilspycmd"
	ContainerPaketCmdPath = "/usr/local/bin/paket"
)

// AllContainerProviders lists all providers supported in container mode.
var AllContainerProviders = []string{
	util.JavaProvider, "builtin", util.GoProvider,
	util.PythonProvider, util.NodeJSProvider, "yaml",
}

// AllLocalProviders lists all providers supported in local/containerless mode.
var AllLocalProviders = []string{util.JavaProvider, "builtin"}

// AllNetworkProviders lists all providers supported in network/hybrid mode.
var AllNetworkProviders = []string{
	util.JavaProvider, "builtin", util.GoProvider,
	util.PythonProvider, util.NodeJSProvider, util.CsharpProvider,
}

// DefaultOptions configures provider config generation.
type DefaultOptions struct {
	// Providers filters which providers to include.
	// If empty, all providers supported by the mode are included.
	Providers []string

	// Location is the source code path. Interpretation depends on mode:
	//   ModeLocal: local filesystem path (e.g., /home/user/project)
	//   ModeContainer: container-internal path, or empty (set later)
	//   ModeNetwork: container-internal path where source is mounted
	Location string

	LocalLocation string

	// AnalysisMode (e.g., "full", "source-only"). Empty uses provider defaults.
	AnalysisMode string

	// -- ModeLocal fields --

	// KantraDir is the local kantra installation directory containing
	// provider binaries and resources. Required for ModeLocal.
	KantraDir string

	// DisableMavenSearch disables Maven central search for the Java provider.
	DisableMavenSearch bool

	// -- ModeNetwork fields --

	// ProviderAddresses maps provider names to their network addresses
	// (e.g., "localhost:12345"). Required for ModeNetwork.
	ProviderAddresses map[string]string

	// -- Shared optional fields --

	// ContextLines sets the number of context lines for code snippets.
	ContextLines int

	// Proxy settings applied to all provider configs.
	HTTPProxy, HTTPSProxy, NoProxy string

	// MavenSettingsFile path for the Java provider.
	MavenSettingsFile string

	// JvmMaxMem sets JVM max memory for the Java provider.
	JvmMaxMem string
}

// DefaultProviderConfig returns provider configs for the given execution mode.
// This is the single source of truth for provider configuration across
// the analyze, test, and hybrid commands.
//
// The returned configs can be:
//   - Serialized to JSON for konveyor.NewAnalyzer (via WithProviderConfigFilePath)
//   - Used directly by the test runner
//   - Customized by the caller (e.g., adding excludedDirs, merging overrides)
func DefaultProviderConfig(mode ExecutionMode, opts DefaultOptions) []provider.Config {
	providers := opts.Providers
	if len(providers) == 0 {
		switch mode {
		case ModeContainer:
			providers = AllContainerProviders
		case ModeLocal:
			providers = AllLocalProviders
		case ModeNetwork:
			providers = AllNetworkProviders
		}
	}

	configs := make([]provider.Config, 0, len(providers))
	for _, name := range providers {
		var cfg provider.Config
		switch mode {
		case ModeContainer:
			cfg = containerConfig(name, opts)
		case ModeLocal:
			cfg = localConfig(name, opts)
		case ModeNetwork:
			cfg = networkConfig(name, opts)
		}
		if cfg.Name == "" {
			continue
		}
		if opts.ContextLines > 0 {
			cfg.ContextLines = opts.ContextLines
		}
		applyProxy(&cfg, opts)
		configs = append(configs, cfg)
	}
	return configs
}

// applyProxy sets proxy configuration on a provider config if any proxy is specified.
func applyProxy(cfg *provider.Config, opts DefaultOptions) {
	if opts.HTTPProxy != "" || opts.HTTPSProxy != "" {
		cfg.Proxy = &provider.Proxy{
			HTTPProxy:  opts.HTTPProxy,
			HTTPSProxy: opts.HTTPSProxy,
			NoProxy:    opts.NoProxy,
		}
	}
}

// containerConfig returns a provider config for running inside a container.
// Providers are started as local binaries within the container.
func containerConfig(name string, opts DefaultOptions) provider.Config {
	analysisMode := resolveAnalysisMode(opts.AnalysisMode, provider.FullAnalysisMode)

	switch name {
	case util.JavaProvider:
		cfg := provider.Config{
			Name:       util.JavaProvider,
			BinaryPath: ContainerJavaProviderBin,
			InitConfig: []provider.InitConfig{{
				Location:     opts.Location,
				AnalysisMode: analysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					"lspServerName":                 util.JavaProvider,
					"bundles":                       ContainerJavaBundlePath,
					"depOpenSourceLabelsFile":       ContainerDepOpenSourceLabels,
					provider.LspServerPathConfigKey: ContainerJDTLSPath,
				},
			}},
		}
		applyJavaOverrides(&cfg, opts)
		return cfg

	case "builtin":
		return provider.Config{
			Name: "builtin",
			InitConfig: []provider.InitConfig{{
				Location: opts.LocalLocation,
			}},
		}

	case util.GoProvider:
		return provider.Config{
			Name:       util.GoProvider,
			BinaryPath: ContainerGenericProviderBin,
			InitConfig: []provider.InitConfig{{
				AnalysisMode: analysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					"lspServerName":                 "generic",
					provider.LspServerPathConfigKey: ContainerGoplsPath,
					"lspServerArgs":                 []string{},
					"dependencyProviderPath":        ContainerGolangDepPath,
				},
			}},
		}

	case util.PythonProvider:
		return provider.Config{
			Name:       util.PythonProvider,
			BinaryPath: ContainerGenericProviderBin,
			InitConfig: []provider.InitConfig{{
				AnalysisMode: analysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					"lspServerName":                 "generic",
					provider.LspServerPathConfigKey: ContainerPylspPath,
					"lspServerArgs":                 []string{},
					"workspaceFolders":              []string{},
					"dependencyFolders":             []string{},
				},
			}},
		}

	case util.NodeJSProvider:
		return provider.Config{
			Name:       util.NodeJSProvider,
			BinaryPath: ContainerGenericProviderBin,
			InitConfig: []provider.InitConfig{{
				AnalysisMode: analysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					"lspServerName":                 "nodejs",
					provider.LspServerPathConfigKey: ContainerTSLangServerPath,
					"lspServerArgs":                 []string{"--stdio"},
					"workspaceFolders":              []string{},
					"dependencyFolders":             []string{},
				},
			}},
		}

	case "yaml":
		return provider.Config{
			Name:       "yaml",
			BinaryPath: ContainerYqProviderBin,
			InitConfig: []provider.InitConfig{{
				AnalysisMode: analysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					"name":                          "yq",
					provider.LspServerPathConfigKey: ContainerYqPath,
				},
			}},
		}

	case util.CsharpProvider:
		return provider.Config{
			Name: util.CsharpProvider,
			InitConfig: []provider.InitConfig{{
				Location:     opts.Location,
				AnalysisMode: provider.SourceOnlyAnalysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					"ilspy_cmd": ContainerIlspyCmdPath,
					"paket_cmd": ContainerPaketCmdPath,
				},
			}},
		}
	}
	return provider.Config{}
}

// localConfig returns a provider config for containerless (local) execution.
// Provider binaries and resources are located relative to KantraDir.
func localConfig(name string, opts DefaultOptions) provider.Config {
	analysisMode := resolveAnalysisMode(opts.AnalysisMode, provider.FullAnalysisMode)
	kantraDir := opts.KantraDir

	switch name {
	case util.JavaProvider:
		cfg := provider.Config{
			Name:       util.JavaProvider,
			BinaryPath: filepath.Join(kantraDir, "java-external-provider"),
			InitConfig: []provider.InitConfig{{
				Location:     opts.Location,
				AnalysisMode: analysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					"lspServerName":                 util.JavaProvider,
					"bundles":                       filepath.Join(kantraDir, ContainerJavaBundlePath),
					provider.LspServerPathConfigKey: filepath.Join(kantraDir, ContainerJDTLSPath),
					"depOpenSourceLabelsFile":       filepath.Join(kantraDir, "maven.default.index"),
					"mavenIndexPath":                kantraDir,
					"cleanExplodedBin":              true,
					"fernFlowerPath":                filepath.Join(kantraDir, "fernflower.jar"),
					"gradleSourcesTaskFile":         filepath.Join(kantraDir, "task.gradle"),
					"disableMavenSearch":            opts.DisableMavenSearch,
				},
			}},
		}
		applyJavaOverrides(&cfg, opts)
		return cfg

	case "builtin":
		// In ModeLocal, Location IS the local path -- no distinction needed.
		return provider.Config{
			Name: "builtin",
			InitConfig: []provider.InitConfig{{
				Location:               opts.Location,
				AnalysisMode:           analysisMode,
				ProviderSpecificConfig: map[string]interface{}{},
			}},
		}
	}
	return provider.Config{}
}

// networkConfig returns a provider config for hybrid/network execution.
// Providers run in containers and are reached via network addresses.
// The ProviderSpecificConfig uses container-internal paths since the
// provider processes run inside containers.
func networkConfig(name string, opts DefaultOptions) provider.Config {
	analysisMode := resolveAnalysisMode(opts.AnalysisMode, provider.FullAnalysisMode)
	address := opts.ProviderAddresses[name]

	switch name {
	case util.JavaProvider:
		cfg := provider.Config{
			Name:    util.JavaProvider,
			Address: address,
			InitConfig: []provider.InitConfig{{
				Location:     opts.Location,
				AnalysisMode: analysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					"lspServerName":                 util.JavaProvider,
					provider.LspServerPathConfigKey: ContainerJDTLSPath,
					"bundles":                       ContainerJavaBundlePath,
					"mavenIndexPath":                ContainerMavenIndexPath,
					"depOpenSourceLabelsFile":       ContainerDepOpenSourceLabels,
				},
			}},
		}
		applyJavaOverrides(&cfg, opts)
		return cfg

	case "builtin":
		// Builtin provider always runs in-process, even in hybrid mode.
		return provider.Config{
			Name: "builtin",
			InitConfig: []provider.InitConfig{{
				Location:               opts.LocalLocation,
				AnalysisMode:           analysisMode,
				ProviderSpecificConfig: map[string]interface{}{},
			}},
		}

	case util.GoProvider:
		return provider.Config{
			Name:    util.GoProvider,
			Address: address,
			InitConfig: []provider.InitConfig{{
				Location:     opts.Location,
				AnalysisMode: analysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					"lspServerName":                 "generic",
					provider.LspServerPathConfigKey: ContainerGoplsPath,
					"dependencyProviderPath":        ContainerGolangDepPath,
				},
			}},
		}

	case util.PythonProvider:
		return provider.Config{
			Name:    util.PythonProvider,
			Address: address,
			InitConfig: []provider.InitConfig{{
				Location:     opts.Location,
				AnalysisMode: analysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					"lspServerName":                 "generic",
					provider.LspServerPathConfigKey: ContainerPylspPath,
				},
			}},
		}

	case util.NodeJSProvider:
		return provider.Config{
			Name:    util.NodeJSProvider,
			Address: address,
			InitConfig: []provider.InitConfig{{
				Location:     opts.Location,
				AnalysisMode: analysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					"lspServerName":                 "nodejs",
					provider.LspServerPathConfigKey: ContainerTSLangServerPath,
					"lspServerArgs":                 []string{"--stdio"},
				},
			}},
		}

	case util.CsharpProvider:
		return provider.Config{
			Name:    util.CsharpProvider,
			Address: address,
			InitConfig: []provider.InitConfig{{
				Location:     opts.Location,
				AnalysisMode: provider.SourceOnlyAnalysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					"ilspy_cmd": ContainerIlspyCmdPath,
					"paket_cmd": ContainerPaketCmdPath,
				},
			}},
		}
	}
	return provider.Config{}
}

// resolveAnalysisMode returns the specified mode, or the fallback if empty.
func resolveAnalysisMode(mode string, fallback provider.AnalysisMode) provider.AnalysisMode {
	if mode != "" {
		return provider.AnalysisMode(mode)
	}
	return fallback
}

// applyJavaOverrides applies optional Java-specific settings to a config.
func applyJavaOverrides(cfg *provider.Config, opts DefaultOptions) {
	if len(cfg.InitConfig) == 0 {
		return
	}
	psc := cfg.InitConfig[0].ProviderSpecificConfig
	if psc == nil {
		return
	}
	if opts.MavenSettingsFile != "" {
		psc["mavenSettingsFile"] = opts.MavenSettingsFile
	}
	if opts.JvmMaxMem != "" {
		psc["jvmMaxMem"] = opts.JvmMaxMem
	}
}
