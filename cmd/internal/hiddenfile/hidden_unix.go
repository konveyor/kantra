//go:build !windows && !windows
// +build !windows,!windows

package hiddenfile

import "path/filepath"

const dotCharacter = 46

func IsHidden(path string) (bool, error) {
	filename := filepath.Base(path)
	if filename[0] == dotCharacter {
		return true, nil
	}

	return false, nil
}
