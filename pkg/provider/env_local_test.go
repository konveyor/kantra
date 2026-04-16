package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
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

func TestCleanLocalJavaBinaryProjectDirs(t *testing.T) {
	logger := getTestLogger()

	t.Run("removes java-project and java-project-suffix beside binary", func(t *testing.T) {
		tmp := t.TempDir()
		bin := filepath.Join(tmp, "app.war")
		require.NoError(t, os.WriteFile(bin, []byte("x"), 0644))
		legacy := filepath.Join(tmp, "java-project")
		require.NoError(t, os.MkdirAll(filepath.Join(legacy, "src"), 0755))
		keep := filepath.Join(tmp, "other-dir")
		require.NoError(t, os.MkdirAll(keep, 0755))

		cleanLocalJavaBinaryProjectDirs(logger, bin, true)

		_, err := os.Stat(legacy)
		require.True(t, os.IsNotExist(err))
		_, err = os.Stat(keep)
		require.NoError(t, err)
	})

	t.Run("no-op when not file input", func(t *testing.T) {
		tmp := t.TempDir()
		proj := filepath.Join(tmp, "java-project")
		require.NoError(t, os.MkdirAll(proj, 0755))
		cleanLocalJavaBinaryProjectDirs(logger, tmp, false)
		_, err := os.Stat(proj)
		require.NoError(t, err)
	})

	t.Run("removes java-project for any file path when isFileInput true", func(t *testing.T) {
		tmp := t.TempDir()
		f := filepath.Join(tmp, "readme.txt")
		require.NoError(t, os.WriteFile(f, []byte("x"), 0644))
		proj := filepath.Join(tmp, "java-project")
		require.NoError(t, os.MkdirAll(proj, 0755))
		cleanLocalJavaBinaryProjectDirs(logger, f, true)
		_, err := os.Stat(proj)
		require.True(t, os.IsNotExist(err))
	})
}
