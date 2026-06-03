package util

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingKantraPath(t *testing.T) {
	kantraDir := filepath.Join("/home", "me", ".kantra")
	missing := filepath.Join(kantraDir, "rulesets")

	err := MissingKantraPath(kantraDir, missing)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, want := range []string{
		"missing kantra dependency",
		"required: rulesets",
		missing,
		kantraDir,
		kantraInstallHint,
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestMissingKantraDirectory(t *testing.T) {
	kantraDir := filepath.Join("/home", "me", ".kantra")
	err := MissingKantraDirectory(kantraDir)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, want := range []string{
		"kantra installation directory not found",
		kantraDir,
		kantraInstallHint,
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestMissingKantraPath_KANTRA_DIR_set(t *testing.T) {
	orig := os.Getenv(KantraDirEnv)
	t.Cleanup(func() {
		if orig != "" {
			os.Setenv(KantraDirEnv, orig)
		} else {
			os.Unsetenv(KantraDirEnv)
		}
	})

	kantraDir := "/custom/kantra"
	os.Setenv(KantraDirEnv, kantraDir)
	missing := filepath.Join(kantraDir, "rulesets")

	err := MissingKantraPath(kantraDir, missing)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `KANTRA_DIR="/custom/kantra"`) {
		t.Errorf("expected KANTRA_DIR in error, got %q", err.Error())
	}
}

func TestCheckKantraSubpath(t *testing.T) {
	tmp := t.TempDir()
	rulesets := filepath.Join(tmp, "rulesets")
	if err := os.MkdirAll(rulesets, 0755); err != nil {
		t.Fatal(err)
	}

	if err := CheckKantraSubpath(tmp, "rulesets"); err != nil {
		t.Fatalf("CheckKantraSubpath() unexpected error = %v", err)
	}

	if err := CheckKantraSubpath(tmp, "jdtls"); err == nil {
		t.Fatal("expected error for missing subpath")
	} else if !strings.Contains(err.Error(), "missing kantra dependency") {
		t.Fatalf("unexpected error = %v", err)
	}

	missingDir := filepath.Join(tmp, "missing")
	if err := CheckKantraSubpath(missingDir, "rulesets"); err == nil {
		t.Fatal("expected error for missing kantra directory")
	} else if !strings.Contains(err.Error(), "kantra installation directory not found") {
		t.Fatalf("unexpected error = %v", err)
	}
}
