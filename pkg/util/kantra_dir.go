package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const kantraInstallHint = "Install containerless dependencies by extracting the kantra release archive into ~/.kantra, " +
	"or run from a directory that contains rulesets/, jdtls/, and static-report/. " +
	"Set KANTRA_DIR to point at an existing installation directory."

// MissingKantraDirectory returns an error when the kantra installation directory does not exist.
func MissingKantraDirectory(kantraDir string) error {
	var b strings.Builder
	b.WriteString("kantra installation directory not found\n")
	b.WriteString(fmt.Sprintf("  path: %s\n", kantraDir))
	b.WriteString(kantraDirResolutionNote(kantraDir))
	b.WriteString("\n\n")
	b.WriteString(kantraInstallHint)
	return fmt.Errorf("%s", b.String())
}

// MissingKantraPath returns an error when a required path under the kantra directory is missing.
func MissingKantraPath(kantraDir, missingPath string) error {
	rel := missingPath
	if r, err := filepath.Rel(kantraDir, missingPath); err == nil && r != "." && !strings.HasPrefix(r, "..") {
		rel = r
	}

	var b strings.Builder
	b.WriteString("missing kantra dependency\n")
	b.WriteString(fmt.Sprintf("  required: %s\n", rel))
	b.WriteString(fmt.Sprintf("  expected at: %s\n", missingPath))
	b.WriteString(fmt.Sprintf("  kantra directory: %s\n", kantraDir))
	b.WriteString(kantraDirResolutionNote(kantraDir))
	b.WriteString("\n\n")
	b.WriteString(kantraInstallHint)
	return fmt.Errorf("%s", b.String())
}

// CheckKantraSubpath verifies that kantraDir exists and contains subpath.
func CheckKantraSubpath(kantraDir, subpath string) error {
	if _, err := os.Stat(kantraDir); os.IsNotExist(err) {
		return MissingKantraDirectory(kantraDir)
	} else if err != nil {
		return err
	}

	full := filepath.Join(kantraDir, subpath)
	if _, err := os.Stat(full); os.IsNotExist(err) {
		return MissingKantraPath(kantraDir, full)
	} else if err != nil {
		return err
	}
	return nil
}

func kantraDirResolutionNote(kantraDir string) string {
	if envDir := os.Getenv(KantraDirEnv); envDir != "" {
		return fmt.Sprintf("  (resolved from %s=%q)", KantraDirEnv, filepath.Clean(envDir))
	}
	return fmt.Sprintf("  (auto-resolved; set %s=%q to override)", KantraDirEnv, kantraDir)
}
