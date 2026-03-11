package provider

import (
	"github.com/konveyor/analyzer-lsp/provider"
)

type BuiltinProvider struct {
	baseProvider
	config provider.Config
}

func (p *BuiltinProvider) Name() string {
	return "builtin"
}

func (p *BuiltinProvider) GetConfig(mode ExecutionMode, opts BaseOptions, extra ...ProviderOption) (provider.Config, error) {
	cfg := NewBaseConfig("builtin", mode, opts)
	// Builtin has no binary, no LSP — it's always in-process.
	// Clear any address/binary that base might have set.
	cfg.Address = ""
	cfg.BinaryPath = ""
	return cfg, nil
}
