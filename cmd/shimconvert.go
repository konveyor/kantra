package cmd

import (
	"context"
	"errors"
	"fmt"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/container"
	"github.com/spf13/cobra"
	"golang.org/x/exp/maps"

	"github.com/fabianvf/windup-rulesets-yaml/pkg/conversion"
	"github.com/fabianvf/windup-rulesets-yaml/pkg/windup"
)

type windupShimCommand struct {
	input  []string
	output string

	log     logr.Logger
	cleanup bool
}

func NewWindupShimCommand(log logr.Logger) *cobra.Command {
	windupShimCmd := &windupShimCommand{
		log:     log,
		cleanup: true,
	}

	windupShimCommand := &cobra.Command{
		Use: "rules",

		Short: "Convert XML rules to YAML",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			cmd.MarkFlagRequired("input")
			cmd.MarkFlagRequired("output")
			if err := cmd.ValidateRequiredFlags(); err != nil {
				return err
			}

			err := windupShimCmd.Validate()
			if err != nil {
				log.Error(err, "failed to validate flags")
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if val, err := cmd.Flags().GetBool(noCleanupFlag); err == nil {
				windupShimCmd.cleanup = !val
			}
			err := windupShimCmd.Run(cmd.Context())
			if err != nil {
				log.Error(err, "failed to execute windup shim")
				return err
			}
			return nil
		},
	}
	windupShimCommand.Flags().StringArrayVarP(&windupShimCmd.input, "input", "i", []string{}, "path to XML rule file(s) or directory")
	windupShimCommand.Flags().StringVarP(&windupShimCmd.output, "output", "o", "", "path to output directory")

	return windupShimCommand
}

func (w *windupShimCommand) Validate() error {
	for _, input := range w.input {
		_, err := os.Stat(input)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("input rule file or directory %s does not exist", input)
			}
			return fmt.Errorf("failed to stat input rule file or directory %s: %w", input, err)
		}

	}
	outputStat, err := os.Stat(w.output)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = os.MkdirAll(w.output, os.ModePerm)
			if err != nil {
				return fmt.Errorf("%w failed to create output dir %s", err, w.output)
			}
		} else {
			return fmt.Errorf("failed to stat output directory %s", w.output)
		}
	}
	if outputStat != nil && !outputStat.IsDir() {
		return fmt.Errorf("output path %s is not a directory", w.output)
	}
	if w.input == nil || len(w.input) == 0 {
		return fmt.Errorf("input for rule file or directory must not be empty")
	}
	for _, r := range w.input {
		if filepath.Clean(r) == filepath.Clean(w.output) {
			return fmt.Errorf("input rule directory and output directory must be different")
		}
	}
	// try to get abs path, if not, continue with relative path
	if absPath, err := filepath.Abs(w.output); err == nil {
		w.output = absPath
	}
	for idx := range w.input {
		if absPath, err := filepath.Abs(w.input[idx]); err == nil {
			w.input[idx] = absPath
		}
	}
	return nil
}

func (w *windupShimCommand) getRulesVolumes(tempRuleDir string) (map[string]string, error) {
	rulesVolumes := make(map[string]string)
	mountTempDir := false
	for _, r := range w.input {
		stat, err := os.Stat(r)
		if err != nil {
			w.log.V(1).Error(err, "failed to stat rules")
			return nil, err
		}
		// move xml rule files from user into dir to mount
		if !stat.IsDir() {
			mountTempDir = true
			xmlFileName := filepath.Base(r)
			destFile := filepath.Join(tempRuleDir, xmlFileName)
			err := util.CopyFileContents(r, destFile)
			if err != nil {
				w.log.V(1).Error(err, "failed to move rules file from source to destination", "src", r, "dest", destFile)
				return nil, err
			}
		} else {
			rulesVolumes[r] = path.Join(util.XMLRulePath, filepath.Base(r))
		}
	}
	if mountTempDir {
		rulesVolumes[tempRuleDir] = util.XMLRulePath
	}
	return rulesVolumes, nil
}

func (w *windupShimCommand) Run(ctx context.Context) error {
	tempDir, err := os.MkdirTemp("", "transform-rules-")
	if err != nil {
		w.log.V(1).Error(err, "failed to create temp dir for rules")
		return err
	}
	w.log.V(1).Info("created temp directory for XML rules", "dir", tempDir)
	if w.cleanup {
		defer os.RemoveAll(tempDir)
	}
	volumes := map[string]string{
		w.output: util.ShimOutputPath,
	}
	ruleVols, err := w.getRulesVolumes(tempDir)
	if err != nil {
		w.log.V(1).Error(err, "failed to get xml rules for conversion")
		return err
	}
	maps.Copy(volumes, ruleVols)

	shimLogPath := filepath.Join(w.output, "shim.log")
	shimLog, err := os.Create(shimLogPath)
	if err != nil {
		return fmt.Errorf("failed creating shim log file %s", shimLogPath)
	}
	defer shimLog.Close()

	args := []string{"convert",
		fmt.Sprintf("--outputdir=%v", util.ShimOutputPath),
		util.XMLRulePath,
	}
	w.log.Info("running windup-shim convert command",
		"args", strings.Join(args, " "), "volumes", volumes, "output", w.output, "inputs", strings.Join(w.input, ","))
	w.log.Info("generating shim log in file", "file", shimLogPath)
	err = container.NewContainer().Run(
		ctx,
		container.WithImage(Settings.RunnerImage),
		container.WithLog(w.log.V(1)),
		container.WithVolumes(volumes),
		container.WithStdout(shimLog),
		container.WithStderr(shimLog),
		container.WithEntrypointArgs(args...),
		container.WithEntrypointBin("/usr/local/bin/windup-shim"),
		container.WithContainerToolBin(Settings.ContainerBinary),
		container.WithCleanup(w.cleanup),
	)
	if err != nil {
		w.log.V(1).Error(err, "failed to run convert command")
		return err
	}
	return nil
}

func (a *analyzeCommand) ConvertXMLContainerless() (string, []string, error) {
	shimLogFilePath := filepath.Join(a.output, "shim.log")
	shimLog, err := os.Create(shimLogFilePath)
	if err != nil {
		return "", nil, err
	}
	originalStdout := os.Stdout
	defer func() {
		os.Stdout = originalStdout
		shimLog.Close()
	}()
	os.Stdout = shimLog

	tempDirConverted, err := os.MkdirTemp("", "converted-rules-")
	if err != nil {
		a.log.V(1).Error(err, "failed creating temp dir", "dir", tempDirConverted)
		return "", nil, err
	}
	a.log.V(7).Info("created temp directory for xml rules", "dir", tempDirConverted)

	ruleInDir, tempFileDirs, err := a.convertXmlRules(tempDirConverted)
	if err != nil {
		return "", nil, err
	}

	isDirEmpty, err := util.IsXMLDirEmpty(tempDirConverted)
	if err != nil {
		return "", nil, err
	}
	// if only xml rule dirs were passed in
	if !isDirEmpty && !ruleInDir {
		a.rules = append(a.rules, tempDirConverted)
	}

	return tempDirConverted, tempFileDirs, nil
}

func (a *analyzeCommand) convertXmlRules(tempDirConverted string) (bool, []string, error) {
	convertedDirInRules := false
	tempXmlFileDirs := []string{}
	for i, r := range a.rules {
		rulesets := []windup.Ruleset{}
		ruletests := []windup.Ruletest{}
		stat, err := os.Stat(r)
		if err != nil {
			a.log.V(1).Error(err, "failed to stat rules")
			return convertedDirInRules, nil, err
		}
		// move xml rule files from user into dir
		if !stat.IsDir() {
			if !util.IsXMLFile(r) {
				continue
			}
			convertedDirInRules, err = a.convertXmlRuleFiles(r, i, tempXmlFileDirs, convertedDirInRules, tempDirConverted)
			if err != nil {
				a.log.V(1).Error(err, "failed to stat rules")
				return convertedDirInRules, nil, err
			}
			// if xml rule passes in a dir, convert from that dir
		} else {
			err = filepath.WalkDir(r, walkXML(r, &rulesets, &ruletests))
			if err != nil {
				a.log.V(1).Error(err, "failed to get get xml rule")
			}
			_, err = conversion.ConvertWindupRulesetsToAnalyzer(rulesets, r, tempDirConverted, true, false)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
	return convertedDirInRules, tempXmlFileDirs, nil
}

func (a *analyzeCommand) convertXmlRuleFiles(r string, i int, tempXmlFileDir []string, convertedDirInRules bool, tempDirConverted string) (bool, error) {
	rulesets := []windup.Ruleset{}
	ruletests := []windup.Ruletest{}
	tempDirXml, err := os.MkdirTemp("", "xml-rules-")
	if err != nil {
		a.log.V(1).Error(err, "failed creating temp dir", "dir", tempDirXml)
		return false, err
	}
	a.log.V(7).Info("created temp directory for xml rules", "dir", tempDirXml)
	// for cleanup
	tempXmlFileDir = append(tempXmlFileDir, tempDirXml)
	xmlFileName := filepath.Base(r)
	destFile := filepath.Join(tempDirXml, xmlFileName)
	err = util.CopyFileContents(r, destFile)
	if err != nil {
		a.log.V(1).Error(err, "failed to move rules file from source to destination", "src", r, "dest", destFile)
		return false, err
	}
	if !convertedDirInRules {
		a.rules[i] = tempDirConverted
		convertedDirInRules = true
		// remove xml files from a.rules
	} else {
		a.rules = append(a.rules[:i], a.rules[i+1:]...)
	}

	err = filepath.WalkDir(tempDirXml, walkXML(tempDirXml, &rulesets, &ruletests))
	if err != nil {
		return false, err
	}
	_, err = conversion.ConvertWindupRulesetsToAnalyzer(rulesets, tempDirXml, tempDirConverted, true, false)
	if err != nil {
		return false, err
	}

	return convertedDirInRules, nil
}

func walkXML(root string, rulesets *[]windup.Ruleset, rulestest *[]windup.Ruletest) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if !strings.HasSuffix(path, ".xml") {
			return nil
		}
		if rulesets != nil {
			ruleset := windup.ProcessWindupRuleset(path)
			if ruleset != nil {
				*rulesets = append(*rulesets, *ruleset)
			}
		}
		return err
	}
}
