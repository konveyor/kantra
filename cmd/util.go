package cmd

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"sort"
	"strings"

	outputv1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
)

var (
	RootCommandName      = "kantra"
	JavaBundlesLocation  = "/jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar"
	JDTLSBinLocation     = "/jdtls/bin/jdtls"
	RulesetsLocation     = "rulesets/default/generated"
	JavaProviderImage    = "quay.io/konveyor/java-external-provider"
	GenericProviderImage = "quay.io/konveyor/generic-external-provider"
	DotnetProviderImage  = "quay.io/konveyor/dotnet-external-provider"
)

// provider config options
const (
	mavenSettingsFile      = "mavenSettingsFile"
	lspServerPath          = "lspServerPath"
	lspServerName          = "lspServerName"
	workspaceFolders       = "workspaceFolders"
	dependencyProviderPath = "dependencyProviderPath"
)

func walkRuleSets(root string, label string, labelsSlice *[]string) fs.WalkDirFunc {
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

func listOptionsFromLabels(sl []string, label string, out io.Writer) {
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
