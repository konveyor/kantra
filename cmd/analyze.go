package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"

	"path/filepath"
	"sort"
	"strings"

	"github.com/apex/log"
	outputv1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"

	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"
)

// kantra analyze flags
type analyzeCommand struct {
	listSources      bool
	listTargets      bool
	skipStaticReport bool
	sources          []string
	targets          []string
	input            string
	output           string
	mode             string
	rules            string
}

// analyzeCmd represents the analyze command
func NewAnalyzeCmd() *cobra.Command {
	analyzeCmd := &analyzeCommand{}

	analyzeCommand := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze application source code",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			err := analyzeCmd.Validate()
			if err != nil {
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if analyzeCmd.listSources || analyzeCmd.listTargets {
				err := analyzeCmd.AnalyzeFlags()
				if err != nil {
					log.Errorf("Failed to execute analyzeFlags", err)
					return err
				}
				return nil
			}
			err := analyzeCmd.Run(cmd.Context())
			if err != nil {
				log.Errorf("failed to execute analyze command", err)
				return err
			}
			return nil
		},
	}
	analyzeCommand.Flags().BoolVar(&analyzeCmd.listSources, "list-sources", false, "List rules for available migration sources")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.listTargets, "list-targets", false, "List rules for available migration targets")
	analyzeCommand.Flags().StringArrayVarP(&analyzeCmd.sources, "source", "s", []string{}, "Source technology to consider for analysis")
	analyzeCommand.Flags().StringArrayVarP(&analyzeCmd.targets, "target", "t", []string{}, "Target technology to consider for analysis")
	analyzeCommand.Flags().StringVar(&analyzeCmd.rules, "rules", "", "Rules for analysis")
	analyzeCommand.Flags().StringVarP(&analyzeCmd.input, "input", "i", "", "Path to application source code or a binary")
	analyzeCommand.Flags().StringVarP(&analyzeCmd.output, "output", "o", "", "Path to the directory for analysis output")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.skipStaticReport, "skip-static-report", false, "Do not generate static report")
	analyzeCommand.Flags().StringVarP(&analyzeCmd.mode, "mode", "m", "full", "Analysis mode. Must be one of 'full' or 'source-only'")

	return analyzeCommand
}

func (a *analyzeCommand) Validate() error {
	if a.listSources || a.listTargets {
		return nil
	}
	stat, err := os.Stat(a.output)
	if err != nil {
		log.Errorf("failed to stat output directory %s", a.output)
		return err
	}
	if !stat.IsDir() {
		log.Errorf("output path %s is not a directory", a.output)
		return err
	}
	if a.mode != string(provider.FullAnalysisMode) &&
		a.mode != string(provider.SourceOnlyAnalysisMode) {
		return fmt.Errorf("mode must be one of 'full' or 'source-only'")
	}
	return nil
}

func (a *analyzeCommand) AnalyzeFlags() error {
	// reserved labels
	sourceLabel := outputv1.SourceTechnologyLabel
	targetLabel := outputv1.TargetTechnologyLabel

	if a.listSources {
		sourceSlice, err := a.readRuleFilesForLabels(sourceLabel)
		if err != nil {
			return err
		}
		listOptionsFromLabels(sourceSlice, sourceLabel)
		return nil
	}
	if a.listTargets {
		targetsSlice, err := a.readRuleFilesForLabels(targetLabel)
		if err != nil {
			return err
		}
		listOptionsFromLabels(targetsSlice, targetLabel)
		return nil
	}

	return nil
}

func (a *analyzeCommand) readRuleFilesForLabels(label string) ([]string, error) {
	var labelsSlice []string
	err := filepath.WalkDir(RulesetPath, walkRuleSets(RulesetPath, label, &labelsSlice))
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

func (a *analyzeCommand) createOutputFile() (string, error) {
	// if trailing '/' in given output dir, remove it
	trimmedOutput := strings.TrimRight(a.output, "/")
	trimmedOutput = strings.TrimRight(a.output, "\\")

	fp := filepath.Join(trimmedOutput, "output.yaml")
	outputFile, err := os.Create(fp)
	if err != nil {
		return "", err
	}
	defer outputFile.Close()
	return fp, nil
}

// TODO: *** write all of provider settings here ***
func (a *analyzeCommand) writeProviderSettings(dir string, settingsFilePath string, sourceAppPath string) error {

	providerConfig := []provider.Config{}
	jsonFile, err := os.Open(settingsFilePath)
	if err != nil {
		return err
	}
	defer jsonFile.Close()
	byteValue, err := ioutil.ReadAll(jsonFile)
	err = json.Unmarshal(byteValue, &providerConfig)
	if err != nil {
		return err
	}
	for i := range providerConfig {
		for init := range providerConfig[i].InitConfig {
			providerConfig[i].InitConfig[init].Location = sourceAppPath
		}
	}
	providerLocation, err := json.MarshalIndent(providerConfig, "", "	")
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(settingsFilePath, providerLocation, 0644)
	if err != nil {
		return err
	}
	return nil
}

func (a *analyzeCommand) Run(ctx context.Context) error {
	if len(a.rules) == 0 {
		a.rules = RulesetPath
	}
	outputFilePath, err := a.createOutputFile()
	if err != nil {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	settingsFilePath := filepath.Join(dir, "settings.json")
	settingsMountedPath := filepath.Join(InputPath, "settings.json")
	outputMountedPath := filepath.Join(InputPath, "output.yaml")
	sourceAppPath := filepath.Join(InputPath, "example")
	err = a.writeProviderSettings(dir, settingsFilePath, sourceAppPath)
	if err != nil {
		return err
	}
	volumes := map[string]string{
		a.input:          sourceAppPath,
		settingsFilePath: settingsMountedPath,
		outputFilePath:   outputMountedPath,
	}
	args := []string{
		fmt.Sprintf("--provider-settings=%v", settingsMountedPath),
		fmt.Sprintf("--rules=%v", a.rules),
		fmt.Sprintf("--output-file=%v", outputMountedPath),
	}
	cmd := NewContainerCommand(
		ctx,
		WithEntrypointArgs(args...),
		WithEntrypointBin("/usr/bin/konveyor-analyzer"),
		WithVolumes(volumes),
	)
	err = cmd.Run()
	if err != nil {
		return err
	}
	return nil
}
