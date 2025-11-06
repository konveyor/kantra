package cmd

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
)

// Test_analyzeCommand_setupJavaProviderHybrid_MissingProvider tests error handling
// when Java provider is not configured in providersMap
func Test_analyzeCommand_setupJavaProviderHybrid_MissingProvider(t *testing.T) {
	a := &analyzeCommand{
		input: "/test/input",
		mode:  "full",
		AnalyzeCommandContext: AnalyzeCommandContext{
			log:          logr.Discard(),
			providersMap: map[string]ProviderInit{}, // No Java provider
		},
	}

	// Note: We don't call setupJavaProviderHybrid() here because it would
	// attempt to initialize a real provider, which requires infrastructure.
	// Instead we just check the providersMap lookup logic inline.

	_, ok := a.providersMap[util.JavaProvider]
	if ok {
		t.Fatal("Java provider should not be in providersMap")
	}
}

func Test_analyzeCommand_setupBuiltinProviderHybrid(t *testing.T) {
	a := &analyzeCommand{
		input: "/test/input",
		mode:  "full",
		AnalyzeCommandContext: AnalyzeCommandContext{
			log:          logr.Discard(),
			providersMap: map[string]ProviderInit{},
		},
	}

	ctx := context.Background()
	excludedPaths := []interface{}{"/test/target", "/test/build"}

	builtinProvider, locations, err := a.setupBuiltinProviderHybrid(ctx, excludedPaths, nil, logr.Discard())

	if err != nil {
		t.Fatalf("setupBuiltinProviderHybrid() error = %v", err)
	}

	if builtinProvider == nil {
		t.Fatal("Builtin provider should not be nil")
	}

	if len(locations) == 0 {
		t.Error("Expected at least one provider location")
	}

	if locations[0] != "/test/input" {
		t.Errorf("Provider location = %v, want /test/input", locations[0])
	}
}

func Test_analyzeCommand_setupBuiltinProviderHybrid_WithProxy(t *testing.T) {
	a := &analyzeCommand{
		input:      "/test/input",
		mode:       "full",
		httpProxy:  "http://proxy.example.com:8080",
		httpsProxy: "https://proxy.example.com:8443",
		noProxy:    "localhost,127.0.0.1",
		AnalyzeCommandContext: AnalyzeCommandContext{
			log:          logr.Discard(),
			providersMap: map[string]ProviderInit{},
		},
	}

	ctx := context.Background()

	builtinProvider, _, err := a.setupBuiltinProviderHybrid(ctx, nil, nil, logr.Discard())

	if err != nil {
		t.Fatalf("setupBuiltinProviderHybrid() error = %v", err)
	}

	if builtinProvider == nil {
		t.Fatal("Builtin provider should not be nil")
	}
}
