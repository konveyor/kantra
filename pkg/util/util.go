package util

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	outputv1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
)

// provider config options
const (
	MavenSettingsFile      = "mavenSettingsFile"
	LspServerPath          = "lspServerPath"
	LspServerName          = "lspServerName"
	WorkspaceFolders       = "workspaceFolders"
	DependencyProviderPath = "dependencyProviderPath"
)

var (
	// TODO (pgaikwad): this assumes that the $USER in container is always root, it may not be the case in future
	M2Dir = path.Join("/", "root", ".m2")
	// SourceMountPath application source path inside the container
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
		return nil
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

func WalkRuleSets(root string, label string, labelsSlice *[]string) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			*labelsSlice, err = readRuleFile(path, labelsSlice, label)
			if err != nil {
				return err
			}
		}
		return err
	}
}

func readRuleFile(filePath string, labelsSlice *[]string, label string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanWords)

	for scanner.Scan() {
		// add source/target labels to slice
		label := getSourceOrTargetLabel(scanner.Text(), label)
		if len(label) > 0 && !slices.Contains(*labelsSlice, label) {
			*labelsSlice = append(*labelsSlice, label)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return *labelsSlice, nil
}

func getSourceOrTargetLabel(text string, label string) string {
	if strings.Contains(text, label) {
		return text
	}
	return ""
}

func ListOptionsFromLabels(sl []string, label string, out io.Writer) {
	var newSl []string
	l := label + "="

	for _, label := range sl {
		newSt := strings.TrimPrefix(label, l)

		if newSt != label {
			newSt = strings.TrimSuffix(newSt, "+")
			newSt = strings.TrimSuffix(newSt, "-")

			if !slices.Contains(newSl, newSt) {
				newSl = append(newSl, newSt)

			}
		}
	}
	sort.Strings(newSl)

	if label == outputv1.SourceTechnologyLabel {
		fmt.Fprintln(out, "available source technologies:")
	} else {
		fmt.Fprintln(out, "available target technologies:")
	}
	for _, tech := range newSl {
		fmt.Fprintln(out, tech)
	}
}

const ProfilesPath = ".konveyor/profiles"

func GetProfilesExcludedDir(inputPath string, useContainerPath bool) string {
	profilesDir := filepath.Join(inputPath, ProfilesPath)
	if _, err := os.Stat(profilesDir); err == nil {
		if useContainerPath {
			return path.Join(SourceMountPath, ProfilesPath)
		}
		return profilesDir
	}
	return ""
}

// KantraDirEnv is the environment variable that can override the kantra directory
// (e.g. when the binary is invoked with a different working directory, as in runLocal).
const KantraDirEnv = "KANTRA_DIR"

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
