package cmd

import "github.com/konveyor/analyzer-lsp/provider"

type Provider interface {
	GetConfigVolume(a *analyzeCommand, tmpDir string) (provider.Config, error)
}
