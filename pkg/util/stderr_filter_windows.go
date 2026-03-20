//go:build windows

package util

import (
	"os"
	"sync"

	"golang.org/x/sys/windows"
)

// InstallStderrFilter redirects stderr through a filtering pipe on Windows
// to suppress noisy "Windows system assumed buffer larger than it is" messages
// from the nxadm/tail library's internal fsnotify watcher.
//
// This works by duplicating the original stderr handle, pointing the Win32
// standard error handle at a pipe, and running a goroutine that filters
// lines before writing them to the original stderr.
func InstallStderrFilter() (restore func()) {
	noop := func() {}

	origHandle := windows.Handle(os.Stderr.Fd())

	// Duplicate the original stderr handle so we can still write to it.
	var savedHandle windows.Handle
	proc := windows.CurrentProcess()
	err := windows.DuplicateHandle(
		proc, origHandle,
		proc, &savedHandle,
		0, true, windows.DUPLICATE_SAME_ACCESS,
	)
	if err != nil {
		return noop
	}
	origStderr := os.NewFile(uintptr(savedHandle), "stderr-orig")

	pr, pw, err := os.Pipe()
	if err != nil {
		origStderr.Close()
		return noop
	}

	// Point the Win32 standard error handle to our pipe.
	if err := windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(pw.Fd())); err != nil {
		pr.Close()
		pw.Close()
		origStderr.Close()
		return noop
	}

	// Replace Go's os.Stderr so new Go code also writes to the pipe.
	os.Stderr = pw

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		FilterStderr(pr, origStderr)
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			// Close the pipe write end so the filter goroutine sees EOF.
			pw.Close()
			// Wait for the filter goroutine to finish draining.
			wg.Wait()
			// Now safe to close the pipe read end.
			pr.Close()
			// Restore the Win32 standard error handle and Go's os.Stderr.
			// Reuse origStderr directly rather than closing it and recreating
			// from savedHandle, which would leave stderr pointing at a closed handle.
			windows.SetStdHandle(windows.STD_ERROR_HANDLE, savedHandle)
			os.Stderr = origStderr
		})
	}
}
