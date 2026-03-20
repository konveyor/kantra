//go:build !windows

package util

// InstallStderrFilter is a no-op on non-Windows platforms.
// The "Windows system assumed buffer larger than it is" message
// from nxadm/tail's fsnotify watcher only occurs on Windows.
func InstallStderrFilter() (restore func()) {
	return func() {}
}
