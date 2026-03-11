package analyze

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/konveyor-ecosystem/kantra/pkg/util"
	outputv1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"gopkg.in/yaml.v2"
)

func (a *analyzeCommand) CreateJSONOutput() error {
	if !a.jsonOutput {
		return nil
	}
	a.log.Info("writing analysis results as json output", "output", a.output)
	outputPath := filepath.Join(a.output, "output.yaml")
	depPath := filepath.Join(a.output, "dependencies.yaml")

	data, err := os.ReadFile(outputPath)
	if err != nil {
		return err
	}
	ruleOutput := &[]outputv1.RuleSet{}
	err = yaml.Unmarshal(data, ruleOutput)
	if err != nil {
		a.log.V(1).Error(err, "failed to unmarshal output yaml")
		return err
	}

	jsonData, err := json.MarshalIndent(ruleOutput, "", "	")
	if err != nil {
		a.log.V(1).Error(err, "failed to marshal output file to json")
		return err
	}
	err = os.WriteFile(filepath.Join(a.output, "output.json"), jsonData, 0644)
	if err != nil {
		a.log.V(1).Error(err, "failed to write json output", "dir", a.output, "file", "output.json")
		return err
	}

	// in case of no dep output
	_, noDepFileErr := os.Stat(filepath.Join(a.output, "dependencies.yaml"))
	if errors.Is(noDepFileErr, os.ErrNotExist) || a.mode == string(provider.SourceOnlyAnalysisMode) {
		a.log.Info("skipping dependency output for json output")
		return nil
	}
	depData, err := os.ReadFile(depPath)
	if err != nil {
		return err
	}
	depOutput := &[]outputv1.DepsFlatItem{}
	err = yaml.Unmarshal(depData, depOutput)
	if err != nil {
		a.log.V(1).Error(err, "failed to unmarshal dependencies yaml")
		return err
	}

	jsonDataDep, err := json.MarshalIndent(depOutput, "", "	")
	if err != nil {
		a.log.V(1).Error(err, "failed to marshal dependencies file to json")
		return err
	}
	err = os.WriteFile(filepath.Join(a.output, "dependencies.json"), jsonDataDep, 0644)
	if err != nil {
		a.log.V(1).Error(err, "failed to write json dependencies output", "dir", a.output, "file", "dependencies.json")
		return err
	}

	return nil
}

func (a *analyzeCommand) moveResults() error {
	outputPath := filepath.Join(a.output, "output.yaml")
	analysisLogFilePath := filepath.Join(a.output, "analysis.log")
	depsPath := filepath.Join(a.output, "dependencies.yaml")
	err := util.CopyFileContents(outputPath, fmt.Sprintf("%s.%s", outputPath, a.inputShortName()))
	if err != nil {
		return err
	}
	err = os.Remove(outputPath)
	if err != nil {
		return err
	}
	err = util.CopyFileContents(analysisLogFilePath, fmt.Sprintf("%s.%s", analysisLogFilePath, a.inputShortName()))
	if err != nil {
		return err
	}
	err = os.Remove(analysisLogFilePath)
	if err != nil {
		return err
	}
	// dependencies.yaml is optional
	_, noDepFileErr := os.Stat(depsPath)
	if noDepFileErr == nil {
		err = util.CopyFileContents(depsPath, fmt.Sprintf("%s.%s", depsPath, a.inputShortName()))
		if err != nil {
			return err
		}
		err = os.Remove(depsPath)
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *analyzeCommand) inputShortName() string {
	return filepath.Base(a.input)
}
