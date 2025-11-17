package provider

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to get a test logger
func getTestLogger() logr.Logger {
	return logr.Discard()
}

func TestJavaProvider_GetConfigVolume(t *testing.T) {
	// Setup a temporary directory
	tmpDir := t.TempDir()

	tests := []struct {
		name                   string
		configInput            ConfigInput
		expectedProviderName   string
		expectedMode           string
		expectedAddress        string
		expectedBundleLocation string
		expectMavenSettings    bool
		expectJvmMaxMem        bool
		expectError            bool
	}{
		{
			name: "basic configuration with directory input",
			configInput: ConfigInput{
				Name:               "java",
				IsFileInput:        false,
				InputPath:          "/tmp/project",
				OutputPath:         "/tmp/output",
				Port:               12345,
				Mode:               "source-only",
				TmpDir:             tmpDir,
				JavaBundleLocation: "/bundles/java",
				DisableMavenSearch: false,
			},
			expectedProviderName:   "java",
			expectedMode:           "source-only",
			expectedAddress:        "0.0.0.0:12345",
			expectedBundleLocation: "/bundles/java",
			expectError:            false,
		},
		{
			name: "configuration with file input",
			configInput: ConfigInput{
				Name:               "java",
				IsFileInput:        true,
				InputPath:          "/tmp/project/app.jar",
				OutputPath:         "/tmp/output",
				Port:               54321,
				Mode:               "full",
				TmpDir:             tmpDir,
				JavaBundleLocation: "/bundles/java",
				DisableMavenSearch: true,
				Log:                getTestLogger(),
			},
			expectedProviderName:   "java",
			expectedMode:           "full",
			expectedAddress:        "0.0.0.0:54321",
			expectedBundleLocation: "/bundles/java",
			expectError:            false,
		},
		{
			name: "configuration with maven settings file",
			configInput: ConfigInput{
				Name:               "java",
				IsFileInput:        false,
				InputPath:          "/tmp/project",
				OutputPath:         "/tmp/output",
				Port:               12345,
				Mode:               "source-only",
				TmpDir:             tmpDir,
				MavenSettingsFile:  createTempMavenSettings(t, tmpDir),
				JavaBundleLocation: "/bundles/java",
				DisableMavenSearch: false,
				Log:                getTestLogger(),
			},
			expectedProviderName:   "java",
			expectedMode:           "source-only",
			expectedAddress:        "0.0.0.0:12345",
			expectedBundleLocation: "/bundles/java",
			expectMavenSettings:    true,
			expectError:            false,
		},
		{
			name: "configuration with JVM max memory",
			configInput: ConfigInput{
				Name:               "java",
				IsFileInput:        false,
				InputPath:          "/tmp/project",
				OutputPath:         "/tmp/output",
				Port:               12345,
				Mode:               "source-only",
				TmpDir:             tmpDir,
				JvmMaxMem:          "4096m",
				JavaBundleLocation: "/bundles/java",
				DisableMavenSearch: false,
			},
			expectedProviderName:   "java",
			expectedMode:           "source-only",
			expectedAddress:        "0.0.0.0:12345",
			expectedBundleLocation: "/bundles/java",
			expectJvmMaxMem:        true,
			expectError:            false,
		},
		{
			name: "configuration with all optional parameters",
			configInput: ConfigInput{
				Name:               "java",
				IsFileInput:        false,
				InputPath:          "/tmp/project",
				OutputPath:         "/tmp/output",
				Port:               12345,
				Mode:               "full",
				TmpDir:             tmpDir,
				MavenSettingsFile:  createTempMavenSettings(t, tmpDir),
				JvmMaxMem:          "2048m",
				JavaBundleLocation: "/bundles/java",
				DisableMavenSearch: true,
				Log:                getTestLogger(),
			},
			expectedProviderName:   "java",
			expectedMode:           "full",
			expectedAddress:        "0.0.0.0:12345",
			expectedBundleLocation: "/bundles/java",
			expectMavenSettings:    true,
			expectJvmMaxMem:        true,
			expectError:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &JavaProvider{}

			config, err := p.GetConfigVolume(tt.configInput)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedProviderName, config.Name)
			assert.Equal(t, tt.expectedAddress, config.Address)

			require.Len(t, config.InitConfig, 1)
			initConfig := config.InitConfig[0]
			assert.Equal(t, tt.expectedMode, string(initConfig.AnalysisMode))
			assert.Equal(t, tt.expectedBundleLocation, initConfig.ProviderSpecificConfig["bundles"])
			assert.Equal(t, "/usr/local/etc", initConfig.ProviderSpecificConfig["mavenIndexPath"])
			assert.Equal(t, "/usr/local/etc/maven.default.index", initConfig.ProviderSpecificConfig["depOpenSourceLabelsFile"])
			assert.Equal(t, "/jdtls/bin/jdtls", initConfig.ProviderSpecificConfig["lspServerPath"])
			assert.Equal(t, tt.configInput.DisableMavenSearch, initConfig.ProviderSpecificConfig["disableMavenSearch"])

			if tt.expectMavenSettings {
				assert.Contains(t, initConfig.ProviderSpecificConfig, "mavenSettingsFile")
				assert.Contains(t, initConfig.ProviderSpecificConfig["mavenSettingsFile"], "settings.xml")
			}

			if tt.expectJvmMaxMem {
				assert.Equal(t, tt.configInput.JvmMaxMem, initConfig.ProviderSpecificConfig["jvmMaxMem"])
			}

			if !tt.configInput.IsFileInput {
				// Check mount path for directory input
				assert.Contains(t, initConfig.Location, "source")
			} else {
				// Check mount path for file input
				assert.Contains(t, initConfig.Location, filepath.Base(tt.configInput.InputPath))
			}
		})
	}
}

func TestJavaProvider_GetConfigVolumeWithNonExistentMavenSettings(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistentMavenSettings := filepath.Join(tmpDir, "nonexistent-settings.xml")

	configInput := ConfigInput{
		Name:               "java",
		IsFileInput:        false,
		InputPath:          "/tmp/project",
		OutputPath:         "/tmp/output",
		Port:               12345,
		Mode:               "source-only",
		TmpDir:             tmpDir,
		MavenSettingsFile:  nonExistentMavenSettings,
		JavaBundleLocation: "/bundles/java",
		DisableMavenSearch: false,
		Log:                getTestLogger(),
	}

	p := &JavaProvider{}
	// CopyFileContents returns nil for non-existent files, so the check passes
	// and the maven settings file is still added to config
	config, err := p.GetConfigVolume(configInput)
	require.NoError(t, err)
	// Even though the file doesn't exist, CopyFileContents returns nil
	// so the config still includes mavenSettingsFile
	assert.Contains(t, config.InitConfig[0].ProviderSpecificConfig, "mavenSettingsFile")
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
	assert.NotEmpty(t, targetPaths)
}

func TestWalkJavaPathForTargetWithNonexistentDir(t *testing.T) {
	logger := getTestLogger()
	_, err := WalkJavaPathForTarget(logger, false, "/nonexistent/path")
	assert.Error(t, err)
}

func TestGetJavaBinaryProjectDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure
	baseDir := filepath.Join(tmpDir, "base")
	err := os.MkdirAll(baseDir, 0755)
	require.NoError(t, err)

	// Create directory with "java-project-" pattern
	projectDir := filepath.Join(baseDir, "java-project-abc123")
	err = os.MkdirAll(projectDir, 0755)
	require.NoError(t, err)

	// Test successful case
	foundDir, err := GetJavaBinaryProjectDir(baseDir)
	require.NoError(t, err)
	assert.Equal(t, projectDir, foundDir)

	// Test case where directory doesn't exist
	_, err = GetJavaBinaryProjectDir("/nonexistent/path")
	assert.Error(t, err)

	// Test case where no matching directory exists
	emptyDir := filepath.Join(tmpDir, "empty")
	err = os.MkdirAll(emptyDir, 0755)
	require.NoError(t, err)

	foundDir, err = GetJavaBinaryProjectDir(emptyDir)
	require.NoError(t, err)
	assert.Empty(t, foundDir)
}

func TestWaitForTargetDir_ExistingTarget(t *testing.T) {
	tmpDir := t.TempDir()
	logger := getTestLogger()

	// Create target directory immediately
	targetDir := filepath.Join(tmpDir, "target")
	err := os.MkdirAll(targetDir, 0755)
	require.NoError(t, err)

	// Should return immediately since target already exists
	err = WaitForTargetDir(logger, tmpDir, 5*time.Second)
	require.NoError(t, err)
}

func TestWaitForTargetDir_Timeout(t *testing.T) {
	tmpDir := t.TempDir()
	logger := getTestLogger()

	// Don't create target directory
	// Should timeout waiting for it
	err := WaitForTargetDir(logger, tmpDir, 100*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestWaitForTargetDir_CreateTargetDuringWait(t *testing.T) {
	tmpDir := t.TempDir()
	logger := getTestLogger()

	// Start a goroutine to create the target directory after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		targetDir := filepath.Join(tmpDir, "target")
		err := os.MkdirAll(targetDir, 0755)
		require.NoError(t, err)
	}()

	// Should successfully detect the target directory once it's created
	err := WaitForTargetDir(logger, tmpDir, 5*time.Second)
	require.NoError(t, err)
}

func TestWaitForTargetDir_WithNonexistentPath(t *testing.T) {
	logger := getTestLogger()
	err := WaitForTargetDir(logger, "/nonexistent/path", 100*time.Millisecond)
	// The initial Stat will fail, but it depends on the implementation
	// This test is mostly to ensure we don't panic
	t.Logf("Error for nonexistent path: %v", err)
}

func TestGetConfigVolume_FileInputUsesCorrectMountPath(t *testing.T) {
	tmpDir := t.TempDir()

	p := &JavaProvider{}
	configInput := ConfigInput{
		Name:               "java",
		IsFileInput:        true,
		InputPath:          "/tmp/project/myapp.jar",
		OutputPath:         "/tmp/output",
		Port:               12345,
		Mode:               "full",
		TmpDir:             tmpDir,
		JavaBundleLocation: "/bundles/java",
		DisableMavenSearch: false,
	}

	config, err := p.GetConfigVolume(configInput)
	require.NoError(t, err)

	// For file input, the mount path should include the filename
	assert.Contains(t, config.InitConfig[0].Location, "myapp.jar")
}

func TestGetConfigVolume_DirectoryInputUsesSourceMount(t *testing.T) {
	tmpDir := t.TempDir()

	p := &JavaProvider{}
	configInput := ConfigInput{
		Name:               "java",
		IsFileInput:        false,
		InputPath:          "/tmp/project",
		OutputPath:         "/tmp/output",
		Port:               12345,
		Mode:               "source-only",
		TmpDir:             tmpDir,
		JavaBundleLocation: "/bundles/java",
		DisableMavenSearch: false,
	}

	config, err := p.GetConfigVolume(configInput)
	require.NoError(t, err)

	// For directory input, should use standard source mount path
	assert.NotContains(t, config.InitConfig[0].Location, filepath.Base(configInput.InputPath))
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

func TestGetJavaBinaryProjectDir_MultipleProjects(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple directories, only one with "java-project-" prefix
	err := os.MkdirAll(filepath.Join(tmpDir, "other-project"), 0755)
	require.NoError(t, err)

	javaProjectDir := filepath.Join(tmpDir, "java-project-12345")
	err = os.MkdirAll(javaProjectDir, 0755)
	require.NoError(t, err)

	foundDir, err := GetJavaBinaryProjectDir(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, javaProjectDir, foundDir)
}

func TestWaitForTargetDir_InvalidPath(t *testing.T) {
	logger := getTestLogger()

	// Test with an invalid path (file instead of directory)
	tmpFile := filepath.Join(t.TempDir(), "notadir")
	err := os.WriteFile(tmpFile, []byte("content"), 0644)
	require.NoError(t, err)

	err = WaitForTargetDir(logger, tmpFile, 100*time.Millisecond)
	// This should either error or timeout
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

// Test the provider-specific config keys are correctly set
func TestGetConfigVolume_ProviderSpecificConfig(t *testing.T) {
	tmpDir := t.TempDir()

	p := &JavaProvider{}
	configInput := ConfigInput{
		Name:               "java",
		IsFileInput:        false,
		InputPath:          "/tmp/project",
		OutputPath:         "/tmp/output",
		Port:               12345,
		Mode:               "full",
		TmpDir:             tmpDir,
		JavaBundleLocation: "/custom/bundles",
		DisableMavenSearch: true,
	}

	config, err := p.GetConfigVolume(configInput)
	require.NoError(t, err)

	initConfig := config.InitConfig[0]
	require.NotNil(t, initConfig.ProviderSpecificConfig)

	// Verify all expected config keys are present
	assert.Equal(t, "java", initConfig.ProviderSpecificConfig["lspServerName"])
	assert.Equal(t, "/custom/bundles", initConfig.ProviderSpecificConfig["bundles"])
	assert.Equal(t, "/usr/local/etc", initConfig.ProviderSpecificConfig["mavenIndexPath"])
	assert.Equal(t, "/usr/local/etc/maven.default.index", initConfig.ProviderSpecificConfig["depOpenSourceLabelsFile"])
	assert.Equal(t, "/jdtls/bin/jdtls", initConfig.ProviderSpecificConfig["lspServerPath"])
	assert.Equal(t, true, initConfig.ProviderSpecificConfig["disableMavenSearch"])
}
