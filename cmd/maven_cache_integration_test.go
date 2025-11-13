package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-logr/logr"
)

// TestMavenCacheVolumeCreation tests that createMavenCacheVolume creates
// the volume successfully and sets up the host directory correctly.
func TestMavenCacheVolumeCreation(t *testing.T) {
	// Skip if container binary is not available
	binary := getContainerBinary()
	if binary == "" {
		t.Skip("Container runtime not available, skipping integration test")
	}

	// Initialize Settings for tests
	if Settings.ContainerBinary == "" {
		Settings.ContainerBinary = binary
	}

	ctx := &AnalyzeCommandContext{
		log: logr.Discard(),
	}

	// Clean up any existing test volume
	cleanupMavenCacheVolume(t)
	defer cleanupMavenCacheVolume(t)

	volName, err := ctx.createMavenCacheVolume()
	if err != nil {
		t.Fatalf("createMavenCacheVolume() failed: %v", err)
	}

	if volName != "maven-cache-volume" {
		t.Errorf("expected volume name 'maven-cache-volume', got '%s'", volName)
	}

	if ctx.mavenCacheVolumeName != "maven-cache-volume" {
		t.Errorf("expected mavenCacheVolumeName to be set to 'maven-cache-volume', got '%s'", ctx.mavenCacheVolumeName)
	}

	// Verify the volume was actually created
	if !volumeExists("maven-cache-volume") {
		t.Error("maven-cache-volume was not created")
	}

	// Verify host directory was created
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get user home directory: %v", err)
	}
	m2RepoPath := filepath.Join(homeDir, ".m2", "repository")
	if _, err := os.Stat(m2RepoPath); os.IsNotExist(err) {
		t.Errorf("expected ~/.m2/repository to be created, but it does not exist")
	}
}

// TestMavenCacheVolumeReuse tests that createMavenCacheVolume reuses
// an existing volume instead of failing when called multiple times.
func TestMavenCacheVolumeReuse(t *testing.T) {
	// Skip if container binary is not available
	binary := getContainerBinary()
	if binary == "" {
		t.Skip("Container runtime not available, skipping integration test")
	}

	// Initialize Settings for tests
	if Settings.ContainerBinary == "" {
		Settings.ContainerBinary = binary
	}

	ctx := &AnalyzeCommandContext{
		log: logr.Discard(),
	}

	// Clean up any existing test volume
	cleanupMavenCacheVolume(t)
	defer cleanupMavenCacheVolume(t)

	// First creation
	volName1, err := ctx.createMavenCacheVolume()
	if err != nil {
		t.Fatalf("first createMavenCacheVolume() failed: %v", err)
	}

	// Second creation (should reuse, not error)
	ctx2 := &AnalyzeCommandContext{
		log: logr.Discard(),
	}
	volName2, err := ctx2.createMavenCacheVolume()
	if err != nil {
		t.Fatalf("second createMavenCacheVolume() failed: %v", err)
	}

	if volName1 != volName2 {
		t.Errorf("expected same volume name on reuse, got '%s' and '%s'", volName1, volName2)
	}

	// Verify only one volume exists (not duplicates)
	if !volumeExists("maven-cache-volume") {
		t.Error("maven-cache-volume does not exist after reuse")
	}
}

// TestMavenCacheVolumeCleanup tests that the Maven cache volume
// is NOT removed during cleanup (intentional persistence).
func TestMavenCacheVolumeCleanup(t *testing.T) {
	// Skip if container binary is not available
	binary := getContainerBinary()
	if binary == "" {
		t.Skip("Container runtime not available, skipping integration test")
	}

	// Initialize Settings for tests
	if Settings.ContainerBinary == "" {
		Settings.ContainerBinary = binary
	}

	ctx := &AnalyzeCommandContext{
		log: logr.Discard(),
	}

	// Clean up any existing test volume
	cleanupMavenCacheVolume(t)
	defer cleanupMavenCacheVolume(t)

	// Create the volume
	volName, err := ctx.createMavenCacheVolume()
	if err != nil {
		t.Fatalf("createMavenCacheVolume() failed: %v", err)
	}

	if volName != "maven-cache-volume" {
		t.Fatalf("expected volume name 'maven-cache-volume', got '%s'", volName)
	}

	// Run cleanup
	if err := ctx.RmVolumes(context.Background()); err != nil {
		t.Fatalf("RmVolumes() failed: %v", err)
	}

	// Verify the Maven cache volume still exists (was NOT removed)
	if !volumeExists("maven-cache-volume") {
		t.Error("maven-cache-volume was incorrectly removed during cleanup - it should persist!")
	}
}

// TestMavenCacheVolumeHostPathMapping tests that the volume correctly
// maps to the host's ~/.m2/repository directory.
func TestMavenCacheVolumeHostPathMapping(t *testing.T) {
	// Skip if container binary is not available
	binary := getContainerBinary()
	if binary == "" {
		t.Skip("Container runtime not available, skipping integration test")
	}

	// Initialize Settings for tests
	if Settings.ContainerBinary == "" {
		Settings.ContainerBinary = binary
	}

	ctx := &AnalyzeCommandContext{
		log: logr.Discard(),
	}

	// Clean up any existing test volume
	cleanupMavenCacheVolume(t)
	defer cleanupMavenCacheVolume(t)

	// Create the volume
	_, err := ctx.createMavenCacheVolume()
	if err != nil {
		t.Fatalf("createMavenCacheVolume() failed: %v", err)
	}

	// Inspect the volume to verify the mount point
	cmd := exec.Command(Settings.ContainerBinary, "volume", "inspect", "maven-cache-volume", "--format", "{{.Mountpoint}}")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to inspect volume: %v", err)
	}

	mountPoint := strings.TrimSpace(string(output))
	if mountPoint == "" {
		t.Error("volume mount point is empty")
	}

	// Verify it points to a valid directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get user home directory: %v", err)
	}
	m2RepoPath := filepath.Join(homeDir, ".m2", "repository")

	// The mount point should exist and be accessible
	if _, err := os.Stat(m2RepoPath); os.IsNotExist(err) {
		t.Errorf("expected mount point to exist at %s, but it does not", m2RepoPath)
	}

	// For bind-mounted volumes, verify the volume options contain the correct device path
	// The Mountpoint field shows podman's internal storage, but we need to check the
	// bind mount source which is in the Options.Device field
	inspectCmd := exec.Command(Settings.ContainerBinary, "volume", "inspect", "maven-cache-volume", "--format", "{{.Options.device}}")
	deviceOutput, err := inspectCmd.Output()
	if err != nil {
		t.Fatalf("failed to inspect volume device: %v", err)
	}

	devicePath := strings.TrimSpace(string(deviceOutput))
	if devicePath == "" {
		t.Error("volume device path is empty")
	}

	// Verify the device path points to the .m2/repository directory
	if !strings.Contains(devicePath, ".m2"+string(filepath.Separator)+"repository") &&
		!strings.HasSuffix(devicePath, ".m2/repository") {
		t.Errorf("volume device path %q does not point to .m2/repository (expected %s)", devicePath, m2RepoPath)
	}
}

// TestMavenCacheVolumeConcurrentCreation tests that concurrent volume creation
// is handled correctly (idempotent operation).
func TestMavenCacheVolumeConcurrentCreation(t *testing.T) {
	// Skip if container binary is not available
	binary := getContainerBinary()
	if binary == "" {
		t.Skip("Container runtime not available, skipping integration test")
	}

	// Initialize Settings for tests
	if Settings.ContainerBinary == "" {
		Settings.ContainerBinary = binary
	}

	// Clean up any existing test volume
	cleanupMavenCacheVolume(t)
	defer cleanupMavenCacheVolume(t)

	// Create two contexts that will try to create the volume concurrently
	ctx1 := &AnalyzeCommandContext{
		log: logr.Discard(),
	}
	ctx2 := &AnalyzeCommandContext{
		log: logr.Discard(),
	}

	// Use a channel to coordinate concurrent execution
	type result struct {
		volName string
		err     error
	}
	results := make(chan result, 2)

	// Start both volume creations simultaneously
	go func() {
		volName, err := ctx1.createMavenCacheVolume()
		results <- result{volName, err}
	}()

	go func() {
		volName, err := ctx2.createMavenCacheVolume()
		results <- result{volName, err}
	}()

	// Collect both results
	res1 := <-results
	res2 := <-results

	// Both should succeed (one creates, one detects existing and reuses)
	if res1.err != nil {
		t.Errorf("first createMavenCacheVolume() failed: %v", res1.err)
	}
	if res2.err != nil {
		t.Errorf("second createMavenCacheVolume() failed: %v", res2.err)
	}

	// Both should return the same volume name
	if res1.volName != "maven-cache-volume" {
		t.Errorf("first call expected volume name 'maven-cache-volume', got '%s'", res1.volName)
	}
	if res2.volName != "maven-cache-volume" {
		t.Errorf("second call expected volume name 'maven-cache-volume', got '%s'", res2.volName)
	}

	// Verify only one volume exists (not duplicates)
	if !volumeExists("maven-cache-volume") {
		t.Error("maven-cache-volume does not exist after concurrent creation")
	}
}

// TestMavenCacheVolumeSkipped tests that Maven cache volume creation
// is skipped when KANTRA_SKIP_MAVEN_CACHE environment variable is set.
func TestMavenCacheVolumeSkipped(t *testing.T) {
	// Skip if container binary is not available
	binary := getContainerBinary()
	if binary == "" {
		t.Skip("Container runtime not available, skipping integration test")
	}

	// Initialize Settings for tests
	if Settings.ContainerBinary == "" {
		Settings.ContainerBinary = binary
	}

	// Set environment variable to skip Maven cache
	os.Setenv("KANTRA_SKIP_MAVEN_CACHE", "true")
	defer os.Unsetenv("KANTRA_SKIP_MAVEN_CACHE")

	ctx := &AnalyzeCommandContext{
		log: logr.Discard(),
	}

	// Attempt to create Maven cache volume
	volName, err := ctx.createMavenCacheVolume()
	if err != nil {
		t.Fatalf("createMavenCacheVolume() should not error when skipped: %v", err)
	}

	// Volume name should be empty when skipped
	if volName != "" {
		t.Errorf("expected empty volume name when KANTRA_SKIP_MAVEN_CACHE=true, got '%s'", volName)
	}

	// mavenCacheVolumeName should not be set
	if ctx.mavenCacheVolumeName != "" {
		t.Errorf("expected mavenCacheVolumeName to be empty when cache is skipped, got '%s'", ctx.mavenCacheVolumeName)
	}
}

// Helper function to get container binary (podman or docker)
func getContainerBinary() string {
	// Try Settings.ContainerBinary first (if initialized)
	if Settings.ContainerBinary != "" {
		return Settings.ContainerBinary
	}

	// Fall back to checking for podman or docker
	if _, err := exec.LookPath("podman"); err == nil {
		return "podman"
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker"
	}
	return ""
}

// Helper function to check if a container runtime is available
func isContainerRuntimeAvailable() bool {
	binary := getContainerBinary()
	if binary == "" {
		return false
	}
	cmd := exec.Command(binary, "version")
	return cmd.Run() == nil
}

// Helper function to check if a volume exists
func volumeExists(volumeName string) bool {
	binary := getContainerBinary()
	if binary == "" {
		return false
	}
	cmd := exec.Command(binary, "volume", "inspect", volumeName)
	return cmd.Run() == nil
}

// Helper function to clean up the maven cache volume for testing
func cleanupMavenCacheVolume(t *testing.T) {
	if volumeExists("maven-cache-volume") {
		binary := getContainerBinary()
		if binary == "" {
			return
		}
		cmd := exec.Command(binary, "volume", "rm", "maven-cache-volume")
		if err := cmd.Run(); err != nil {
			t.Logf("Warning: failed to cleanup maven-cache-volume: %v", err)
		}
	}
}
