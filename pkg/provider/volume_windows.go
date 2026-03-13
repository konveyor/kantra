//go:build windows

package provider

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var windowsMountRegex = regexp.MustCompile(`/mnt/([a-z])/`)

// normalizeVolumePath converts Linux-style mount paths used by
// container runtimes on Windows (e.g. /mnt/c/Users/...) to native
// Windows paths (e.g. C:\Users\...).
//
// Podman and Docker on Windows store volume device paths using the
// /mnt/<drive>/ convention internally. This function translates them
// back so that os.Lstat and the builtin provider can find the files.
func normalizeVolumePath(p string) string {
	if !windowsMountRegex.MatchString(p) {
		return p
	}
	drive := windowsMountRegex.FindStringSubmatch(p)
	if len(drive) != 2 {
		return p
	}
	// Strip the /mnt/<drive>/ prefix and convert forward slashes
	stripped := filepath.FromSlash(windowsMountRegex.ReplaceAllString(p, ""))
	return fmt.Sprintf("%s:\\%s", strings.ToUpper(drive[1]), stripped)
}
