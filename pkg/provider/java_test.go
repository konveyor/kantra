package provider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to get a test logger
func getTestLogger() logr.Logger {
	return logr.Discard()
}

func TestJavaProvider_GetConfig_Container(t *testing.T) {
	tests := []struct {
		name                string
		opts                BaseOptions
		mavenSettingsFile   string
		jvmMaxMem           string
		disableMavenSearch  bool
		expectedMode        string
		expectedAddress     string
		expectMavenSettings bool
		expectJvmMaxMem     bool
	}{
		{
			name: "basic configuration with address",
			opts: BaseOptions{
				Location:     "/opt/input/source",
				AnalysisMode: "source-only",
				Address:      "0.0.0.0:12345",
			},
			expectedMode:    "source-only",
			expectedAddress: "0.0.0.0:12345",
		},
		{
			name: "full analysis mode",
			opts: BaseOptions{
				Location:     "/opt/input/source/app.jar",
				AnalysisMode: "full",
				Address:      "0.0.0.0:54321",
			},
			disableMavenSearch: true,
			expectedMode:       "full",
			expectedAddress:    "0.0.0.0:54321",
		},
		{
			name: "with maven settings",
			opts: BaseOptions{
				Location:     "/opt/input/source",
				AnalysisMode: "source-only",
				Address:      "0.0.0.0:12345",
			},
			mavenSettingsFile:   "/opt/input/config/settings.xml",
			expectedMode:        "source-only",
			expectedAddress:     "0.0.0.0:12345",
			expectMavenSettings: true,
		},
		{
			name: "with JVM max memory",
			opts: BaseOptions{
				Location:     "/opt/input/source",
				AnalysisMode: "source-only",
				Address:      "0.0.0.0:12345",
			},
			jvmMaxMem:       "4096m",
			expectedMode:    "source-only",
			expectedAddress: "0.0.0.0:12345",
			expectJvmMaxMem: true,
		},
		{
			name: "with all optional parameters",
			opts: BaseOptions{
				Location:     "/opt/input/source",
				AnalysisMode: "full",
				Address:      "0.0.0.0:12345",
			},
			mavenSettingsFile:   "/opt/input/config/settings.xml",
			jvmMaxMem:           "2048m",
			disableMavenSearch:  true,
			expectedMode:        "full",
			expectedAddress:     "0.0.0.0:12345",
			expectMavenSettings: true,
			expectJvmMaxMem:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &JavaProvider{}

			config, err := p.GetConfig(ModeContainer, tt.opts, JavaOptions{
				MavenSettingsFile:  tt.mavenSettingsFile,
				JvmMaxMem:          tt.jvmMaxMem,
				DisableMavenSearch: tt.disableMavenSearch,
			})
			require.NoError(t, err)

			assert.Equal(t, "java", config.Name)
			assert.Equal(t, tt.expectedAddress, config.Address)

			require.Len(t, config.InitConfig, 1)
			initConfig := config.InitConfig[0]
			assert.Equal(t, tt.expectedMode, string(initConfig.AnalysisMode))

			// Container mode uses canonical paths
			assert.Equal(t, ContainerJavaBundlePath, initConfig.ProviderSpecificConfig["bundles"])
			assert.Equal(t, ContainerDepOpenSourceLabels, initConfig.ProviderSpecificConfig["depOpenSourceLabelsFile"])
			assert.Equal(t, ContainerJDTLSPath, initConfig.ProviderSpecificConfig[provider.LspServerPathConfigKey])

			if tt.expectMavenSettings {
				assert.Equal(t, tt.mavenSettingsFile, initConfig.ProviderSpecificConfig["mavenSettingsFile"])
			}

			if tt.expectJvmMaxMem {
				assert.Equal(t, tt.jvmMaxMem, initConfig.ProviderSpecificConfig["jvmMaxMem"])
			}
		})
	}
}

func TestJavaProvider_GetConfig_ContainerBinaryPath(t *testing.T) {
	// When no address is provided, container mode should set binary path
	p := &JavaProvider{}
	config, err := p.GetConfig(ModeContainer, BaseOptions{
		Location:     "/opt/input/source",
		AnalysisMode: "source-only",
	})
	require.NoError(t, err)

	assert.Equal(t, ContainerJavaProviderBin, config.BinaryPath)
	assert.Empty(t, config.Address)
}

func TestJavaProvider_GetConfig_Local(t *testing.T) {
	kantraDir := "/home/user/.kantra"
	p := &JavaProvider{}

	config, err := p.GetConfig(ModeLocal, BaseOptions{
		Location:     "/home/user/project",
		AnalysisMode: "source-only",
		KantraDir:    kantraDir,
	}, JavaOptions{DisableMavenSearch: true})
	require.NoError(t, err)

	assert.Equal(t, kantraDir+"/java-external-provider", config.BinaryPath)
	assert.Equal(t, "/home/user/project", config.InitConfig[0].Location)

	psc := config.InitConfig[0].ProviderSpecificConfig
	assert.Equal(t, kantraDir+ContainerJavaBundlePath, psc["bundles"])
	assert.Equal(t, kantraDir+ContainerJDTLSPath, psc[provider.LspServerPathConfigKey])
	assert.Equal(t, true, psc["disableMavenSearch"])
	assert.Equal(t, true, psc["cleanExplodedBin"])
}

func TestJavaProvider_GetConfig_Network(t *testing.T) {
	p := &JavaProvider{}

	config, err := p.GetConfig(ModeNetwork, BaseOptions{
		Location: "/opt/input/source",
		Address:  "localhost:12345",
	}, JavaOptions{
		MavenSettingsFile: "/opt/input/config/settings.xml",
		JvmMaxMem:         "2048m",
	})
	require.NoError(t, err)

	assert.Equal(t, "localhost:12345", config.Address)
	assert.Empty(t, config.BinaryPath)

	psc := config.InitConfig[0].ProviderSpecificConfig
	assert.Equal(t, ContainerJDTLSPath, psc[provider.LspServerPathConfigKey])
	assert.Equal(t, ContainerJavaBundlePath, psc["bundles"])
	assert.Equal(t, ContainerMavenIndexPath, psc["mavenIndexPath"])
	assert.Equal(t, "/opt/input/config/settings.xml", psc["mavenSettingsFile"])
	assert.Equal(t, "2048m", psc["jvmMaxMem"])
}

func TestJavaProvider_GetConfig_FileInputLocation(t *testing.T) {
	p := &JavaProvider{}
	config, err := p.GetConfig(ModeContainer, BaseOptions{
		Location:     "/opt/input/source/myapp.jar",
		AnalysisMode: "full",
		Address:      "0.0.0.0:12345",
	})
	require.NoError(t, err)

	// Location should be exactly what was passed
	assert.Equal(t, "/opt/input/source/myapp.jar", config.InitConfig[0].Location)
}

func TestJavaProvider_GetConfig_DirectoryInputLocation(t *testing.T) {
	p := &JavaProvider{}
	config, err := p.GetConfig(ModeContainer, BaseOptions{
		Location:     "/opt/input/source",
		AnalysisMode: "source-only",
		Address:      "0.0.0.0:12345",
	})
	require.NoError(t, err)

	assert.Equal(t, "/opt/input/source", config.InitConfig[0].Location)
}

func TestWalkJavaPathForTarget(t *testing.T) {
	tmpDir := t.TempDir()
	logger := getTestLogger()

	// Create a proper directory structure
	testProjectDir := filepath.Join(tmpDir, "project")
	err := os.MkdirAll(filepath.Join(testProjectDir, "src", "main", "java"), 0755)
	require.NoError(t, err)

	// Create target directory
	targetDir := filepath.Join(testProjectDir, "target")
	err = os.MkdirAll(targetDir, 0755)
	require.NoError(t, err)

	// Create nested target directories
	nestedTargetDir := filepath.Join(testProjectDir, "module1", "target")
	err = os.MkdirAll(nestedTargetDir, 0755)
	require.NoError(t, err)

	// Create a separate directory without target folders
	cleanDir := filepath.Join(tmpDir, "clean")
	err = os.MkdirAll(filepath.Join(cleanDir, "src", "main", "java"), 0755)
	require.NoError(t, err)

	tests := []struct {
		name         string
		isFileInput  bool
		root         string
		expectTarget bool
		expectError  bool
	}{
		{
			name:         "directory input with target directories",
			isFileInput:  false,
			root:         testProjectDir,
			expectTarget: true,
			expectError:  false,
		},
		{
			name:         "directory input without target",
			isFileInput:  false,
			root:         cleanDir,
			expectTarget: false,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetPaths, err := WalkJavaPathForTarget(logger, tt.isFileInput, tt.root)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.expectTarget {
				assert.NotEmpty(t, targetPaths)
			} else {
				assert.Empty(t, targetPaths)
			}
		})
	}
}

func TestWalkJavaPathForTargetWithFileInput(t *testing.T) {
	tmpDir := t.TempDir()
	logger := getTestLogger()

	// Create directory structure for binary input
	binDir := filepath.Join(tmpDir, "bin")
	err := os.MkdirAll(binDir, 0755)
	require.NoError(t, err)

	// Create a directory that looks like "java-project-XXX"
	projectDir := filepath.Join(binDir, "java-project-12345")
	err = os.MkdirAll(projectDir, 0755)
	require.NoError(t, err)

	// Create target directory
	targetDir := filepath.Join(projectDir, "target")
	err = os.MkdirAll(targetDir, 0755)
	require.NoError(t, err)

	// Create a fake binary file
	binaryFile := filepath.Join(binDir, "app.jar")
	err = os.WriteFile(binaryFile, []byte("fake binary"), 0644)
	require.NoError(t, err)

	targetPaths, err := WalkJavaPathForTarget(logger, true, binaryFile)
	require.NoError(t, err)
	assert.Empty(t, targetPaths)
}

func TestWalkJavaPathForTargetWithNonexistentDir(t *testing.T) {
	logger := getTestLogger()
	_, err := WalkJavaPathForTarget(logger, false, "/nonexistent/path")
	assert.Error(t, err)
}

// Additional test for edge case: empty root directory
func TestWalkJavaPathForTarget_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	logger := getTestLogger()

	targetPaths, err := WalkJavaPathForTarget(logger, false, tmpDir)
	require.NoError(t, err)
	assert.Empty(t, targetPaths)
}

// Test for file input when JavaBinaryProjectDir returns empty
func TestWalkJavaPathForTarget_FileInputNoProjectDir(t *testing.T) {
	tmpDir := t.TempDir()
	logger := getTestLogger()

	// Create a binary file but no java-project- directory
	binaryFile := filepath.Join(tmpDir, "app.jar")
	err := os.WriteFile(binaryFile, []byte("fake"), 0644)
	require.NoError(t, err)

	_, err = WalkJavaPathForTarget(logger, true, binaryFile)
	// Should handle the case where GetJavaBinaryProjectDir returns empty string
	// This depends on implementation - might error or return empty
	t.Logf("Error for file input with no project dir: %v", err)
}

// Test the provider-specific config keys are correctly set for container mode
func TestJavaProvider_GetConfig_ProviderSpecificConfig(t *testing.T) {
	p := &JavaProvider{}

	config, err := p.GetConfig(ModeContainer, BaseOptions{
		Location:     "/opt/input/source",
		AnalysisMode: "full",
	}, JavaOptions{DisableMavenSearch: true})
	require.NoError(t, err)

	initConfig := config.InitConfig[0]
	require.NotNil(t, initConfig.ProviderSpecificConfig)

	// Verify all expected config keys are present
	assert.Equal(t, "java", initConfig.ProviderSpecificConfig["lspServerName"])
	assert.Equal(t, ContainerJavaBundlePath, initConfig.ProviderSpecificConfig["bundles"])
	assert.Equal(t, ContainerDepOpenSourceLabels, initConfig.ProviderSpecificConfig["depOpenSourceLabelsFile"])
	assert.Equal(t, ContainerJDTLSPath, initConfig.ProviderSpecificConfig[provider.LspServerPathConfigKey])
}

// Helper function to create a temporary maven settings file
func createTempMavenSettings(t *testing.T, tmpDir string) string {
	settingsFile := filepath.Join(tmpDir, "settings.xml")
	err := os.WriteFile(settingsFile, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<settings>
  <profiles>
    <profile>
      <id>test-profile</id>
    </profile>
  </profiles>
</settings>`), 0644)
	require.NoError(t, err)
	return settingsFile
}
