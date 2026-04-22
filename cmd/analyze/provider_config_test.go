package analyze

import (
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_analyzeCommand_validateProviders(t *testing.T) {
	tests := []struct {
		name          string
		providers     []string
		overrideNames map[string]bool
		wantErr       bool
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
		{
			name:          "override provider is accepted",
			providers:     []string{"my-custom-provider"},
			overrideNames: map[string]bool{"my-custom-provider": true},
			wantErr:       false,
		},
		{
			name:          "mix of builtin and override providers",
			providers:     []string{"java", "my-custom-provider"},
			overrideNames: map[string]bool{"my-custom-provider": true},
			wantErr:       false,
		},
		{
			name:          "unknown provider not in overrides still fails",
			providers:     []string{"unknown"},
			overrideNames: map[string]bool{"my-custom-provider": true},
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &analyzeCommand{
				AnalyzeCommandContext: AnalyzeCommandContext{log: testr.NewWithOptions(t, testr.Options{Verbosity: 5})},
			}

			overrideNames := tt.overrideNames
			if overrideNames == nil {
				overrideNames = map[string]bool{}
			}
			err := a.validateProviders(tt.providers, overrideNames)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_externalProviderNames(t *testing.T) {
	tests := []struct {
		name      string
		overrides []provider.Config
		want      []string
	}{
		{
			name:      "nil overrides",
			overrides: nil,
			want:      nil,
		},
		{
			name:      "empty overrides",
			overrides: []provider.Config{},
			want:      nil,
		},
		{
			name: "override with address returns name",
			overrides: []provider.Config{
				{Name: "my-provider", Address: "localhost:9999"},
			},
			want: []string{"my-provider"},
		},
		{
			name: "override without address excluded",
			overrides: []provider.Config{
				{Name: "java"}, // no Address — tweaking existing provider settings
			},
			want: nil,
		},
		{
			name: "mix of addressed and non-addressed",
			overrides: []provider.Config{
				{Name: "java"},
				{Name: "my-provider", Address: "localhost:9999"},
				{Name: "another-provider", Address: "localhost:8888"},
			},
			want: []string{"my-provider", "another-provider"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := externalProviderNames(tt.overrides)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_overrideProviderNameSet(t *testing.T) {
	tests := []struct {
		name      string
		overrides []provider.Config
		want      map[string]bool
	}{
		{
			name:      "empty overrides",
			overrides: []provider.Config{},
			want:      map[string]bool{},
		},
		{
			name: "returns all names regardless of address",
			overrides: []provider.Config{
				{Name: "java"},
				{Name: "my-provider", Address: "localhost:9999"},
			},
			want: map[string]bool{"java": true, "my-provider": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := overrideProviderNameSet(tt.overrides)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_hasNonStandardExternalProviders(t *testing.T) {
	tests := []struct {
		name      string
		overrides []provider.Config
		want      bool
	}{
		{
			name:      "nil overrides",
			overrides: nil,
			want:      false,
		},
		{
			name: "standard provider with address is not non-standard",
			overrides: []provider.Config{
				{Name: "java", Address: "localhost:9999"},
			},
			want: false,
		},
		{
			name: "standard provider without address is not non-standard",
			overrides: []provider.Config{
				{Name: "java"},
			},
			want: false,
		},
		{
			name: "non-standard provider with address is detected",
			overrides: []provider.Config{
				{Name: "my-custom-provider", Address: "localhost:9999"},
			},
			want: true,
		},
		{
			name: "non-standard provider without address is not detected",
			overrides: []provider.Config{
				{Name: "my-custom-provider"}, // no address — not externally managed
			},
			want: false,
		},
		{
			name: "mix: standard with address + non-standard with address",
			overrides: []provider.Config{
				{Name: "java", Address: "localhost:8888"},
				{Name: "my-custom-provider", Address: "localhost:9999"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasNonStandardExternalProviders(tt.overrides)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_isExternalOnly(t *testing.T) {
	tests := []struct {
		name           string
		foundProviders []string
		externalNames  []string
		want           bool
	}{
		{
			name:           "no providers and no externals",
			foundProviders: []string{},
			externalNames:  []string{},
			want:           false,
		},
		{
			name:           "no detected providers but externals exist",
			foundProviders: []string{},
			externalNames:  []string{"my-provider"},
			want:           true,
		},
		{
			name:           "detected provider matches external",
			foundProviders: []string{"my-provider"},
			externalNames:  []string{"my-provider"},
			want:           true,
		},
		{
			name:           "detected provider is not external",
			foundProviders: []string{"java"},
			externalNames:  []string{"my-provider"},
			want:           false,
		},
		{
			name:           "mix of detected and external — java blocks external-only",
			foundProviders: []string{"java", "my-provider"},
			externalNames:  []string{"my-provider"},
			want:           false,
		},
		{
			name:           "multiple external providers all match",
			foundProviders: []string{"provider-a", "provider-b"},
			externalNames:  []string{"provider-a", "provider-b"},
			want:           true,
		},
		{
			name:           "external names exist but no detected providers",
			foundProviders: nil,
			externalNames:  []string{"my-provider"},
			want:           true,
		},
		{
			name:           "detected providers but no external names",
			foundProviders: []string{"java"},
			externalNames:  nil,
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isExternalOnly(tt.foundProviders, tt.externalNames)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_applyAllProviderOverrides_externalProvider(t *testing.T) {
	t.Run("external provider appended to builtin-only base", func(t *testing.T) {
		baseConfigs := []provider.Config{
			{Name: "builtin"},
		}
		overrideConfigs := []provider.Config{
			{
				Name:    "my-provider",
				Address: "localhost:9999",
				InitConfig: []provider.InitConfig{{
					AnalysisMode:           "source-only",
					ProviderSpecificConfig: map[string]interface{}{"key": "value"},
				}},
			},
		}

		result := applyAllProviderOverrides(baseConfigs, overrideConfigs)
		require.Len(t, result, 2)
		assert.Equal(t, "builtin", result[0].Name)
		assert.Equal(t, "my-provider", result[1].Name)
		assert.Equal(t, "localhost:9999", result[1].Address)
		assert.Equal(t, "value", result[1].InitConfig[0].ProviderSpecificConfig["key"])
	})

	t.Run("external provider appended alongside java and builtin", func(t *testing.T) {
		baseConfigs := []provider.Config{
			{Name: "java"},
			{Name: "builtin"},
		}
		overrideConfigs := []provider.Config{
			{Name: "my-provider", Address: "localhost:9999"},
		}

		result := applyAllProviderOverrides(baseConfigs, overrideConfigs)
		require.Len(t, result, 3)
		assert.Equal(t, "java", result[0].Name)
		assert.Equal(t, "builtin", result[1].Name)
		assert.Equal(t, "my-provider", result[2].Name)
	})

	t.Run("override for existing provider merges not appends", func(t *testing.T) {
		baseConfigs := []provider.Config{
			{
				Name: "java",
				InitConfig: []provider.InitConfig{{
					ProviderSpecificConfig: map[string]interface{}{"bundles": "/default/path"},
				}},
			},
		}
		overrideConfigs := []provider.Config{
			{
				Name: "java",
				InitConfig: []provider.InitConfig{{
					ProviderSpecificConfig: map[string]interface{}{"mavenSettingsFile": "/custom/settings.xml"},
				}},
			},
		}

		result := applyAllProviderOverrides(baseConfigs, overrideConfigs)
		require.Len(t, result, 1, "should merge, not append")
		assert.Equal(t, "java", result[0].Name)
		psc := result[0].InitConfig[0].ProviderSpecificConfig
		assert.Equal(t, "/default/path", psc["bundles"], "base config preserved")
		assert.Equal(t, "/custom/settings.xml", psc["mavenSettingsFile"], "override applied")
	})

	t.Run("nil overrides returns base unchanged", func(t *testing.T) {
		baseConfigs := []provider.Config{{Name: "builtin"}}
		result := applyAllProviderOverrides(baseConfigs, nil)
		require.Len(t, result, 1)
		assert.Equal(t, "builtin", result[0].Name)
	})
}
