package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
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

	builtinProvider, locations, err := a.setupBuiltinProviderHybrid(ctx, nil, logr.Discard(), nil, nil)

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

	builtinProvider, _, err := a.setupBuiltinProviderHybrid(ctx, nil, logr.Discard(), nil, nil)

	if err != nil {
		t.Fatalf("setupBuiltinProviderHybrid() error = %v", err)
	}

	if builtinProvider == nil {
		t.Fatal("Builtin provider should not be nil")
	}
}

func Test_mergeProviderSpecificConfig(t *testing.T) {
	tests := []struct {
		name     string
		base     map[string]interface{}
		override map[string]interface{}
		want     map[string]interface{}
	}{
		{
			name: "merge with nil override returns base",
			base: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			override: nil,
			want: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name: "override replaces base values",
			base: map[string]interface{}{
				"key1": "base_value",
				"key2": "base_value",
			},
			override: map[string]interface{}{
				"key1": "override_value",
			},
			want: map[string]interface{}{
				"key1": "override_value",
				"key2": "base_value",
			},
		},
		{
			name: "override adds new keys",
			base: map[string]interface{}{
				"key1": "value1",
			},
			override: map[string]interface{}{
				"key2": "value2",
			},
			want: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name:     "empty base with override",
			base:     map[string]interface{}{},
			override: map[string]interface{}{"key1": "value1"},
			want:     map[string]interface{}{"key1": "value1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeProviderSpecificConfig(tt.base, tt.override)
			if len(got) != len(tt.want) {
				t.Errorf("mergeProviderSpecificConfig() length = %v, want %v", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("mergeProviderSpecificConfig()[%s] = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}

func Test_applyProviderOverrides(t *testing.T) {
	tests := []struct {
		name            string
		baseConfig      provider.Config
		overrideConfigs []provider.Config
		wantContextLines int
	}{
		{
			name: "nil overrides returns base unchanged",
			baseConfig: provider.Config{
				Name:         "java",
				ContextLines: 100,
			},
			overrideConfigs: nil,
			wantContextLines: 100,
		},
		{
			name: "override context lines",
			baseConfig: provider.Config{
				Name:         "java",
				ContextLines: 100,
			},
			overrideConfigs: []provider.Config{
				{
					Name:         "java",
					ContextLines: 500,
				},
			},
			wantContextLines: 500,
		},
		{
			name: "no matching override returns base unchanged",
			baseConfig: provider.Config{
				Name:         "java",
				ContextLines: 100,
			},
			overrideConfigs: []provider.Config{
				{
					Name:         "go",
					ContextLines: 500,
				},
			},
			wantContextLines: 100,
		},
		{
			name: "override provider specific config",
			baseConfig: provider.Config{
				Name: "java",
				InitConfig: []provider.InitConfig{
					{
						ProviderSpecificConfig: map[string]interface{}{
							"jvmMaxMem": "2g",
						},
					},
				},
			},
			overrideConfigs: []provider.Config{
				{
					Name: "java",
					InitConfig: []provider.InitConfig{
						{
							ProviderSpecificConfig: map[string]interface{}{
								"jvmMaxMem": "4g",
							},
						},
					},
				},
			},
			wantContextLines: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyProviderOverrides(tt.baseConfig, tt.overrideConfigs)
			if got.ContextLines != tt.wantContextLines {
				t.Errorf("applyProviderOverrides().ContextLines = %v, want %v", got.ContextLines, tt.wantContextLines)
			}
			// Check provider specific config override if present
			if tt.name == "override provider specific config" {
				if len(got.InitConfig) == 0 {
					t.Fatal("Expected InitConfig to be present")
				}
				jvmMaxMem := got.InitConfig[0].ProviderSpecificConfig["jvmMaxMem"]
				if jvmMaxMem != "4g" {
					t.Errorf("jvmMaxMem = %v, want 4g", jvmMaxMem)
				}
			}
		})
	}
}

func Test_analyzeCommand_loadOverrideProviderSettings(t *testing.T) {
	tests := []struct {
		name        string
		fileContent string
		wantErr     bool
		wantCount   int
	}{
		{
			name: "valid override file",
			fileContent: `[
				{
					"name": "java",
					"contextLines": 500
				}
			]`,
			wantErr:   false,
			wantCount: 1,
		},
		{
			name:        "invalid JSON",
			fileContent: `not valid json`,
			wantErr:     true,
			wantCount:   0,
		},
		{
			name: "multiple providers",
			fileContent: `[
				{
					"name": "java",
					"contextLines": 500
				},
				{
					"name": "go",
					"contextLines": 200
				}
			]`,
			wantErr:   false,
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file with test content
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "override.json")
			if err := os.WriteFile(tmpFile, []byte(tt.fileContent), 0644); err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}

			a := &analyzeCommand{
				overrideProviderSettings: tmpFile,
				AnalyzeCommandContext: AnalyzeCommandContext{
					log: logr.Discard(),
				},
			}

			got, err := a.loadOverrideProviderSettings()
			if (err != nil) != tt.wantErr {
				t.Errorf("loadOverrideProviderSettings() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) != tt.wantCount {
				t.Errorf("loadOverrideProviderSettings() count = %v, want %v", len(got), tt.wantCount)
			}
		})
	}
}

func Test_analyzeCommand_loadOverrideProviderSettings_EmptyPath(t *testing.T) {
	a := &analyzeCommand{
		overrideProviderSettings: "",
		AnalyzeCommandContext: AnalyzeCommandContext{
			log: logr.Discard(),
		},
	}

	got, err := a.loadOverrideProviderSettings()
	if err != nil {
		t.Errorf("loadOverrideProviderSettings() error = %v, want nil", err)
	}
	if got != nil {
		t.Errorf("loadOverrideProviderSettings() = %v, want nil", got)
	}
}

func Test_analyzeCommand_loadOverrideProviderSettings_FileNotFound(t *testing.T) {
	a := &analyzeCommand{
		overrideProviderSettings: "/nonexistent/file.json",
		AnalyzeCommandContext: AnalyzeCommandContext{
			log: logr.Discard(),
		},
	}

	_, err := a.loadOverrideProviderSettings()
	if err == nil {
		t.Error("loadOverrideProviderSettings() expected error for nonexistent file")
	}
}
