package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mkMinimalKantraDir creates a kantra directory with only the base dir.
// No rulesets, no Java bundles — suitable for ExternalOnly mode.
func mkMinimalKantraDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

// mkKantraDirWithRulesets creates a kantra directory with a rulesets subdirectory.
func mkKantraDirWithRulesets(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	err := os.MkdirAll(filepath.Join(dir, rulesetsSubdir), 0755)
	require.NoError(t, err)
	return dir
}

// mkKantraDirWithRulesetSubdirs creates a kantra directory with rulesets/<subdirs>.
func mkKantraDirWithRulesetSubdirs(t *testing.T, subdirs ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, s := range subdirs {
		err := os.MkdirAll(filepath.Join(dir, rulesetsSubdir, s), 0o755)
		require.NoError(t, err)
	}
	return dir
}

func TestLocalEnvironment_Start_ExternalOnly_NoJavaRequired(t *testing.T) {
	// Create a minimal kantra dir with no Java artifacts.
	// ExternalOnly + EnableDefaultRulesets=false should succeed
	// without mvn, java, JAVA_HOME, or Java bundles.
	kantraDir := mkMinimalKantraDir(t)
	inputDir := t.TempDir()

	env := newLocalEnvironment(EnvironmentConfig{
		Mode:                 ModeLocal,
		Input:                inputDir,
		KantraDir:            kantraDir,
		ExternalOnly:         true,
		EnableDefaultRulesets: false,
		Log:                  logr.Discard(),
	})

	err := env.Start(context.Background())
	require.NoError(t, err, "Start() should succeed in external-only mode without Java")
}

func TestLocalEnvironment_Start_ExternalOnly_ProviderConfigsBuiltinOnly(t *testing.T) {
	// When ExternalOnly=true, ProviderConfigs() should return
	// only the builtin provider — no Java provider.
	kantraDir := mkMinimalKantraDir(t)
	inputDir := t.TempDir()

	env := newLocalEnvironment(EnvironmentConfig{
		Mode:                 ModeLocal,
		Input:                inputDir,
		KantraDir:            kantraDir,
		ExternalOnly:         true,
		EnableDefaultRulesets: false,
		Log:                  logr.Discard(),
	})

	err := env.Start(context.Background())
	require.NoError(t, err)

	configs := env.ProviderConfigs()
	require.Len(t, configs, 1, "should return only builtin provider")
	assert.Equal(t, "builtin", configs[0].Name)
}

func TestLocalEnvironment_Start_ExternalOnly_WithDefaultRulesets(t *testing.T) {
	// ExternalOnly with EnableDefaultRulesets=true requires rulesets dir to exist.
	kantraDir := mkKantraDirWithRulesets(t)
	inputDir := t.TempDir()

	env := newLocalEnvironment(EnvironmentConfig{
		Mode:                 ModeLocal,
		Input:                inputDir,
		KantraDir:            kantraDir,
		ExternalOnly:         true,
		EnableDefaultRulesets: true,
		Log:                  logr.Discard(),
	})

	err := env.Start(context.Background())
	require.NoError(t, err, "Start() should succeed when rulesets dir exists")

	// Rules should include the default rulesets path
	rules, err := env.Rules(nil, true)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, filepath.Join(kantraDir, rulesetsSubdir), rules[0])
}

func TestLocalEnvironment_Start_ExternalOnly_MissingRulesetsDir(t *testing.T) {
	// ExternalOnly with EnableDefaultRulesets=true should fail when
	// the rulesets directory does not exist.
	kantraDir := mkMinimalKantraDir(t) // no rulesets subdir
	inputDir := t.TempDir()

	env := newLocalEnvironment(EnvironmentConfig{
		Mode:                 ModeLocal,
		Input:                inputDir,
		KantraDir:            kantraDir,
		ExternalOnly:         true,
		EnableDefaultRulesets: true,
		Log:                  logr.Discard(),
	})

	err := env.Start(context.Background())
	require.Error(t, err, "Start() should fail when rulesets dir is missing and default rulesets enabled")
}

func TestLocalEnvironment_Start_ExternalOnly_RulesWithoutDefaults(t *testing.T) {
	// When EnableDefaultRulesets=false, Rules() should not include
	// default rulesets path even if the dir exists.
	kantraDir := mkKantraDirWithRulesets(t)
	inputDir := t.TempDir()

	env := newLocalEnvironment(EnvironmentConfig{
		Mode:                 ModeLocal,
		Input:                inputDir,
		KantraDir:            kantraDir,
		ExternalOnly:         true,
		EnableDefaultRulesets: false,
		Log:                  logr.Discard(),
	})

	err := env.Start(context.Background())
	require.NoError(t, err)

	userRules := []string{"/path/to/my/rules"}
	rules, err := env.Rules(userRules, false)
	require.NoError(t, err)
	require.Len(t, rules, 1, "should contain only user rules")
	assert.Equal(t, "/path/to/my/rules", rules[0])
}

func TestLocalEnvironment_Start_InputIsCwd(t *testing.T) {
	// Setting input to the current directory should fail regardless
	// of ExternalOnly setting.
	kantraDir := mkMinimalKantraDir(t)
	cwd, err := os.Getwd()
	require.NoError(t, err)

	env := newLocalEnvironment(EnvironmentConfig{
		Mode:                 ModeLocal,
		Input:                cwd,
		KantraDir:            kantraDir,
		ExternalOnly:         true,
		EnableDefaultRulesets: false,
		Log:                  logr.Discard(),
	})

	err = env.Start(context.Background())
	require.Error(t, err, "Start() should fail when input is the current directory")
	assert.Contains(t, err.Error(), "cannot be the current directory")
}

func TestLocalEnvironment_Stop_CleansUpEclipseDirs(t *testing.T) {
	kantraDir := mkMinimalKantraDir(t)
	inputDir := t.TempDir()

	env := newLocalEnvironment(EnvironmentConfig{
		Mode:                 ModeLocal,
		Input:                inputDir,
		KantraDir:            kantraDir,
		ExternalOnly:         true,
		EnableDefaultRulesets: false,
		Log:                  logr.Discard(),
	})

	// Stop should not error even if no eclipse dirs exist
	err := env.Stop(context.Background())
	require.NoError(t, err)
}

func TestLocalEnvironment_Rules(t *testing.T) {
	tests := []struct {
		name           string
		prep           func(t *testing.T) (kantraDir string, cfg EnvironmentConfig)
		enableDefaults bool
		assert         func(t *testing.T, kantraDir string, rules []string, err error)
	}{
		{
			name: "external_only_enable_defaults_uses_rulesets_root",
			prep: func(t *testing.T) (string, EnvironmentConfig) {
				d := mkKantraDirWithRulesets(t)
				return d, EnvironmentConfig{
					KantraDir:    d,
					ExternalOnly: true,
					Log:          logr.Discard(),
				}
			},
			enableDefaults: true,
			assert: func(t *testing.T, kantraDir string, rules []string, err error) {
				require.NoError(t, err)
				require.Len(t, rules, 1)
				assert.Equal(t, filepath.Join(kantraDir, rulesetsSubdir), rules[0])
			},
		},
		{
			name: "external_only_enable_defaults_false",
			prep: func(t *testing.T) (string, EnvironmentConfig) {
				d := mkKantraDirWithRulesets(t)
				return d, EnvironmentConfig{
					KantraDir:    d,
					ExternalOnly: true,
					Log:          logr.Discard(),
				}
			},
			enableDefaults: false,
			assert: func(t *testing.T, _ string, rules []string, err error) {
				require.NoError(t, err)
				assert.Empty(t, rules)
			},
		},
		{
			name: "non_external_fallback_java_subdir",
			prep: func(t *testing.T) (string, EnvironmentConfig) {
				d := mkKantraDirWithRulesetSubdirs(t, util.DefaultRulesetDir[util.JavaProvider])
				return d, EnvironmentConfig{
					KantraDir:    d,
					ExternalOnly: false,
					Log:          logr.Discard(),
				}
			},
			enableDefaults: true,
			assert: func(t *testing.T, kantraDir string, rules []string, err error) {
				require.NoError(t, err)
				require.Len(t, rules, 1)
				want := filepath.Join(kantraDir, rulesetsSubdir, util.DefaultRulesetDir[util.JavaProvider])
				assert.Equal(t, want, rules[0])
			},
		},
		{
			name: "non_external_fallback_no_provider_subdir",
			prep: func(t *testing.T) (string, EnvironmentConfig) {
				// rulesets/ exists but no rulesets/java — bundled defaults resolve to nothing.
				d := mkKantraDirWithRulesets(t)
				return d, EnvironmentConfig{
					KantraDir:    d,
					ExternalOnly: false,
					Log:          logr.Discard(),
				}
			},
			enableDefaults: true,
			assert: func(t *testing.T, _ string, rules []string, err error) {
				require.NoError(t, err)
				assert.Empty(t, rules)
			},
		},
		{
			name: "non_external_explicit_providers",
			prep: func(t *testing.T) (string, EnvironmentConfig) {
				sub := BundledDefaultRulesetSubdir(util.NodeJSProvider)
				require.NotEmpty(t, sub)
				d := mkKantraDirWithRulesetSubdirs(t, sub)
				return d, EnvironmentConfig{
					KantraDir:    d,
					ExternalOnly: false,
					Providers: []ProviderInfo{
						{Name: util.NodeJSProvider, DefaultRulesetSubdir: sub},
					},
					Log: logr.Discard(),
				}
			},
			enableDefaults: true,
			assert: func(t *testing.T, kantraDir string, rules []string, err error) {
				require.NoError(t, err)
				require.Len(t, rules, 1)
				sub := BundledDefaultRulesetSubdir(util.NodeJSProvider)
				assert.Equal(t, filepath.Join(kantraDir, rulesetsSubdir, sub), rules[0])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kantraDir, cfg := tt.prep(t)
			env := newLocalEnvironment(cfg)
			rules, err := env.Rules(nil, tt.enableDefaults)
			tt.assert(t, kantraDir, rules, err)
		})
	}
}
