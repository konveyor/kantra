package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/konveyor-ecosystem/kantra/pkg/container"
	"github.com/spf13/cobra"
	"golang.org/x/exp/maps"

	"github.com/fabianvf/windup-rulesets-yaml/pkg/conversion"
	"github.com/fabianvf/windup-rulesets-yaml/pkg/windup"
)

type windupShimCommand struct {
	input  []string
	output string

	cleanup bool
}

func NewWindupShimCommand() *cobra.Command {
	windupShimCmd := &windupShimCommand{
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
				analysisLogger.Error(err, "failed to validate flags")
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
				analysisLogger.Error(err, "failed to execute windup shim")
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
			analysisLogger.V(1).Error(err, "failed to stat rules")
			return nil, err
		}
		// move xml rule files from user into dir to mount
		if !stat.IsDir() {
			mountTempDir = true
			xmlFileName := filepath.Base(r)
			destFile := filepath.Join(tempRuleDir, xmlFileName)
			err := CopyFileContents(r, destFile)
			if err != nil {
				analysisLogger.V(1).Error(err, "failed to move rules file from source to destination", "src", r, "dest", destFile)
				return nil, err
			}
		} else {
			rulesVolumes[r] = path.Join(XMLRulePath, filepath.Base(r))
		}
	}
	if mountTempDir {
		rulesVolumes[tempRuleDir] = XMLRulePath
	}
	return rulesVolumes, nil
}

func (w *windupShimCommand) Run(ctx context.Context) error {
	tempDir, err := os.MkdirTemp("", "transform-rules-")
	if err != nil {
		analysisLogger.V(1).Error(err, "failed to create temp dir for rules")
		return err
	}
	analysisLogger.V(1).Info("created temp directory for XML rules", "dir", tempDir)
	if w.cleanup {
		defer os.RemoveAll(tempDir)
	}
	volumes := map[string]string{
		w.output: ShimOutputPath,
	}
	ruleVols, err := w.getRulesVolumes(tempDir)
	if err != nil {
		analysisLogger.V(1).Error(err, "failed to get xml rules for conversion")
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
		fmt.Sprintf("--outputdir=%v", ShimOutputPath),
		XMLRulePath,
	}
	analysisLogger.Info("running windup-shim convert command",
		"args", strings.Join(args, " "), "volumes", volumes, "output", w.output, "inputs", strings.Join(w.input, ","))
	analysisLogger.Info("generating shim log in file", "file", shimLogPath)
	err = container.NewContainer().Run(
		ctx,
		container.WithImage(Settings.RunnerImage),
		container.WithLog(analysisLogger.V(1)),
		container.WithVolumes(volumes),
		container.WithStdout(shimLog),
		container.WithStderr(shimLog),
		container.WithEntrypointArgs(args...),
		container.WithEntrypointBin("/usr/local/bin/windup-shim"),
		container.WithContainerToolBin(Settings.ContainerBinary),
		container.WithCleanup(w.cleanup),
	)
	if err != nil {
		analysisLogger.V(1).Error(err, "failed to run convert command")
		return err
	}
	return nil
}

func (a *analyzeCommand) ConvertXMLContainerless() (string, error) {
	shimLogFilePath := filepath.Join(a.output, "shim.log")
	shimLog, err := os.Create(shimLogFilePath)
	if err != nil {
		return "", err
	}
	defer shimLog.Close()
	os.Stdout = shimLog

	tempDir, err := os.MkdirTemp("", "analyze-rules-")
	if err != nil {
		analysisLogger.V(1).Error(err, "failed creating temp dir", "dir", tempDir)
		return "", err
	}
	analysisLogger.V(7).Info("created temp directory for xml rules", "dir", tempDir)

	for _, location := range a.rules {
		rulesets := []windup.Ruleset{}
		ruletests := []windup.Ruletest{}
		err := filepath.WalkDir(location, walkXML(location, &rulesets, &ruletests))
		if err != nil {
			analysisLogger.V(1).Error(err, "failed to get get xml rule")
		}
		_, err = conversion.ConvertWindupRulesetsToAnalyzer(rulesets, location, tempDir, true, false)
		if err != nil {
			log.Fatal(err)
		}
	}

	return tempDir, nil
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
