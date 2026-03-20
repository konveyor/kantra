//go:build !windows

package analyze

// installStderrFilter is a no-op on non-Windows platforms.
// The "Windows system assumed buffer larger than it is" message
// from nxadm/tail's fsnotify watcher only occurs on Windows.
func installStderrFilter() (restore func()) {
	return func() {}
}
