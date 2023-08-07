package cmd

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/apex/log"
	"github.com/codingconcepts/env"
	outputv1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"
)

var (
	listSources bool
	listTargets bool
)

type Settings struct {
	RuleSetPath string `env:"RULESET_PATH" default:"/opt/rulesets/"`
}

// analyzeCmd represents the analyze command
var analyzeCmd = &cobra.Command{
	Use: "analyze",

	// TODO:  need better descriptions
	Short: "A tool to analyze applications",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {

		err := AnalyzeFlags()
		if err != nil {
			log.Errorf("Failed to execute analyzeFlags")
		}
	},
}

func init() {
	rootCmd.AddCommand(analyzeCmd)

	analyzeCmd.PersistentFlags().BoolVar(&listSources, "list-sources", false, "List rules for available migration sources")
	analyzeCmd.PersistentFlags().BoolVar(&listTargets, "list-targets", false, "List rules for available migration targets")

}

func AnalyzeFlags() error {
	// reserved labels
	sourceLabel := outputv1.SourceTechnologyLabel
	targetLabel := outputv1.TargetTechnologyLabel

	if listSources {
		sourceSlice, err := readRuleFilesForLabels(sourceLabel)
		if err != nil {
			return err
		}
		listOptionsFromLabels(sourceSlice, sourceLabel)
	}
	if listTargets {
		targetsSlice, err := readRuleFilesForLabels(targetLabel)
		if err != nil {
			return err
		}
		listOptionsFromLabels(targetsSlice, targetLabel)
	}
	return nil
}

func readRuleFilesForLabels(label string) ([]string, error) {
	p := &Settings{}
	if err := env.Set(p); err != nil {
		return nil, err
	}

	var labelsSlice []string
	err := filepath.WalkDir(p.RuleSetPath, walkRuleSets(p.RuleSetPath, label, &labelsSlice))
	if err != nil {
		return nil, err
	}
	return labelsSlice, nil
}

func walkRuleSets(root string, label string, labelsSlice *[]string) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {

		if !d.IsDir() {
			*labelsSlice, err = readRuleFiles(path, labelsSlice, label)
			if err != nil {
				return err
			}
		}
		return err
	}
}

func readRuleFiles(filePath string, labelsSlice *[]string, label string) ([]string, error) {
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

func listOptionsFromLabels(sl []string, label string) {
	var newSl []string
	l := label + "="

	for _, label := range sl {
		newSt := strings.TrimPrefix(label, l)

		if newSt != label {
			newSl = append(newSl, newSt)
		}
	}
	sort.Strings(newSl)

	if label == outputv1.SourceTechnologyLabel {
		fmt.Println("available source technologies:")
	} else {
		fmt.Println("available target technologies:")
	}

	for _, tech := range newSl {
		fmt.Println(tech)
	}
}
