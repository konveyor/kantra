package provider

import (
	"context"

	"github.com/go-logr/logr"
	analyzerprovider "github.com/konveyor/analyzer-lsp/provider"
)

// Environment manages the lifecycle of analysis providers.
// Implementations handle the differences between running providers
// locally (containerless) vs in containers (hybrid). The caller
// creates an Environment via NewEnvironment and interacts with it
// through this interface without knowing which mode is active.
type Environment interface {
	// Start sets up provider infrastructure.
	//   Local: validates java/mvn/JAVA_HOME, checks .kantra directory
	//   Container: creates volumes, starts provider containers, health checks
	Start(ctx context.Context) error

	// Stop tears down provider infrastructure.
	//   Local: removes Eclipse jdtls temp directories
	//   Container: stops/removes provider containers, removes volumes
	Stop(ctx context.Context) error

	// ProviderConfigs returns the provider configurations for this environment.
	// Must be called after Start.
	//   Local: ModeLocal configs with host binary paths
	//   Container: ModeNetwork configs with localhost:PORT addresses
	ProviderConfigs() []analyzerprovider.Config

	// Rules returns rule file/directory paths for analysis.
	//   Local: kantraDir/rulesets + user rules
	//   Container: rulesets extracted from container image + user rules
	Rules(userRules []string, enableDefaults bool) ([]string, error)

	// ExtraOptions returns mode-specific options that the caller needs
	// to build analyzer options. Returns zero value for local mode.
	// The isBinaryAnalysis flag controls whether path mappings (binary)
	// or ignore-additional-builtin-configs (source) are returned.
	ExtraOptions(ctx context.Context, isBinaryAnalysis bool) ExtraEnvironmentOptions

	// PostAnalysis runs after analysis completes.
	//   Local: no-op
	//   Container: collects provider container logs
	PostAnalysis(ctx context.Context) error
}

// ExtraEnvironmentOptions carries mode-specific data that the caller
// translates into analyzer options. This avoids pkg/provider importing
// analyzer-lsp/core.
type ExtraEnvironmentOptions struct {
	// PathMappings for translating container paths to host paths
	// during binary analysis. nil for local mode or source analysis.
	PathMappings []analyzerprovider.PathMapping

	// IgnoreAdditionalBuiltinConfigs is true for source analysis in
	// container mode. The caller should pass this to
	// core.WithIgnoreAdditionalBuiltinConfigs.
	IgnoreAdditionalBuiltinConfigs bool
}

// EnvironmentConfig configures how an Environment is created.
// The Mode field determines which implementation is returned by
// NewEnvironment. Fields irrelevant to the selected mode are ignored.
type EnvironmentConfig struct {
	// Mode determines which Environment implementation is created.
	Mode ExecutionMode

	// -- Shared fields --

	// Input is the path to the source code or binary to analyze.
	Input string

	// IsFileInput indicates the input is a single file (e.g., WAR/JAR)
	// rather than a directory.
	IsFileInput bool

	// AnalysisMode (e.g., "full", "source-only").
	AnalysisMode string

	// ContextLines for code snippets in violations.
	ContextLines int

	// MavenSettingsFile path for the Java provider.
	MavenSettingsFile string

	// JvmMaxMem sets JVM max memory for the Java provider.
	JvmMaxMem string

	// HTTPProxy, HTTPSProxy, NoProxy settings.
	HTTPProxy  string
	HTTPSProxy string
	NoProxy    string

	// Log is the logger for operational messages.
	Log logr.Logger

	// -- Local mode fields (ignored in container mode) --

	// KantraDir is the local kantra installation directory (~/.kantra).
	KantraDir string

	// DisableMavenSearch disables Maven central search for the Java provider.
	DisableMavenSearch bool

	// -- Container mode fields (ignored in local mode) --

	// Providers lists the providers to start in containers.
	Providers []ProviderInfo

	// ContainerBinary is the path to the container runtime (podman/docker).
	ContainerBinary string

	// RunnerImage is the kantra runner container image (for ruleset extraction).
	RunnerImage string

	// OutputDir is the analysis output directory (used for ruleset cache).
	OutputDir string

	// EnableDefaultRulesets controls whether default rulesets are extracted.
	EnableDefaultRulesets bool

	// LogLevel for provider containers.
	LogLevel *uint32

	// Cleanup controls whether containers are cleaned up on Stop.
	Cleanup bool

	// DepFolders are additional dependency directories to mount.
	DepFolders []string

	// Version is the kantra version string (used for ruleset cache naming).
	Version string

	// HealthCheckTimeout is the maximum time to wait for a provider to
	// become ready. Zero uses the default (30s).
	HealthCheckTimeout int
}

// ProviderInfo describes a provider to start in container mode.
type ProviderInfo struct {
	// Name is the provider's canonical name (e.g., "java", "go", "python").
	Name string

	// Image is the container image for this provider.
	Image string
}

// NewEnvironment creates an Environment for the given configuration.
// The Mode field determines the implementation; the caller interacts
// only through the Environment interface.
func NewEnvironment(cfg EnvironmentConfig) Environment {
	switch cfg.Mode {
	case ModeLocal:
		return newLocalEnvironment(cfg)
	case ModeNetwork:
		return newContainerEnvironment(cfg)
	default:
		return newContainerEnvironment(cfg)
	}
}
