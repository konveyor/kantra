package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVolumeInspect_DevicePath(t *testing.T) {
	// Create a real directory to pass os.Lstat validation
	tmpDir := t.TempDir()

	volume := []map[string]any{{
		"Name": "test-volume",
		"Options": map[string]any{
			"type":   "none",
			"device": tmpDir,
			"o":      "bind",
		},
		"Mountpoint": "/var/lib/containers/storage/volumes/test-volume/_data",
	}}

	data, err := json.Marshal(volume)
	require.NoError(t, err)

	result, ok := parseVolumeInspect(data)
	assert.True(t, ok)
	assert.Equal(t, tmpDir, result)
}

func TestParseVolumeInspect_MountpointFallback(t *testing.T) {
	// Create a real directory to pass os.Lstat validation
	tmpDir := t.TempDir()

	// No Options.device, so it should fall back to Mountpoint
	volume := []map[string]any{{
		"Name":       "test-volume",
		"Mountpoint": tmpDir,
	}}

	data, err := json.Marshal(volume)
	require.NoError(t, err)

	result, ok := parseVolumeInspect(data)
	assert.True(t, ok)
	assert.Equal(t, tmpDir, result)
}

func TestParseVolumeInspect_DevicePreferredOverMountpoint(t *testing.T) {
	// Both exist — device should win
	deviceDir := t.TempDir()
	mountDir := t.TempDir()

	volume := []map[string]any{{
		"Options": map[string]any{
			"device": deviceDir,
		},
		"Mountpoint": mountDir,
	}}

	data, err := json.Marshal(volume)
	require.NoError(t, err)

	result, ok := parseVolumeInspect(data)
	assert.True(t, ok)
	assert.Equal(t, deviceDir, result)
}

func TestParseVolumeInspect_InvalidDevice_FallsBackToMountpoint(t *testing.T) {
	mountDir := t.TempDir()

	volume := []map[string]any{{
		"Options": map[string]any{
			"device": "/nonexistent/path/that/does/not/exist",
		},
		"Mountpoint": mountDir,
	}}

	data, err := json.Marshal(volume)
	require.NoError(t, err)

	result, ok := parseVolumeInspect(data)
	assert.True(t, ok)
	assert.Equal(t, mountDir, result)
}

func TestParseVolumeInspect_NeitherValid(t *testing.T) {
	volume := []map[string]any{{
		"Options": map[string]any{
			"device": "/nonexistent/a",
		},
		"Mountpoint": "/nonexistent/b",
	}}

	data, err := json.Marshal(volume)
	require.NoError(t, err)

	_, ok := parseVolumeInspect(data)
	assert.False(t, ok)
}

func TestParseVolumeInspect_InvalidJSON(t *testing.T) {
	_, ok := parseVolumeInspect([]byte("not json"))
	assert.False(t, ok)
}

func TestParseVolumeInspect_EmptyArray(t *testing.T) {
	_, ok := parseVolumeInspect([]byte("[]"))
	assert.False(t, ok)
}

func TestParseVolumeInspect_MultipleVolumes(t *testing.T) {
	// Only single-volume inspect results are valid
	data, _ := json.Marshal([]map[string]any{{}, {}})
	_, ok := parseVolumeInspect(data)
	assert.False(t, ok)
}

func TestBuildPathMappings(t *testing.T) {
	mappings := BuildPathMappings("/opt/input", "/home/user/project")

	require.Len(t, mappings, 1)
	assert.Equal(t, "/opt/input", mappings[0].From)
	assert.Equal(t, "/home/user/project", mappings[0].To)
}

func TestNormalizeVolumePath_UnixPassthrough(t *testing.T) {
	// On Unix, normalizeVolumePath should be a no-op
	paths := []string{
		"/home/user/project",
		"/tmp/kantra-binary-123",
		"/var/lib/containers/storage",
	}
	for _, p := range paths {
		assert.Equal(t, p, normalizeVolumePath(p))
	}
}

func TestResolveVolumeHostPath_FallbackOnError(t *testing.T) {
	// When the container binary doesn't exist, should return fallback
	fallback := t.TempDir()

	result := ResolveVolumeHostPath(
		t.Context(),
		logr.Discard(),
		"/nonexistent/container-binary",
		"nonexistent-volume",
		fallback,
	)

	assert.Equal(t, fallback, result)
}

func TestResolveVolumeHostPath_Integration(t *testing.T) {
	// This test validates that ResolveVolumeHostPath returns the fallback
	// when the container binary is unavailable (which is the case in most
	// unit test environments).
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "test-input")
	require.NoError(t, os.MkdirAll(subDir, 0755))

	// Create a fake "container" binary that always fails
	fakeBinary := filepath.Join(tmpDir, "fake-container")
	require.NoError(t, os.WriteFile(fakeBinary, []byte("#!/bin/sh\nexit 1\n"), 0755))

	result := ResolveVolumeHostPath(
		t.Context(),
		logr.Discard(),
		fakeBinary,
		"test-volume",
		subDir,
	)

	assert.Equal(t, subDir, result)
}
