package util

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShouldFilterLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		filtered bool
	}{
		{
			name:     "exact Windows watcher message",
			line:     "Windows system assumed buffer larger than it is, events have likely been missed",
			filtered: true,
		},
		{
			name:     "message with timestamp prefix",
			line:     "2024/12/09 11:39:26 Error in Watcher Error channel: Windows system assumed buffer larger than it is, events have likely been missed",
			filtered: true,
		},
		{
			name:     "normal log line passes through",
			line:     `time="2024-12-09T11:36:08-08:00" level=info msg="running source analysis"`,
			filtered: false,
		},
		{
			name:     "empty line passes through",
			line:     "",
			filtered: false,
		},
		{
			name:     "partial match does not filter",
			line:     "Windows system assumed buffer",
			filtered: false,
		},
		{
			name:     "unrelated error passes through",
			line:     "Error in Watcher Error channel: some other error",
			filtered: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldFilterLine(tt.line)
			if got != tt.filtered {
				t.Errorf("ShouldFilterLine(%q) = %v, want %v", tt.line, got, tt.filtered)
			}
		})
	}
}

func TestFilterStderr(t *testing.T) {
	inputR, inputW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outputR, outputW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	lines := []string{
		"normal line 1",
		"2024/12/09 11:39:26 Error in Watcher Error channel: Windows system assumed buffer larger than it is, events have likely been missed",
		"normal line 2",
		"Windows system assumed buffer larger than it is, events have likely been missed",
		"normal line 3",
	}
	go func() {
		for _, line := range lines {
			inputW.WriteString(line + "\n")
		}
		inputW.Close()
	}()

	FilterStderr(inputR, outputW)
	outputW.Close()

	outputBytes, err := io.ReadAll(outputR)
	if err != nil {
		t.Fatal(err)
	}
	output := string(outputBytes)
	outputR.Close()
	inputR.Close()

	if strings.Contains(output, "Windows system assumed buffer larger than it is") {
		t.Error("filtered pattern should not appear in output")
	}
	for _, expected := range []string{"normal line 1", "normal line 2", "normal line 3"} {
		if !strings.Contains(output, expected) {
			t.Errorf("expected %q in output, got: %q", expected, output)
		}
	}
}

func TestFilterStderrEmptyInput(t *testing.T) {
	inputR, inputW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outputR, outputW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	inputW.Close()

	FilterStderr(inputR, outputW)
	outputW.Close()

	outputBytes, err := io.ReadAll(outputR)
	if err != nil {
		t.Fatal(err)
	}
	if len(outputBytes) != 0 {
		t.Errorf("expected no output for empty input, got %d bytes: %q", len(outputBytes), string(outputBytes))
	}

	outputR.Close()
	inputR.Close()
}

func TestFilterStderrWriteError(t *testing.T) {
	inputR, inputW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	outputR, outputW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	// Close the dest write end to force a write error in FilterStderr
	outputW.Close()

	go func() {
		inputW.WriteString("line 1\n")
		inputW.WriteString("line 2\n")
		inputW.Close()
	}()

	// Should return without panicking when write fails
	FilterStderr(inputR, outputW)

	outputR.Close()
	inputR.Close()
}

func TestFilterStderrPartialLine(t *testing.T) {
	inputR, inputW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outputR, outputW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	// Write data without a trailing newline to exercise the partial-line flush path.
	go func() {
		inputW.WriteString("complete line\npartial no newline")
		inputW.Close()
	}()

	FilterStderr(inputR, outputW)
	outputW.Close()

	outputBytes, err := io.ReadAll(outputR)
	if err != nil {
		t.Fatal(err)
	}
	output := string(outputBytes)
	outputR.Close()
	inputR.Close()

	if !strings.Contains(output, "complete line\n") {
		t.Errorf("expected complete line in output, got: %q", output)
	}
	if !strings.Contains(output, "partial no newline") {
		t.Errorf("expected partial line in output, got: %q", output)
	}
}

func TestFilterStderrPartialLineFiltered(t *testing.T) {
	inputR, inputW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outputR, outputW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	// Write a filter-matching line without a trailing newline.
	go func() {
		inputW.WriteString("Windows system assumed buffer larger than it is, events have likely been missed")
		inputW.Close()
	}()

	FilterStderr(inputR, outputW)
	outputW.Close()

	outputBytes, err := io.ReadAll(outputR)
	if err != nil {
		t.Fatal(err)
	}
	if len(outputBytes) != 0 {
		t.Errorf("expected no output for filtered partial line, got: %q", string(outputBytes))
	}
	outputR.Close()
	inputR.Close()
}

func TestInstallStderrFilter_ReturnsRestoreFunc(t *testing.T) {
	restore := InstallStderrFilter()
	if restore == nil {
		t.Fatal("InstallStderrFilter should return a non-nil restore function")
	}
	// Calling restore should not panic
	restore()
}

func TestInstallStderrFilter_RestoreIsIdempotent(t *testing.T) {
	restore := InstallStderrFilter()
	restore()
	// Calling restore a second time should not panic
	restore()
}

func TestGetKantraDir_KANTRA_DIR_set(t *testing.T) {
	// Save and restore so we don't affect other tests or the process
	orig := os.Getenv(KantraDirEnv)
	defer func() {
		if orig != "" {
			os.Setenv(KantraDirEnv, orig)
		} else {
			os.Unsetenv(KantraDirEnv)
		}
	}()

	tests := []struct {
		name    string
		envDir  string
		wantDir string // expected cleaned path
	}{
		{
			name:    "existing absolute path",
			envDir:  os.TempDir(),
			wantDir: filepath.Clean(os.TempDir()),
		},
		{
			name:    "path with trailing slash returns cleaned",
			envDir:  os.TempDir() + string(filepath.Separator),
			wantDir: filepath.Clean(os.TempDir()),
		},
		{
			name:    "non-existing path still returned",
			envDir:  "/nonexistent/kantra/dir",
			wantDir: "/nonexistent/kantra/dir",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv(KantraDirEnv, tt.envDir)
			got, err := GetKantraDir()
			if err != nil {
				t.Fatalf("GetKantraDir() error = %v", err)
			}
			if got != tt.wantDir {
				t.Errorf("GetKantraDir() = %q, want %q", got, tt.wantDir)
			}
		})
	}
}

func TestGetKantraDir_KANTRA_DIR_empty_unchanged_behavior(t *testing.T) {
	orig := os.Getenv(KantraDirEnv)
	defer func() {
		if orig != "" {
			os.Setenv(KantraDirEnv, orig)
		} else {
			os.Unsetenv(KantraDirEnv)
		}
	}()
	os.Unsetenv(KantraDirEnv)

	// When KANTRA_DIR is unset, we get either cwd (if reqs present) or home/.kantra.
	// We only assert we get a non-empty path and no error.
	got, err := GetKantraDir()
	if err != nil {
		t.Fatalf("GetKantraDir() with KANTRA_DIR unset error = %v", err)
	}
	if got == "" {
		t.Error("GetKantraDir() with KANTRA_DIR unset returned empty path")
	}
}
