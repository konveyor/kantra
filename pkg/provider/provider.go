package provider

import (
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
)

// ProviderOption carries provider-specific configuration to GetConfig.
// Providers inspect options they understand via type assertion and ignore
// the rest, keeping the interface uniform while allowing extensibility.
type ProviderOption interface {
	providerOption() // marker method to restrict implementations to this package
}

// JavaOptions carries Java provider-specific configuration.
// Pass as a ProviderOption to GetConfig; non-Java providers ignore it.
type JavaOptions struct {
	MavenSettingsFile  string
	JvmMaxMem          string
	DisableMavenSearch bool
}

func (JavaOptions) providerOption() {}

// Provider generates provider configuration for the analyzer engine.
// Each implementation owns its provider-specific config (binary paths, LSP
// settings, etc.) and delegates common concerns to NewBaseConfig.
type Provider interface {
	// GetConfig returns the provider config for the given execution mode.
	// Implementations should call NewBaseConfig first, then layer on
	// provider-specific settings. Provider-specific options (e.g., JavaOptions)
	// are passed via the variadic extra parameter.
	GetConfig(mode ExecutionMode, opts BaseOptions, extra ...ProviderOption) (provider.Config, error)

	// SupportsLogLevel reports whether the provider supports log level configuration.
	SupportsLogLevel() bool

	// Name returns the provider's canonical name.
	Name() string
}

// BaseOptions carries the shared options that all providers need.
type BaseOptions struct {
	// Location is the source code path.
	//   ModeLocal: local filesystem path
	//   ModeContainer: container-internal path (or empty, set later)
	//   ModeNetwork: container-internal path where source is mounted
	Location string

	// LocalLocation is the host-side source path (used by builtin in network mode).
	LocalLocation string

	// AnalysisMode (e.g., "full", "source-only"). Empty uses provider defaults.
	AnalysisMode string

	// InputPath is the original host-side input path (used for excluded dirs detection).
	InputPath string

	// Address is the network address for this provider. Takes precedence
	// over BinaryPath when set. Used by ModeNetwork and the container
	// flow where providers run in separate containers.
	Address string

	// BinaryPath override for the provider binary (ModeLocal/ModeContainer).
	BinaryPath string

	// Proxy settings applied to all providers.
	HTTPProxy, HTTPSProxy, NoProxy string

	// ContextLines for code snippets.
	ContextLines int

	// KantraDir is the local kantra installation directory (ModeLocal).
	KantraDir string
}

// FindOption extracts a ProviderOption of type T from the variadic extras.
// Returns the zero value and false if not found.
func FindOption[T ProviderOption](extra []ProviderOption) (T, bool) {
	for _, opt := range extra {
		if t, ok := opt.(T); ok {
			return t, true
		}
	}
	var zero T
	return zero, false
}

// baseProvider provides default implementations for Provider methods.
type baseProvider struct{}

func (b *baseProvider) SupportsLogLevel() bool {
	return true
}

// NewBaseConfig creates a skeleton provider.Config with the common fields
// that all providers share: name, address/binary path, location, analysis
// mode, excluded dirs, and proxy settings.
//
// Individual providers call this first, then add their provider-specific
// config on top of the returned config.
func NewBaseConfig(name string, mode ExecutionMode, opts BaseOptions) provider.Config {
	analysisMode := resolveAnalysisMode(opts.AnalysisMode, provider.FullAnalysisMode)

	cfg := provider.Config{
		Name: name,
		InitConfig: []provider.InitConfig{{
			Location:               resolveLocation(name, mode, opts),
			AnalysisMode:           analysisMode,
			ProviderSpecificConfig: map[string]interface{}{},
		}},
	}

	// Set connectivity: address takes precedence if provided (used by
	// both ModeNetwork and the legacy container flow where providers run
	// in separate containers). Otherwise use binary path.
	if opts.Address != "" {
		cfg.Address = opts.Address
	} else if opts.BinaryPath != "" {
		cfg.BinaryPath = opts.BinaryPath
	}

	// Context lines
	if opts.ContextLines > 0 {
		cfg.ContextLines = opts.ContextLines
	}

	// Proxy
	if opts.HTTPProxy != "" || opts.HTTPSProxy != "" {
		cfg.Proxy = &provider.Proxy{
			HTTPProxy:  opts.HTTPProxy,
			HTTPSProxy: opts.HTTPSProxy,
			NoProxy:    opts.NoProxy,
		}
	}

	// Excluded dirs from profiles
	if opts.InputPath != "" {
		useContainerPath := mode != ModeLocal
		containerDir := util.SourceMountPath
		if !useContainerPath {
			containerDir = ""
		}
		if excludedDir := util.GetProfilesExcludedDir(opts.InputPath, containerDir, useContainerPath); excludedDir != "" {
			cfg.InitConfig[0].ProviderSpecificConfig["excludedDirs"] = []interface{}{excludedDir}
		}
	}

	return cfg
}

// resolveLocation determines the source location based on provider name and mode.
func resolveLocation(name string, mode ExecutionMode, opts BaseOptions) string {
	// Builtin in network mode uses local location (it runs in-process on the host)
	if name == "builtin" && mode == ModeNetwork {
		return opts.LocalLocation
	}
	return opts.Location
}
