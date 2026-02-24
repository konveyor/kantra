package util

import (
	"os"
	"path/filepath"
	"testing"
)

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
			wantDir: os.TempDir(),
		},
		{
			name:    "path with trailing slash returns cleaned",
			envDir:  os.TempDir() + string(filepath.Separator),
			wantDir: os.TempDir(),
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
