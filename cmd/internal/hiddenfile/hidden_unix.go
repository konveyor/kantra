//go:build !windows && !windows
// +build !windows,!windows

package hiddenfile

const dotCharacter = 46

func IsHidden(name string) (bool, error) {
	if name[0] == dotCharacter {
		return true, nil
	}

	return false, nil
}
