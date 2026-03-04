package analyze

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_analyzeCommand_writeProvConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "prov-config-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configs := []provider.Config{
		{
			Name: "java",
			InitConfig: []provider.InitConfig{
				{
					AnalysisMode: provider.FullAnalysisMode,
					ProviderSpecificConfig: map[string]interface{}{
						"lspServerName": "java",
					},
				},
			},
		},
		{
			Name: "builtin",
		},
	}

	a := &analyzeCommand{
		AnalyzeCommandContext: AnalyzeCommandContext{log: testr.NewWithOptions(t, testr.Options{Verbosity: 5})},
	}

	err = a.writeProvConfig(tmpDir, configs)
	require.NoError(t, err)

	// Verify file was created and is valid JSON
	settingsPath := filepath.Join(tmpDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)

	var parsed []provider.Config
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	require.Len(t, parsed, 2)
	assert.Equal(t, "java", parsed[0].Name)
}

func Test_analyzeCommand_writeProvConfig_EmptyConfigs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "prov-config-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	a := &analyzeCommand{
		AnalyzeCommandContext: AnalyzeCommandContext{log: testr.NewWithOptions(t, testr.Options{Verbosity: 5})},
	}

	err = a.writeProvConfig(tmpDir, []provider.Config{})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(tmpDir, "settings.json"))
	require.NoError(t, err)

	var parsed []provider.Config
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Empty(t, parsed)
}

func Test_analyzeCommand_writeProvConfig_InvalidDir(t *testing.T) {
	a := &analyzeCommand{
		AnalyzeCommandContext: AnalyzeCommandContext{log: testr.NewWithOptions(t, testr.Options{Verbosity: 5})},
	}

	err := a.writeProvConfig("/non/existent/dir", []provider.Config{})
	require.Error(t, err)
}

func Test_analyzeCommand_mergeProviderConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "merge-config-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	a := &analyzeCommand{
		AnalyzeCommandContext: AnalyzeCommandContext{log: testr.NewWithOptions(t, testr.Options{Verbosity: 5})},
	}

	defaultConf := []provider.Config{
		{
			Name:         "java",
			ContextLines: 10,
			InitConfig: []provider.InitConfig{
				{
					AnalysisMode: provider.FullAnalysisMode,
					ProviderSpecificConfig: map[string]interface{}{
						"lspServerName": "java",
						"bundles":       "/path/to/bundles",
					},
				},
			},
		},
		{
			Name: "builtin",
		},
	}

	optionsConf := []provider.Config{
		{
			Name:         "java",
			ContextLines: 50,
			InitConfig: []provider.InitConfig{
				{
					ProviderSpecificConfig: map[string]interface{}{
						"jvmMaxMem": "4096m",
					},
				},
			},
		},
	}

	merged, err := a.mergeProviderConfig(defaultConf, optionsConf, tmpDir)
	require.NoError(t, err)

	// Find java config in merged
	var javaConfig *provider.Config
	for i := range merged {
		if merged[i].Name == "java" {
			javaConfig = &merged[i]
			break
		}
	}

	require.NotNil(t, javaConfig, "merged config missing java provider")
	assert.Equal(t, 50, javaConfig.ContextLines)

	// Verify jvmMaxMem was merged into ProviderSpecificConfig
	assert.Equal(t, "4096m", javaConfig.InitConfig[0].ProviderSpecificConfig["jvmMaxMem"])
}

func Test_analyzeCommand_mergeProviderConfig_UnknownProviderIgnored(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "merge-config-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	a := &analyzeCommand{
		AnalyzeCommandContext: AnalyzeCommandContext{log: testr.NewWithOptions(t, testr.Options{Verbosity: 5})},
	}

	defaultConf := []provider.Config{
		{Name: "java"},
	}

	optionsConf := []provider.Config{
		{Name: "unknown-provider", ContextLines: 99},
	}

	merged, err := a.mergeProviderConfig(defaultConf, optionsConf, tmpDir)
	require.NoError(t, err)
	require.Len(t, merged, 1)
	assert.Equal(t, "java", merged[0].Name)
}

func Test_analyzeCommand_mergeProviderConfig_ProxyOverride(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "merge-config-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	a := &analyzeCommand{
		AnalyzeCommandContext: AnalyzeCommandContext{log: testr.NewWithOptions(t, testr.Options{Verbosity: 5})},
	}

	defaultConf := []provider.Config{
		{Name: "java"},
	}

	proxy := &provider.Proxy{
		HTTPProxy:  "http://proxy:8080",
		HTTPSProxy: "https://proxy:8443",
	}
	optionsConf := []provider.Config{
		{Name: "java", Proxy: proxy},
	}

	merged, err := a.mergeProviderConfig(defaultConf, optionsConf, tmpDir)
	require.NoError(t, err)

	require.NotNil(t, merged[0].Proxy, "proxy should have been merged")
	assert.Equal(t, "http://proxy:8080", merged[0].Proxy.HTTPProxy)
}

// Note: loadOverrideProviderSettings tests are in hybrid_inprocess_test.go

func Test_analyzeCommand_getProviderOptions_NonExistentConfigDir(t *testing.T) {
	a := &analyzeCommand{
		AnalyzeCommandContext: AnalyzeCommandContext{log: testr.NewWithOptions(t, testr.Options{Verbosity: 5})},
	}

	tmpDir, err := os.MkdirTemp("", "prov-opts-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// This should return an error since there's no config file
	err = a.getProviderOptions(tmpDir, []provider.Config{}, "java")
	require.Error(t, err)
}

func Test_analyzeCommand_validateProviders(t *testing.T) {
	tests := []struct {
		name      string
		providers []string
		wantErr   bool
	}{
		{
			name:      "valid single provider",
			providers: []string{"java"},
			wantErr:   false,
		},
		{
			name:      "valid multiple providers",
			providers: []string{"java", "go", "python"},
			wantErr:   false,
		},
		{
			name:      "all valid providers",
			providers: []string{"java", "go", "python", "nodejs", "csharp"},
			wantErr:   false,
		},
		{
			name:      "empty providers",
			providers: []string{},
			wantErr:   false,
		},
		{
			name:      "unsupported provider returns error",
			providers: []string{"ruby"},
			wantErr:   true,
		},
		{
			name:      "mix of valid and invalid providers",
			providers: []string{"java", "invalid-provider"},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &analyzeCommand{
				AnalyzeCommandContext: AnalyzeCommandContext{log: testr.NewWithOptions(t, testr.Options{Verbosity: 5})},
			}

			err := a.validateProviders(tt.providers)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
