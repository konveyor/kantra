package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/apex/log"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"golang.org/x/exp/maps"
)

type windupShimCommand struct {
	input  []string
	output string

	log logr.Logger
}

func NewWindupShimCommand(log logr.Logger) *cobra.Command {
	windupShimCmd := &windupShimCommand{
		log: log,
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
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			err := windupShimCmd.Run(cmd.Context())
			if err != nil {
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
		return err
	}
	if !outputStat.IsDir() {
		log.Errorf("output path %s is not a directory", w.output)
		return err
	}
	if w.input == nil || len(w.input) == 0 {
		return fmt.Errorf("input for rule file or directory must not be empty")
	}
	return nil
}

func (w *windupShimCommand) getRulesVolumes(tempRuleDir string) (map[string]string, error) {
	rulesVolumes := make(map[string]string)
	mountTempDir := false
	err := os.Mkdir(tempRuleDir, os.ModePerm)
	if err != nil {
		return nil, err
	}
	for _, r := range w.input {
		stat, err := os.Stat(r)
		if err != nil {
			w.log.V(5).Error(err, "failed to stat rules")
			return nil, err
		}
		// move xml rule files from user into dir to mount
		if !stat.IsDir() {
			mountTempDir = true
			xmlFileName := filepath.Base(r)
			destFile := filepath.Join(tempRuleDir, xmlFileName)
			err := copyFileContents(r, destFile)
			if err != nil {
				w.log.Error(err, "failed to move rules file from source to destination", "src", r, "dest", destFile)
				return nil, err
			}
		} else {
			rulesVolumes[r] = filepath.Join(XMLRulePath, filepath.Base(r))
		}
	}
	if mountTempDir {
		rulesVolumes[tempRuleDir] = XMLRulePath
	}
	return rulesVolumes, nil
}

func (w *windupShimCommand) Run(ctx context.Context) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	tempXMLRulesDir := filepath.Join(wd, "xmlrules")
	volumes := map[string]string{
		w.output: ShimOutputPath,
	}
	ruleVols, err := w.getRulesVolumes(tempXMLRulesDir)
	if err != nil {
		w.log.V(5).Error(err, "failed to get xml rules for conversion")
		return err
	}
	maps.Copy(volumes, ruleVols)

	args := []string{"convert",
		fmt.Sprintf("--outputdir=%v", ShimOutputPath),
		XMLRulePath,
	}
	err = NewContainer().Run(
		ctx,
		WithVolumes(volumes),
		WithEntrypointArgs(args...),
		WithEntrypointBin("/usr/local/bin/windup-shim"),
	)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempXMLRulesDir)
	return nil
}
