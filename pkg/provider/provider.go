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
}

type Provider interface {
	GetConfigVolume(input ConfigInput) (provider.Config, error)
}
