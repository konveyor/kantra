package provider

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	analyzerprovider "github.com/konveyor/analyzer-lsp/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUtilMavenCacheDir_HubAligned(t *testing.T) {
	assert.Equal(t, "/cache/m2", util.MavenCacheDir)
	assert.False(t, strings.HasSuffix(util.MavenCacheDir, "/repository"),
		"maven repo root is /cache/m2, aligned with tackle2-hub")
}

func TestCreateMavenCacheVolume_DisabledByEnv(t *testing.T) {
	t.Setenv("KANTRA_SKIP_MAVEN_CACHE", "true")

	e := newContainerEnvironment(EnvironmentConfig{
		ContainerBinary: "/bin/false",
		Log:             logr.Discard(),
	})

	vol, err := e.createMavenCacheVolume()
	require.NoError(t, err)
	assert.Empty(t, vol)
	assert.Empty(t, e.mavenCacheVol)
}

func TestCreateMavenCacheVolume_BindMountsHostRepository(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bind-mount volume args differ on Windows")
	}

	t.Setenv("KANTRA_SKIP_MAVEN_CACHE", "false")

	tmp := t.TempDir()
	argsFile := filepath.Join(tmp, "args.txt")
	fakeBin := filepath.Join(tmp, "container")
	script := "#!/bin/sh\nprintf '%s' \"$*\" > \"" + argsFile + "\"\n"
	require.NoError(t, os.WriteFile(fakeBin, []byte(script), 0755))

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	e := newContainerEnvironment(EnvironmentConfig{
		ContainerBinary: fakeBin,
		Log:             logr.Discard(),
	})

	vol, err := e.createMavenCacheVolume()
	require.NoError(t, err)
	assert.Equal(t, "maven-cache-volume", vol)
	assert.Equal(t, "maven-cache-volume", e.mavenCacheVol)

	args, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	argsStr := string(args)
	assert.Contains(t, argsStr, "volume create")
	assert.Contains(t, argsStr, "type=none")
	assert.Contains(t, argsStr, "o=bind")
	assert.Contains(t, argsStr, filepath.Join(homeDir, ".m2", "repository"))
	assert.Contains(t, argsStr, "maven-cache-volume")

	repoPath := filepath.Join(homeDir, ".m2", "repository")
	_, err = os.Stat(repoPath)
	require.NoError(t, err, "expected host repository directory to be created")
}

func TestHybridMavenCacheProviderConfig(t *testing.T) {
	inputDir := t.TempDir()
	addresses := map[string]string{util.JavaProvider: "localhost:8001"}

	tests := []struct {
		name             string
		mavenCacheDir    string
		expectMavenCache bool
	}{
		{
			name:             "omits mavenCacheDir when cache volume not mounted",
			mavenCacheDir:    "",
			expectMavenCache: false,
		},
		{
			name:             "sets hub-aligned mavenCacheDir when cache volume mounted",
			mavenCacheDir:    util.MavenCacheDir,
			expectMavenCache: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs := DefaultProviderConfig(ModeNetwork, DefaultOptions{
				Providers:         []string{util.JavaProvider, "builtin"},
				Location:          util.SourceMountPath,
				LocalLocation:     inputDir,
				AnalysisMode:      "full",
				ProviderAddresses: addresses,
				MavenCacheDir:     tt.mavenCacheDir,
			})

			javaCfg := findProviderConfig(t, configs, util.JavaProvider)
			psc := javaCfg.InitConfig[0].ProviderSpecificConfig

			cacheDir, ok := psc["mavenCacheDir"]
			if !tt.expectMavenCache {
				assert.False(t, ok)
				return
			}

			require.True(t, ok)
			assert.Equal(t, "/cache/m2", cacheDir)
		})
	}
}

func findProviderConfig(t *testing.T, configs []analyzerprovider.Config, name string) analyzerprovider.Config {
	t.Helper()
	for _, cfg := range configs {
		if cfg.Name == name {
			return cfg
		}
	}
	t.Fatalf("provider config %q not found", name)
	return analyzerprovider.Config{}
}
