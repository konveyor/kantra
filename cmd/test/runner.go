package test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bombsimon/logrusr/v3"
	"github.com/go-logr/logr"
	kantraprovider "github.com/konveyor-ecosystem/kantra/pkg/provider"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	konveyorAnalyzer "github.com/konveyor/analyzer-lsp/core"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

// defaultTestContextLines matches analyze default --context-lines for incident snippets.
const defaultTestContextLines = 100

// Runner given a list of TestsFile and a TestOptions
// runs the tests, computes and returns results
type Runner interface {
	Run([]TestsFile, TestOptions) ([]Result, error)
}

type TestOptions struct {
	TempDir         string
	LoudOutput      bool
	ProgressPrinter ResultPrinter
	Log             logr.Logger
	Prune           bool
	NoCleanup       bool

	// RunLocal runs Java and builtin in containerless mode on the host (analyze --run-local).
	// Requires ValidateContainerlessProviders to have succeeded.
	RunLocal bool

	// ContainerBinary is the path to the container runtime (podman/docker).
	ContainerBinary string

	// ProviderImages maps provider names to their container images.
	// Example: {"java": "quay.io/konveyor/java-external-provider:latest"}
	ProviderImages map[string]string

	// RunnerImage is the kantra runner container image (used for naming/caching paths in the environment).
	RunnerImage string

	// Version is the kantra version string (for ruleset cache naming).
	Version string

	// Context carries cancellation from the CLI (e.g. root ExecuteContext). If nil,
	// runWithEnvironment uses [context.Background].
	Context context.Context
}

func NewRunner() Runner {
	return defaultRunner{}
}

// defaultRunner runs tests one file at a time
// groups tests within a file by analysisParams
type defaultRunner struct{}

func (r defaultRunner) Run(testFiles []TestsFile, opts TestOptions) ([]Result, error) {
	if opts.Log.GetSink() == nil {
		opts.Log = logr.Discard()
	}

	allResults := []Result{}
	anyFailed := false
	anyErrored := false
	for idx := range testFiles {
		testsFile := testFiles[idx]
		// within a tests file, we group tests by analysis params
		testGroups := groupTestsByAnalysisParams(testsFile.Tests)
		results := []Result{}
		for _, tests := range testGroups {
			tempDir, err := os.MkdirTemp(opts.TempDir, "rules-test-")
			if err != nil {
				err = fmt.Errorf("failed creating temp dir - %w", err)
				opts.Log.Error(err, "failed during execution")
				results = append(results, Result{
					TestsFilePath: testsFile.Path,
					Error:         err})
				continue
			}
			opts.Log.Info("created temporary directory", "dir", tempDir, "tests", testsFile.Path)
			// print analysis logs to a file
			logFile, err := os.OpenFile(filepath.Join(tempDir, "analysis.log"),
				os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
			if err != nil {
				err = fmt.Errorf("failed creating a log file - %w", err)
				opts.Log.Error(err, "failed during execution")
				results = append(results, Result{
					TestsFilePath: testsFile.Path,
					Error:         err})
				logFile.Close()
				continue
			}
			baseLogger := logrus.New()
			baseLogger.SetOutput(logFile)
			baseLogger.SetLevel(logrus.InfoLevel)
			logger := logrusr.New(baseLogger)
			// write rules
			err = ensureRules(testsFile.RulesPath, tempDir, tests)
			if err != nil {
				err = fmt.Errorf("failed writing rules - %w", err)
				opts.Log.Error(err, "failed during execution")
				results = append(results, Result{
					TestsFilePath: testsFile.Path,
					Error:         err})
				logFile.Close()
				continue
			}
			// we already know in this group, all tcs have same params, use any
			analysisParams := tests[0].TestCases[0].AnalysisParams

			// Resolve the data path for this test group.
			// For the moment, use the first provider's dataPath (relative to test file).
			dataPath := ""
			if len(testsFile.Providers) > 0 && testsFile.Providers[0].DataPath != "" {
				dataPath = resolveDataPath(testsFile.Path, testsFile.Providers[0].DataPath)
			}

			reproducerCmd, err := runWithEnvironment(
				logger, opts, testsFile, tempDir, analysisParams, dataPath)
			if err != nil {
				opts.Log.Error(err, "failed during execution")
				results = append(results, Result{
					TestsFilePath: testsFile.Path,
					Error:         err})
				logFile.Close()
				continue
			}

			// Read output from the standard location
			outputPath := filepath.Join(tempDir, "output", "output.yaml")
			content, err := os.ReadFile(outputPath)

			// write reproducer command to a file
			os.WriteFile(filepath.Join(tempDir, "reproducer.sh"), []byte(reproducerCmd), 0755)
			// process output
			outputRulesets := []konveyor.RuleSet{}
			if err != nil {
				results = append(results, Result{
					TestsFilePath: testsFile.Path,
					Error:         fmt.Errorf("failed reading output - %w", err)})
				logFile.Close()
				continue
			}
			err = yaml.Unmarshal(content, &outputRulesets)
			if err != nil {
				results = append(results, Result{
					TestsFilePath: testsFile.Path,
					Error:         fmt.Errorf("failed unmarshaling output %s", outputPath)})
				logFile.Close()
				continue
			}
			anyFailed := false
			groupResults := []Result{}
			for _, test := range tests {
				for _, tc := range test.TestCases {
					result := Result{
						TestsFilePath: testsFile.Path,
						RuleID:        test.RuleID,
						TestCaseName:  tc.Name,
					}
					if len(outputRulesets) > 0 {
						result.FailureReasons = tc.Verify(outputRulesets[0])
					} else {
						result.FailureReasons = []string{"empty output"}
					}
					if len(result.FailureReasons) == 0 {
						result.Passed = true
					} else {
						anyFailed = true
						result.DebugInfo = append(result.DebugInfo,
							fmt.Sprintf("find debug data in %s", tempDir))
					}
					groupResults = append(groupResults, result)
				}
			}
			results = append(results, groupResults...)
			if !anyFailed && !opts.NoCleanup {
				os.RemoveAll(tempDir)
			}
			logFile.Close()
		}
		// print progress
		if opts.ProgressPrinter != nil {
			opts.ProgressPrinter(os.Stdout, results)
		}
		// result
		for _, r := range results {
			if r.Error != nil {
				anyErrored = true
			}
			if !r.Passed {
				anyFailed = true
			}
		}
		allResults = append(allResults, results...)
	}
	// sorting for stability of unit tests
	defer sort.Slice(allResults, func(i, j int) bool {
		return strings.Compare(allResults[i].RuleID, allResults[j].RuleID) > 0
	})

	if anyErrored {
		return allResults, fmt.Errorf("failed to execute one or more tests")
	}
	if anyFailed {
		return allResults, fmt.Errorf("one or more tests failed")
	}
	return allResults, nil
}

func runWithEnvironment(logger logr.Logger, opts TestOptions, testsFile TestsFile, dir string, analysisParams AnalysisParams, input string) (string, error) {
	outputDir := filepath.Join(dir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	var providerInfos []kantraprovider.ProviderInfo
	if !opts.RunLocal {
		providerInfos = buildTestProviderInfos(testsFile.Providers, opts.ProviderImages)
	}

	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	env, err := newEnvironmentForRuleTests(opts, RuleTestRunParams{
		Input:        input,
		AnalysisMode: string(analysisParams.Mode),
		Cleanup:      !opts.NoCleanup,
		ContextLines: defaultTestContextLines,
		Providers:    providerInfos,
		WorkDir:      dir,
	})
	if err != nil {
		return "", err
	}
	if err := env.Start(ctx); err != nil {
		return "", fmt.Errorf("failed to start environment: %w", err)
	}
	defer env.Stop(ctx)

	providerConfigs := env.ProviderConfigs()
	rulesPath := filepath.Join(dir, "rules.yaml")

	analyzerOpts := []konveyorAnalyzer.AnalyzerOption{
		konveyorAnalyzer.WithProviderConfigs(providerConfigs),
		konveyorAnalyzer.WithRuleFilepaths([]string{rulesPath}),
		konveyorAnalyzer.WithLogger(logger),
		konveyorAnalyzer.WithContext(ctx),
		konveyorAnalyzer.WithContextLinesLimit(defaultTestContextLines),
		konveyorAnalyzer.WithDependencyRulesDisabled(),
	}
	// Match cmd/analyze/run.go: path mappings apply to binary file inputs (jar/war/ear/class),
	// not to "full" vs "source-only" analysis mode.
	extra := env.ExtraOptions(ctx, isBinaryAnalysisInput(input))
	if len(extra.PathMappings) > 0 {
		analyzerOpts = append(analyzerOpts, konveyorAnalyzer.WithPathMappings(extra.PathMappings))
	}
	if extra.IgnoreAdditionalBuiltinConfigs {
		analyzerOpts = append(analyzerOpts, konveyorAnalyzer.WithIgnoreAdditionalBuiltinConfigs(true))
	}
	if analysisParams.DepLabelSelector != "" {
		analyzerOpts = append(analyzerOpts,
			konveyorAnalyzer.WithDepLabelSelector(analysisParams.DepLabelSelector))
	}

	return runAnalyzerAndWriteOutput(ctx, logger, opts, env, analyzerOpts, outputDir, dir, analysisParams, input)
}

func runAnalyzerAndWriteOutput(
	ctx context.Context,
	logger logr.Logger,
	opts TestOptions,
	env kantraprovider.Environment,
	analyzerOpts []konveyorAnalyzer.AnalyzerOption,
	outputDir, dir string,
	analysisParams AnalysisParams,
	input string,
) (string, error) {
	anlzr, err := konveyorAnalyzer.NewAnalyzer(analyzerOpts...)
	if err != nil {
		return "", fmt.Errorf("failed to create analyzer: %w", err)
	}
	defer anlzr.Stop()

	if _, err := anlzr.ParseRules(); err != nil {
		return "", fmt.Errorf("failed to parse rules: %w", err)
	}

	if err := anlzr.ProviderStart(); err != nil {
		return "", fmt.Errorf("failed to start providers: %w", err)
	}

	rulesets := anlzr.Run()

	if err := env.PostAnalysis(ctx); err != nil {
		opts.Log.Error(err, "post-analysis tasks failed")
	}

	sort.SliceStable(rulesets, func(i, j int) bool {
		return rulesets[i].Name < rulesets[j].Name
	})
	b, err := yaml.Marshal(rulesets)
	if err != nil {
		return "", fmt.Errorf("failed to marshal analysis output: %w", err)
	}
	outputPath := filepath.Join(outputDir, "output.yaml")
	if err := os.WriteFile(outputPath, b, 0644); err != nil {
		return "", fmt.Errorf("failed to write output.yaml: %w", err)
	}

	reproducerCmd := buildReproducerCmd(opts.RunLocal, input, outputDir,
		filepath.Join(dir, "rules.yaml"), analysisParams)
	return reproducerCmd, nil
}

// buildTestProviderInfos converts the test file's provider configs into
// ProviderInfo structs using the image map from TestOptions.
func buildTestProviderInfos(providers []ProviderConfig, images map[string]string) []kantraprovider.ProviderInfo {
	infos := make([]kantraprovider.ProviderInfo, 0, len(providers))
	for _, p := range providers {
		image := ""
		if images != nil {
			image = images[p.Name]
		}
		if image == "" {
			// Try well-known default images
			image = defaultProviderImage(p.Name)
		}
		if image == "" {
			continue
		}
		infos = append(infos, kantraprovider.ProviderInfo{
			Name:  p.Name,
			Image: image,
		})
	}
	return infos
}

// defaultProviderImage returns a fallback container image for well-known
// provider names. This is used when TestOptions.ProviderImages doesn't
// have an entry for a provider.
func defaultProviderImage(name string) string {
	switch name {
	case util.JavaProvider:
		return "quay.io/konveyor/java-external-provider:latest"
	case util.GoProvider, util.PythonProvider, util.NodeJSProvider:
		return "quay.io/konveyor/generic-external-provider:latest"
	case util.CsharpProvider:
		return "quay.io/konveyor/c-sharp-provider:latest"
	default:
		// "builtin" and other providers don't have separate container images
		return ""
	}
}

// buildReproducerCmd generates a kantra analyze command for debugging test failures.
func buildReproducerCmd(runLocal bool, input string, outputDir string, rulesPath string, params AnalysisParams) string {
	args := []string{"analyze", "--skip-static-report"}
	if runLocal {
		args = append(args, "--run-local=true")
	} else {
		args = append(args, "--run-local=false")
	}
	args = append(args,
		"--input", input,
		"--output", outputDir,
		"--rules", rulesPath,
		"--overwrite", "--enable-default-rulesets=false",
	)
	if params.DepLabelSelector != "" {
		args = append(args, "--label-selector", params.DepLabelSelector)
	}
	return fmt.Sprintf("kantra %s", strings.Join(args, " "))
}

func ensureRules(rulesPath string, tempDirPath string, group []Test) error {
	allRules := []map[string]interface{}{}
	neededRules := map[string]interface{}{}
	for _, test := range group {
		neededRules[test.RuleID] = nil
	}
	content, err := os.ReadFile(rulesPath)
	if err != nil {
		return fmt.Errorf("failed to read rules file %s (%w)", rulesPath, err)
	}
	err = yaml.Unmarshal(content, &allRules)
	if err != nil {
		return fmt.Errorf("error unmarshaling rules at path %s (%w)", rulesPath, err)
	}
	foundRules := []map[string]interface{}{}
	for neededRule := range neededRules {
		found := false
		for _, foundRule := range allRules {
			if foundRule["ruleID"] == neededRule {
				found = true
				foundRules = append(foundRules, foundRule)
				break
			}
		}
		if !found {
			return fmt.Errorf("rule %s not found in file %s", neededRule, rulesPath)
		}
	}

	content, err = yaml.Marshal(foundRules)
	if err != nil {
		return fmt.Errorf("failed marshaling rules - %w", err)
	}
	err = os.WriteFile(filepath.Join(tempDirPath, "rules.yaml"), content, 0644)
	if err != nil {
		return fmt.Errorf("failed writing rules file - %w", err)
	}
	return nil
}

// resolveDataPath resolves a dataPath relative to the test file's directory,
// or returns it as-is if it is already an absolute path.
func resolveDataPath(testsFilePath string, dataPath string) string {
	if filepath.IsAbs(dataPath) {
		return dataPath
	}
	return filepath.Join(filepath.Dir(testsFilePath), filepath.Clean(dataPath))
}

// isBinaryAnalysisInput matches analyze binary detection so hybrid ExtraOptions
// path mappings align with `kantra analyze --run-local=false`.
func isBinaryAnalysisInput(input string) bool {
	info, err := os.Stat(input)
	if err != nil || info.IsDir() {
		return false
	}
	ext := filepath.Ext(input)
	return ext == util.JavaArchive || ext == util.WebArchive ||
		ext == util.EnterpriseArchive || ext == util.ClassFile
}

func groupTestsByAnalysisParams(tests []Test) [][]Test {
	grouped := map[string]map[string]*Test{}
	for _, t := range tests {
		testKey := t.RuleID
		for _, tc := range t.TestCases {
			paramsKey := fmt.Sprintf("%s-%s",
				tc.AnalysisParams.DepLabelSelector, tc.AnalysisParams.Mode)
			if _, ok := grouped[paramsKey]; !ok {
				grouped[paramsKey] = map[string]*Test{}
			}
			if _, ok := grouped[paramsKey][testKey]; !ok {
				grouped[paramsKey][testKey] = &Test{
					RuleID:    t.RuleID,
					TestCases: []TestCase{},
				}
			}
			grouped[paramsKey][testKey].TestCases = append(
				grouped[paramsKey][testKey].TestCases, tc)
		}
	}
	groupedList := [][]Test{}
	for _, tests := range grouped {
		currentGroup := []Test{}
		for _, v := range tests {
			currentGroup = append(currentGroup, *v)
		}
		groupedList = append(groupedList, currentGroup)
	}
	return groupedList
}
