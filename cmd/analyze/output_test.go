package analyze

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	outputv1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
)

func Test_analyzeCommand_CreateJSONOutput_SkippedWhenDisabled(t *testing.T) {
	a := &analyzeCommand{
		jsonOutput:            false,
		AnalyzeCommandContext: AnalyzeCommandContext{log: testr.NewWithOptions(t, testr.Options{Verbosity: 5})},
	}

	err := a.CreateJSONOutput()
	require.NoError(t, err, "CreateJSONOutput() should return nil when jsonOutput=false")
}

func Test_analyzeCommand_CreateJSONOutput_ConvertsOutputYAMLToJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "output-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a sample output.yaml
	rulesets := []outputv1.RuleSet{
		{
			Name:        "test-ruleset",
			Description: "A test ruleset",
		},
	}
	yamlData, err := yaml.Marshal(rulesets)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "output.yaml"), yamlData, 0644)
	require.NoError(t, err)

	// Create a sample dependencies.yaml
	deps := []outputv1.DepsFlatItem{
		{
			Provider: "java",
			FileURI:  "file:///test/pom.xml",
		},
	}
	depsYaml, err := yaml.Marshal(deps)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "dependencies.yaml"), depsYaml, 0644)
	require.NoError(t, err)

	a := &analyzeCommand{
		jsonOutput:            true,
		output:                tmpDir,
		mode:                  string(provider.FullAnalysisMode),
		AnalyzeCommandContext: AnalyzeCommandContext{log: testr.NewWithOptions(t, testr.Options{Verbosity: 5})},
	}

	err = a.CreateJSONOutput()
	require.NoError(t, err)

	// Verify output.json was created and is valid JSON
	outputJSON, err := os.ReadFile(filepath.Join(tmpDir, "output.json"))
	require.NoError(t, err, "output.json was not created")

	var parsedOutput []outputv1.RuleSet
	err = json.Unmarshal(outputJSON, &parsedOutput)
	require.NoError(t, err, "output.json is not valid JSON")
	require.Len(t, parsedOutput, 1)
	assert.Equal(t, "test-ruleset", parsedOutput[0].Name)

	// Verify dependencies.json was created and is valid JSON
	depsJSON, err := os.ReadFile(filepath.Join(tmpDir, "dependencies.json"))
	require.NoError(t, err, "dependencies.json was not created")

	var parsedDeps []outputv1.DepsFlatItem
	err = json.Unmarshal(depsJSON, &parsedDeps)
	require.NoError(t, err, "dependencies.json is not valid JSON")
}

func Test_analyzeCommand_CreateJSONOutput_SkipsDepsForSourceOnlyMode(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "output-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create output.yaml only, no dependencies.yaml
	rulesets := []outputv1.RuleSet{
		{Name: "test"},
	}
	yamlData, err := yaml.Marshal(rulesets)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "output.yaml"), yamlData, 0644)
	require.NoError(t, err)

	a := &analyzeCommand{
		jsonOutput:            true,
		output:                tmpDir,
		mode:                  string(provider.SourceOnlyAnalysisMode),
		AnalyzeCommandContext: AnalyzeCommandContext{log: testr.NewWithOptions(t, testr.Options{Verbosity: 5})},
	}

	err = a.CreateJSONOutput()
	require.NoError(t, err)

	// output.json should exist
	assert.FileExists(t, filepath.Join(tmpDir, "output.json"))

	// dependencies.json should NOT exist (source-only mode)
	_, err = os.Stat(filepath.Join(tmpDir, "dependencies.json"))
	assert.True(t, os.IsNotExist(err), "dependencies.json should NOT be created in source-only mode")
}

func Test_analyzeCommand_CreateJSONOutput_SkipsDepsWhenNoDepsFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "output-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create output.yaml only
	rulesets := []outputv1.RuleSet{{Name: "test"}}
	yamlData, _ := yaml.Marshal(rulesets)
	os.WriteFile(filepath.Join(tmpDir, "output.yaml"), yamlData, 0644)

	a := &analyzeCommand{
		jsonOutput:            true,
		output:                tmpDir,
		mode:                  string(provider.FullAnalysisMode),
		AnalyzeCommandContext: AnalyzeCommandContext{log: testr.NewWithOptions(t, testr.Options{Verbosity: 5})},
	}

	err = a.CreateJSONOutput()
	require.NoError(t, err)

	// dependencies.json should NOT exist when no deps file
	_, err = os.Stat(filepath.Join(tmpDir, "dependencies.json"))
	assert.True(t, os.IsNotExist(err), "dependencies.json should NOT be created when dependencies.yaml is missing")
}

func Test_analyzeCommand_CreateJSONOutput_ErrorOnMissingOutputYaml(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "output-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	a := &analyzeCommand{
		jsonOutput:            true,
		output:                tmpDir,
		AnalyzeCommandContext: AnalyzeCommandContext{log: testr.NewWithOptions(t, testr.Options{Verbosity: 5})},
	}

	err = a.CreateJSONOutput()
	require.Error(t, err, "CreateJSONOutput() should return error when output.yaml is missing")
}
