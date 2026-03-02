package provider

import (
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
)

type ConfigInput struct {
	Name                    string
	IsFileInput             bool
	InputPath               string
	OutputPath              string
	MavenSettingsFile       string
	Log                     logr.Logger
	Mode                    string
	Port                    int
	JvmMaxMem               string
	TmpDir                  string
	DepsFolders             []string
	JavaExcludedTargetPaths []interface{}
	DisableMavenSearch      bool
	JavaBundleLocation      string
	// ContainerSourcePath is the resolved path to the source inside the container.
	// For directory inputs: /opt/input/source
	// For file inputs:      /opt/input/source/app.war
	// This replaces direct reads of util.SourceMountPath in provider implementations.
	ContainerSourcePath string
}

type Provider interface {
	GetConfigVolume(input ConfigInput) (provider.Config, error)
	SupportsLogLevel() bool
}

type baseProvider struct{}

func (b *baseProvider) SupportsLogLevel() bool {
	return true
}
