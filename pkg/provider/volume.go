package provider

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"

	"github.com/go-logr/logr"
	analyzerprovider "github.com/konveyor/analyzer-lsp/provider"
)

// ResolveVolumeHostPath determines the host filesystem path backing a
// container volume. This is needed when providers run in containers and
// return paths relative to the container filesystem, but the builtin
// provider runs on the host and needs host paths.
//
// It inspects the volume via `<containerBinary> volume inspect` and
// looks for the host path in two places (in order):
//  1. Options.device — set when the volume was created with --opt device=<path>
//  2. Mountpoint — the default mount location managed by the container runtime
//
// On Windows the device path may use Linux-style /mnt/<drive>/ notation
// (e.g. /mnt/c/Users/...) which is converted to native Windows paths.
//
// Returns fallback unchanged if the volume cannot be inspected or no
// valid host path is found.
func ResolveVolumeHostPath(ctx context.Context, log logr.Logger, containerBinary, volumeName, fallback string) string {
	cmd := exec.CommandContext(ctx, containerBinary, "volume", "inspect", volumeName)
	output, err := cmd.Output()
	if err != nil {
		log.V(3).Info("volume inspect failed, using fallback", "volume", volumeName, "error", err)
		return fallback
	}

	hostPath, ok := parseVolumeInspect(output)
	if !ok {
		log.V(3).Info("could not extract host path from volume inspect, using fallback", "volume", volumeName)
		return fallback
	}

	log.V(3).Info("resolved volume host path", "volume", volumeName, "hostPath", hostPath)
	return hostPath
}

// parseVolumeInspect extracts the host filesystem path from the JSON
// output of `<container-binary> volume inspect <name>`.
//
// It checks Options.device first (explicit bind mount), then falls
// back to Mountpoint (runtime-managed path). Each candidate is
// normalised for the current platform and validated with os.Lstat.
func parseVolumeInspect(data []byte) (string, bool) {
	var volumes []map[string]any
	if err := json.Unmarshal(data, &volumes); err != nil || len(volumes) != 1 {
		return "", false
	}
	vol := volumes[0]

	// Try Options.device first — this is what createContainerVolume sets.
	if opt, ok := vol["Options"]; ok {
		if options, ok := opt.(map[string]any); ok {
			if device, ok := options["device"]; ok {
				if deviceStr, ok := device.(string); ok {
					normalized := normalizeVolumePath(deviceStr)
					if _, err := os.Lstat(normalized); err == nil {
						return normalized, true
					}
				}
			}
		}
	}

	// Fallback to Mountpoint.
	if mp, ok := vol["Mountpoint"]; ok {
		if mpStr, ok := mp.(string); ok {
			if _, err := os.Lstat(mpStr); err == nil {
				return mpStr, true
			}
		}
	}

	return "", false
}

// BuildPathMappings returns the provider.PathMapping slice needed to
// translate container-internal paths to host paths. This is used with
// konveyor.WithPathMappings when analyzing binaries in hybrid mode.
//
// containerRoot is the path prefix inside the container (e.g. /opt/input/source).
// hostRoot is the corresponding host path resolved from the volume.
func BuildPathMappings(containerRoot, hostRoot string) []analyzerprovider.PathMapping {
	return []analyzerprovider.PathMapping{
		{From: containerRoot, To: hostRoot},
	}
}
