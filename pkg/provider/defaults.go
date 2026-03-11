package provider

import (
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

	// Java provider resources (container absolute paths)
	ContainerJDTLSPath           = "/jdtls/bin/jdtls"
	ContainerJavaBundlePath      = "/jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar"
	ContainerMavenIndexPath      = "/usr/local/etc/maven-index.txt"
	ContainerDepOpenSourceLabels = "/usr/local/etc/maven.default.index"

	// Java provider resources (local relative paths for ModeLocal, relative to KantraDir)
	LocalJDTLSPath      = "jdtls/bin/jdtls"
	LocalJavaBundlePath = "jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar"

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

// providerRegistry maps provider names to their Provider implementation.
// Each provider owns its own configuration logic via GetConfig.
var providerRegistry = map[string]Provider{
	util.JavaProvider:   &JavaProvider{},
	"builtin":           &BuiltinProvider{},
	util.GoProvider:     &GoProvider{},
	util.PythonProvider: &PythonProvider{},
	util.NodeJSProvider: &NodeJsProvider{},
	"yaml":              &YamlProvider{},
	util.CsharpProvider: &CsharpProvider{},
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

	// InputPath is the original host-side input path (used for excluded dirs).
	InputPath string

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
// Each provider's GetConfig method handles its own provider-specific config
// and delegates common concerns (excluded dirs, proxy, analysis mode) to
// NewBaseConfig.
//
// The returned configs can be:
//   - Serialized to JSON for konveyor.NewAnalyzer (via WithProviderConfigFilePath)
//   - Used directly by the test runner
//   - Customized by the caller (e.g., merging overrides)
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
		p, ok := providerRegistry[name]
		if !ok {
			continue
		}

		// Build BaseOptions from DefaultOptions
		baseOpts := BaseOptions{
			Location:      opts.Location,
			LocalLocation: opts.LocalLocation,
			AnalysisMode:  opts.AnalysisMode,
			InputPath:     opts.InputPath,
			KantraDir:     opts.KantraDir,
			ContextLines:  opts.ContextLines,
			HTTPProxy:     opts.HTTPProxy,
			HTTPSProxy:    opts.HTTPSProxy,
			NoProxy:       opts.NoProxy,
		}

		// Set network address if available
		if mode == ModeNetwork && opts.ProviderAddresses != nil {
			baseOpts.Address = opts.ProviderAddresses[name]
		}

		// Build provider-specific options
		extraOpts := []ProviderOption{
			JavaOptions{
				MavenSettingsFile:  opts.MavenSettingsFile,
				JvmMaxMem:          opts.JvmMaxMem,
				DisableMavenSearch: opts.DisableMavenSearch,
			},
		}

		cfg, err := p.GetConfig(mode, baseOpts, extraOpts...)
		if err != nil || cfg.Name == "" {
			continue
		}

		configs = append(configs, cfg)
	}
	return configs
}

// resolveAnalysisMode returns the specified mode, or the fallback if empty.
func resolveAnalysisMode(mode string, fallback provider.AnalysisMode) provider.AnalysisMode {
	if mode != "" {
		return provider.AnalysisMode(mode)
	}
	return fallback
}
