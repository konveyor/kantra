package util

import (
	"bytes"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	// MavenCacheDir is the container-internal directory for the Maven
	// local repository cache. Mounted under /opt/input so it is
	// accessible regardless of which user the container runs as.
	MavenCacheDir = path.Join(InputPath, "maven-cache")
	// SourceMountPath is the directory where source code is mounted in the container.
	// This value must not be modified at runtime. Use analyzeCommand.sourceLocationPath
	// for the resolved source location (which may include a filename for file inputs).
	SourceMountPath = path.Join(InputPath, "source")
	// ConfigMountPath analyzer config files
	ConfigMountPath = path.Join(InputPath, "config")
	// RulesMountPath user provided rules path
	RulesMountPath = path.Join(RulesetPath, "input")
	// AnalysisOutputMountPath paths to files in the container
	AnalysisOutputMountPath   = path.Join(OutputPath, "output.yaml")
	DepsOutputMountPath       = path.Join(OutputPath, "dependencies.yaml")
	ProviderSettingsMountPath = path.Join(ConfigMountPath, "settings.json")
)

// analyzer container paths
const (
	RulesetPath            = "/opt/rulesets"
	OpenRewriteRecipesPath = "/opt/openrewrite"
	InputPath              = "/opt/input"
	OutputPath             = "/opt/output"
	CustomRulePath         = "/opt/input/rules"
)

// supported providers
const (
	JavaProvider   = "java"
	GoProvider     = "go"
	PythonProvider = "python"
	NodeJSProvider = "nodejs"
	CsharpProvider = "csharp"
)

var DefaultRulesetDir = map[string]string{
	JavaProvider:   "java",
	NodeJSProvider: "nodejs",
	CsharpProvider: "dotnet",
}

// valid java file extensions
const (
	JavaArchive       = ".jar"
	WebArchive        = ".war"
	EnterpriseArchive = ".ear"
	ClassFile         = ".class"
)

func CopyFolderContents(src string, dst string) error {
	err := os.MkdirAll(dst, os.ModePerm)
	if err != nil {
		return err
	}
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	contents, err := source.Readdir(-1)
	if err != nil {
		return err
	}

	for _, item := range contents {
		sourcePath := filepath.Join(src, item.Name())
		destinationPath := filepath.Join(dst, item.Name())

		if item.IsDir() {
			// Recursively copy subdirectories
			if err := CopyFolderContents(sourcePath, destinationPath); err != nil {
				return err
			}
		} else {
			// Copy file
			if err := CopyFileContents(sourcePath, destinationPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func CopyFileContents(src string, dst string) (err error) {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()
	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()
	_, err = io.Copy(destination, source)
	if err != nil {
		return err
	}
	return nil
}

func LoadEnvInsensitive(variableName string) string {
	lowerValue := os.Getenv(strings.ToLower(variableName))
	upperValue := os.Getenv(strings.ToUpper(variableName))
	if lowerValue != "" {
		return lowerValue
	} else {
		return upperValue
	}
}

// KantraDirEnv is the environment variable that can override the kantra directory
// (e.g. when the binary is invoked with a different working directory, as in runLocal).
// StderrFilterPatterns contains substrings to filter from stderr output.
// Lines containing any of these patterns will be silently dropped.
var StderrFilterPatterns = []string{
	"Windows system assumed buffer larger than it is, events have likely been missed",
}

// FilterStderr reads from r, drops lines matching any pattern in
// StderrFilterPatterns, and writes the rest to dest. It uses a chunk-based
// approach to avoid deadlocks when stderr output contains long stretches
// without newlines (e.g. progress indicators), which would cause a
// line-oriented scanner to block and fill the pipe buffer.
func FilterStderr(r *os.File, dest *os.File) {
	buf := make([]byte, 32*1024)
	var lineBuf []byte
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			for len(chunk) > 0 {
				idx := bytes.IndexByte(chunk, '\n')
				if idx < 0 {
					// No newline in remaining chunk; buffer it.
					lineBuf = append(lineBuf, chunk...)
					break
				}
				// Complete line found.
				lineBuf = append(lineBuf, chunk[:idx]...)
				line := string(lineBuf)
				lineBuf = lineBuf[:0]
				chunk = chunk[idx+1:]
				if ShouldFilterLine(line) {
					continue
				}
				if _, werr := dest.WriteString(line + "\n"); werr != nil {
					return
				}
			}
		}
		if err != nil {
			break
		}
	}
	// Flush any remaining partial line.
	if len(lineBuf) > 0 {
		line := string(lineBuf)
		if !ShouldFilterLine(line) {
			dest.WriteString(line)
		}
	}
}

// ShouldFilterLine returns true if the line matches any stderr filter pattern.
func ShouldFilterLine(line string) bool {
	for _, pattern := range StderrFilterPatterns {
		if strings.Contains(line, pattern) {
			return true
		}
	}
	return false
}

const KantraDirEnv = "KANTRA_DIR"

// GetKantraDir returns the directory used for rulesets, jdtls, and static-report.
// Resolution order: 1) KANTRA_DIR env var (if set), 2) current directory if it
// contains "rulesets", "jdtls", and "static-report", 3) $HOME/.kantra (or
// $XDG_CONFIG_HOME/.kantra on Linux when set).
func GetKantraDir() (string, error) {
	var dir string
	var err error
	reqs := []string{
		"rulesets",
		"jdtls",
		"static-report",
	}

	// Allow explicit override (e.g. from parent process when running kantra with cmd.Dir set)
	if envDir := os.Getenv(KantraDirEnv); envDir != "" {
		if _, err := os.Stat(envDir); err == nil {
			return filepath.Clean(envDir), nil
		}
		// env set but path missing: still use it so callers get a consistent error
		return filepath.Clean(envDir), nil
	}

	set := true
	// check current dir first for reqs
	dir, err = os.Getwd()
	if err != nil {
		return "", err
	}
	for _, v := range reqs {
		_, err := os.Stat(filepath.Join(dir, v))
		if err != nil {
			set = false
			break
		}
	}
	// all reqs found here
	if set {
		return dir, nil
	}
	// fall back to $HOME/.kantra
	ops := runtime.GOOS
	if ops == "linux" {
		dir, set = os.LookupEnv("XDG_CONFIG_HOME")
	}
	if ops != "linux" || dir == "" || !set {
		// on Unix, including macOS, this returns the $HOME environment variable. On Windows, it returns %USERPROFILE%
		dir, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, ".kantra"), nil
}
