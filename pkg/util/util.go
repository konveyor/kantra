package util

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	outputv1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
)

var (
	RootCommandName      = "kantra"
	JavaBundlesLocation  = "/jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar"
	JDTLSBinLocation     = "/jdtls/bin/jdtls"
	RulesetsLocation     = "rulesets"
	JavaProviderImage    = "quay.io/konveyor/java-external-provider"
	GenericProviderImage = "quay.io/konveyor/generic-external-provider"
	DotnetProviderImage  = "quay.io/konveyor/dotnet-external-provider"
)

// provider config options
const (
	MavenSettingsFile      = "mavenSettingsFile"
	LspServerPath          = "lspServerPath"
	LspServerName          = "lspServerName"
	WorkspaceFolders       = "workspaceFolders"
	DependencyProviderPath = "dependencyProviderPath"
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

func IsXMLFile(rule string) bool {
	return path.Ext(rule) == ".xml"
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

func IsXMLDirEmpty(dir string) (bool, error) {
	f, err := os.Open(dir)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdir(1)
	if err == io.EOF {
		return true, nil
	}

	return false, err
}
