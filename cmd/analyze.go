package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"os"

	"path/filepath"
	"sort"
	"strings"

	"github.com/apex/log"
	"github.com/konveyor/analyzer-lsp/engine"
	outputv1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"gopkg.in/yaml.v2"

	"github.com/spf13/cobra"
	"golang.org/x/exp/maps"
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
	rules            []string
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
	analyzeCommand.Flags().StringArrayVar(&analyzeCmd.rules, "rules", []string{}, "filename or directory containing rule files")
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
	fp := filepath.Join(a.output, "output.yaml")
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

func (a *analyzeCommand) getRules(ruleMountedPath string, wd string, dirName string) (map[string]string, error) {
	rulesMap := make(map[string]string)
	rulesetNeeded := false
	err := os.Mkdir(dirName, os.ModePerm)
	if err != nil {
		return nil, err
	}
	for i, r := range a.rules {
		stat, err := os.Stat(r)
		if err != nil {
			log.Errorf("failed to stat rules %s", r)
			return nil, err
		}
		// move rules files passed into dir to mount
		if !stat.IsDir() {
			rulesetNeeded = true
			destFile := filepath.Join(dirName, fmt.Sprintf("rules%v.yaml", i))
			err := copyFileContents(r, destFile)
			if err != nil {
				return nil, err
			}
		} else {
			dirName = r
		}
	}
	if rulesetNeeded {
		createTempRuleSet(wd, dirName)
	}
	// add to volumes
	rulesMap[dirName] = ruleMountedPath
	return rulesMap, nil
}

func copyFileContents(src string, dst string) (err error) {
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

func createTempRuleSet(wd string, tempDirName string) error {
	tempRuleSet := engine.RuleSet{
		Name:        "ruleset",
		Description: "temp ruleset",
	}
	yamlData, err := yaml.Marshal(&tempRuleSet)
	if err != nil {
		return err
	}
	fileName := "ruleset.yaml"
	err = ioutil.WriteFile(fileName, yamlData, os.ModePerm)
	if err != nil {
		return err
	}
	// move temp ruleset into temp dir
	rulsetDefault := filepath.Join(wd, "ruleset.yaml")
	destRuleSet := filepath.Join(tempDirName, "ruleset.yaml")
	err = copyFileContents(rulsetDefault, destRuleSet)
	if err != nil {
		return err
	}
	defer os.Remove(rulsetDefault)
	return nil
}

func (a *analyzeCommand) Run(ctx context.Context) error {
	outputFilePath, err := a.createOutputFile()
	if err != nil {
		return err
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	// TODO: clean this up
	settingsFilePath := filepath.Join(wd, "settings.json")
	settingsMountedPath := filepath.Join(InputPath, "settings.json")
	outputMountedPath := filepath.Join(InputPath, "output.yaml")
	sourceAppPath := filepath.Join(InputPath, "example")
	rulesMountedPath := filepath.Join(RulesetPath, "input")
	tempDirName := filepath.Join(wd, "tempRulesDir")
	err = a.writeProviderSettings(wd, settingsFilePath, sourceAppPath)
	if err != nil {
		return err
	}
	volumes := map[string]string{
		a.input:          sourceAppPath,
		settingsFilePath: settingsMountedPath,
		outputFilePath:   outputMountedPath,
	}
	var rulePath string
	if len(a.rules) > 0 {
		ruleVols, err := a.getRules(rulesMountedPath, wd, tempDirName)
		if err != nil {
			return err
		}
		maps.Copy(volumes, ruleVols)
		rulePath = rulesMountedPath

		// use default rulesets if none given
	} else {
		rulePath = RulesetPath
	}
	args := []string{
		fmt.Sprintf("--provider-settings=%v", settingsMountedPath),
		fmt.Sprintf("--rules=%v", rulePath),
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
	defer os.RemoveAll(tempDirName)
	return nil
}
