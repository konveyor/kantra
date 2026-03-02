//go:build !windows

package provider

// normalizeVolumePath is a no-op on Unix platforms.
// The device path from volume inspect is already a valid host path.
func normalizeVolumePath(p string) string {
	return p
}
