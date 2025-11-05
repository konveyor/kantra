package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"runtime"

	kantraProvider "github.com/konveyor-ecosystem/kantra/pkg/provider"

	"gopkg.in/yaml.v2"

	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/devfile/alizer/pkg/apis/model"
	"github.com/devfile/alizer/pkg/apis/recognizer"
	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/cmd/internal/hiddenfile"
	"github.com/konveyor-ecosystem/kantra/pkg/container"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	outputv1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/phayes/freeport"
	"go.lsp.dev/uri"

	"github.com/spf13/cobra"
	"golang.org/x/exp/maps"
)

// TODO add network and volume w/ interface
type ProviderInit struct {
	port  int
	image string
	// used for failed provider container retry attempts
	isRunning     bool
	containerName string
	provider      kantraProvider.Provider
}

// kantra analyze flags
type analyzeCommand struct {
	listSources              bool
	listTargets              bool
	listProviders            bool
	listLanguages            bool
	skipStaticReport         bool
	analyzeKnownLibraries    bool
	jsonOutput               bool
	overwrite                bool
	bulk                     bool
	mavenSettingsFile        string
	sources                  []string
	targets                  []string
	labelSelector            string
	input                    string
	output                   string
	mode                     string
	noDepRules               bool
	rules                    []string
	tempRuleDir              string
	jaegerEndpoint           string
	enableDefaultRulesets    bool
	httpProxy                string
	httpsProxy               string
	noProxy                  string
	contextLines             int
	incidentSelector         string
	depFolders               []string
	overrideProviderSettings string
	provider                 []string
	logLevel                 *uint32
	cleanup                  bool
	runLocal                 bool
	disableMavenSearch       bool
	AnalyzeCommandContext
}

// analyzeCmd represents the analyze command
func NewAnalyzeCmd(log logr.Logger) *cobra.Command {
	analyzeCmd := &analyzeCommand{
		cleanup: true,
	}
	analyzeCmd.log = log

	analyzeCommand := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze application source code",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// TODO (pgaikwad): this is nasty
			if !cmd.Flags().Lookup("list-sources").Changed &&
				!cmd.Flags().Lookup("list-targets").Changed &&
				!cmd.Flags().Lookup("list-providers").Changed &&
				!cmd.Flags().Lookup("list-languages").Changed {
				cmd.MarkFlagRequired("input")
				cmd.MarkFlagRequired("output")
				if err := cmd.ValidateRequiredFlags(); err != nil {
					return err
				}
			}
			if cmd.Flags().Lookup("list-languages").Changed {
				cmd.MarkFlagRequired("input")
			}
			if analyzeCmd.runLocal {
				err := analyzeCmd.setKantraDir()
				if err != nil {
					analyzeCmd.log.Error(err, "unable to get analyze reqs")
					return err
				}
			}
			err := analyzeCmd.Validate(cmd.Context())
			if err != nil {
				log.Error(err, "failed to validate flags")
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if val, err := cmd.Flags().GetUint32(logLevelFlag); err == nil {
				analyzeCmd.logLevel = &val
			}
			if val, err := cmd.Flags().GetBool(noCleanupFlag); err == nil {
				analyzeCmd.cleanup = !val
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			if analyzeCmd.listProviders {
				analyzeCmd.ListAllProviders()
				return nil
			}

			// skip container mode check
			if analyzeCmd.listLanguages {
				analyzeCmd.runLocal = false
			}

			if analyzeCmd.overrideProviderSettings == "" {
				if analyzeCmd.listSources || analyzeCmd.listTargets {
					// list sources/targets in containerless mode
					if analyzeCmd.runLocal {
						err := analyzeCmd.listLabelsContainerless(ctx)
						if err != nil {
							analyzeCmd.log.Error(err, "failed to list rule labels")
							return err
						}
						return nil
					}
					// list sources/targets in container mode
					err := analyzeCmd.ListLabels(cmd.Context())
					if err != nil {
						log.Error(err, "failed to list rule labels")
						return err
					}
					return nil
				}
				if analyzeCmd.providersMap == nil {
					analyzeCmd.providersMap = make(map[string]ProviderInit)
				}
				languages, err := recognizer.Analyze(analyzeCmd.input)
				if err != nil {
					log.Error(err, "Failed to determine languages for input")
					return err
				}
				if analyzeCmd.listLanguages {
					// for binaries, assume Java application
					if analyzeCmd.isFileInput {
						fmt.Fprintln(os.Stdout, "found languages for input application:", util.JavaProvider)
						return nil
					}
					err := listLanguages(languages, analyzeCmd.input)
					if err != nil {
						return err
					}
					return nil
				}

				foundProviders := []string{}
				// file input means a binary was given which only the java provider can use
				if analyzeCmd.isFileInput {
					foundProviders = append(foundProviders, util.JavaProvider)
				} else {
					foundProviders, err = analyzeCmd.setProviders(analyzeCmd.provider, languages, foundProviders)
					if err != nil {
						log.Error(err, "failed to set provider info")
						return err
					}
					err = analyzeCmd.validateProviders(foundProviders)
					if err != nil {
						return err
					}
				}

				// default to run container mode if no Java provider found
				if len(foundProviders) > 0 && !slices.Contains(foundProviders, util.JavaProvider) {
					analyzeCmd.runLocal = false
				}

				// ***** RUN CONTAINERLESS MODE *****
				if analyzeCmd.runLocal {
					log.Info("\n --run-local set. running analysis in containerless mode")
					if analyzeCmd.listSources || analyzeCmd.listTargets {
						err := analyzeCmd.listLabelsContainerless(ctx)
						if err != nil {
							analyzeCmd.log.Error(err, "failed to list rule labels")
							return err
						}
						return nil
					}
					cmdCtx, cancelFunc := context.WithCancel(cmd.Context())
					err := analyzeCmd.RunAnalysisContainerless(cmdCtx)
					defer cancelFunc()
					if err != nil {
						return err
					}
					return nil
				}

				// ******* RUN HYBRID MODE ******
				log.Info("--run-local set to false. Running analysis in hybrid mode")
				if len(foundProviders) > 0 && slices.Contains(foundProviders, util.DotnetFrameworkProvider) {
					log.Info(".Net framework provider found, running windows analysis. Otherwise, set --provider")

					return analyzeCmd.analyzeDotnetFramework(ctx)
				}

				// default rulesets are only java rules
				// may want to change this in the future
				if len(foundProviders) > 0 && len(analyzeCmd.rules) == 0 && !slices.Contains(foundProviders, util.JavaProvider) {
					return fmt.Errorf("No providers found with default rules. Use --rules option")
				}

				// alizer does not detect certain files such as xml
				// in this case, we can first check for a java project
				// if not found, only start builtin provider in hybrid mode
				if len(foundProviders) == 0 {
					foundJava, err := analyzeCmd.detectJavaProviderFallback()
					if err != nil {
						return err
					}
					if foundJava {
						foundProviders = append(foundProviders, util.JavaProvider)
					}
					// If no providers found, we'll run builtin-only in hybrid mode
					// (providersMap will be empty, hybrid mode handles this)
				}

				err = analyzeCmd.setProviderInitInfo(foundProviders)
				if err != nil {
					log.Error(err, "failed to set provider init info")
					return err
				}
				// defer cleaning created resources here instead of PostRun
				// if Run returns an error, PostRun does not run
				defer func() {
					// start other context here to cleanup in case of program interrupt
					if err := analyzeCmd.CleanAnalysisResources(context.TODO()); err != nil {
						log.Error(err, "failed to clean temporary directories")
					}
				}()
				// Run hybrid mode analysis (analyzer on host, providers in containers)
				cmdCtx, cancelFunc := context.WithCancel(ctx)
				err = analyzeCmd.RunAnalysisHybrid(cmdCtx)
				defer cancelFunc()
				if err != nil {
					log.Error(err, "failed to run hybrid analysis")
					return err
				}
			} else {
				err := analyzeCmd.RunAnalysisOverrideProviderSettings(ctx)
				if err != nil {
					log.Error(err, "failed to run analysis")
					return err
				}
			}
			err := analyzeCmd.CreateJSONOutput()
			if err != nil {
				log.Error(err, "failed to create json output file")
				return err
			}

			err = analyzeCmd.GenerateStaticReport(ctx)
			if err != nil {
				log.Error(err, "failed to generate static report")
				return err
			}

			return nil
		},
	}
	analyzeCommand.Flags().BoolVar(&analyzeCmd.listSources, "list-sources", false, "list rules for available migration sources")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.listTargets, "list-targets", false, "list rules for available migration targets")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.listProviders, "list-providers", false, "list available supported providers")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.listLanguages, "list-languages", false, "list found application language(s)")
	analyzeCommand.Flags().StringArrayVarP(&analyzeCmd.sources, "source", "s", []string{}, "source technology to consider for analysis. Use multiple times for additional sources: --source <source1> --source <source2> ...")
	analyzeCommand.Flags().StringArrayVarP(&analyzeCmd.targets, "target", "t", []string{}, "target technology to consider for analysis. Use multiple times for additional targets: --target <target1> --target <target2> ...")
	analyzeCommand.Flags().StringVarP(&analyzeCmd.labelSelector, "label-selector", "l", "", "run rules based on specified label selector expression")
	analyzeCommand.Flags().StringArrayVar(&analyzeCmd.rules, "rules", []string{}, "filename or directory containing rule files. Use multiple times for additional rules: --rules <rule1> --rules <rule2> ...")
	analyzeCommand.Flags().StringVarP(&analyzeCmd.input, "input", "i", "", "path to application source code or a binary")
	analyzeCommand.Flags().StringVarP(&analyzeCmd.output, "output", "o", "", "path to the directory for analysis output")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.skipStaticReport, "skip-static-report", false, "do not generate static report")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.analyzeKnownLibraries, "analyze-known-libraries", false, "analyze known open-source libraries")
	analyzeCommand.Flags().StringVar(&analyzeCmd.mavenSettingsFile, "maven-settings", "", "path to a custom maven settings file to use")
	analyzeCommand.Flags().StringVarP(&analyzeCmd.mode, "mode", "m", string(provider.FullAnalysisMode), "analysis mode. Must be one of 'full' (source + dependencies) or 'source-only'")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.noDepRules, "no-dependency-rules", false, "disable dependency analysis rules")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.jsonOutput, "json-output", false, "create analysis and dependency output as json")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.overwrite, "overwrite", false, "overwrite output directory")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.bulk, "bulk", false, "running multiple analyze commands in bulk will result to combined static report")
	analyzeCommand.Flags().StringVar(&analyzeCmd.jaegerEndpoint, "jaeger-endpoint", "", "jaeger endpoint to collect traces")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.enableDefaultRulesets, "enable-default-rulesets", true, "run default rulesets with analysis")
	analyzeCommand.Flags().StringVar(&analyzeCmd.httpProxy, "http-proxy", util.LoadEnvInsensitive("http_proxy"), "HTTP proxy string URL")
	analyzeCommand.Flags().StringVar(&analyzeCmd.httpsProxy, "https-proxy", util.LoadEnvInsensitive("https_proxy"), "HTTPS proxy string URL")
	analyzeCommand.Flags().StringVar(&analyzeCmd.noProxy, "no-proxy", util.LoadEnvInsensitive("no_proxy"), "proxy excluded URLs (relevant only with proxy)")
	analyzeCommand.Flags().IntVar(&analyzeCmd.contextLines, "context-lines", 100, "number of lines of source code to include in the output for each incident")
	analyzeCommand.Flags().StringVar(&analyzeCmd.incidentSelector, "incident-selector", "", "an expression to select incidents based on custom variables. ex: (!package=io.konveyor.demo.config-utils)")
	analyzeCommand.Flags().StringArrayVarP(&analyzeCmd.depFolders, "dependency-folders", "d", []string{}, "directory for dependencies")
	analyzeCommand.Flags().StringVar(&analyzeCmd.overrideProviderSettings, "override-provider-settings", "", "override the provider settings, the analysis pod will be run on the host network and no providers will be started up")
	analyzeCommand.Flags().StringArrayVar(&analyzeCmd.provider, "provider", []string{}, "specify which provider(s) to run")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.runLocal, "run-local", true, "run Java analysis in containerless mode")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.disableMavenSearch, "disable-maven-search", false, "disable maven search for dependencies")
	return analyzeCommand
}

func (a *analyzeCommand) Validate(ctx context.Context) error {
	if a.listSources || a.listTargets || a.listProviders {
		return nil
	}

	if a.listLanguages {
		stat, err := os.Stat(a.input)
		if err != nil {
			return fmt.Errorf("%w failed to stat input path %s", err, a.input)
		}
		if !stat.Mode().IsDir() {
			a.isFileInput = true
		}
		return nil
	}

	if a.labelSelector != "" && (len(a.sources) > 0 || len(a.targets) > 0) {
		return fmt.Errorf("must not specify label-selector and sources or targets")
	}

	for _, rulePath := range a.rules {
		if _, err := os.Stat(rulePath); rulePath != "" && err != nil {
			return fmt.Errorf("%w failed to stat rules at path %s", err, rulePath)
		}
		if rulePath != "" {
			if err := a.validateRulesPath(rulePath); err != nil {
				return err
			}
		}
	}
	// Validate source labels
	// allow custom sources/targets if custom rules are set
	if len(a.sources) > 0 {
		var sourcesRaw bytes.Buffer
		if a.runLocal {
			a.fetchLabelsContainerless(ctx, true, false, &sourcesRaw)
		} else {
			a.fetchLabels(ctx, true, false, &sourcesRaw)
		}
		knownSources := strings.Split(sourcesRaw.String(), "\n")
		for _, source := range a.sources {
			found := false
			for _, knownSource := range knownSources {
				if source == knownSource {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("unknown source: \"%s\"", source)
			}
		}
	}
	// Validate target labels
	if len(a.targets) > 0 {
		var targetRaw bytes.Buffer
		if a.runLocal {
			a.fetchLabelsContainerless(ctx, false, true, &targetRaw)
		} else {
			a.fetchLabels(ctx, false, true, &targetRaw)
		}
		knownTargets := strings.Split(targetRaw.String(), "\n")
		for _, source := range a.targets {
			found := false
			for _, knownTarget := range knownTargets {
				if source == knownTarget {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("unknown target: \"%s\"", source)
			}
		}
	}

	if a.overrideProviderSettings != "" {
		stat, err := os.Stat(a.overrideProviderSettings)
		if err != nil {
			return fmt.Errorf("%w failed to stat overriden provider settings %s", err, a.overrideProviderSettings)
		}
		if stat.IsDir() {
			return fmt.Errorf("provider settings must be a file")
		}
		a.overrideProviderSettings, err = filepath.Abs(a.overrideProviderSettings)
		if err != nil {
			return fmt.Errorf("%w failed to get absolute path for override provider settings %s", err, a.overrideProviderSettings)
		}
	} else if a.input != "" {
		// do not allow multiple input applications
		inputNum := 0
		for _, arg := range os.Args {
			if arg == "-i" || strings.Contains(arg, "--input") {
				inputNum += 1
				if inputNum > 1 {
					return fmt.Errorf("must specify only one input source")
				}
			}
		}
		stat, err := os.Stat(a.input)
		if err != nil {
			return fmt.Errorf("%w failed to stat input path %s", err, a.input)
		}
		// when input isn't a dir, it's pointing to a binary
		// we need abs path to mount the file correctly
		if !stat.Mode().IsDir() {
			// validate file types
			fileExt := filepath.Ext(a.input)
			switch fileExt {
			case util.JavaArchive, util.WebArchive, util.EnterpriseArchive, util.ClassFile:
				a.log.V(5).Info("valid java file found")
			default:
				return fmt.Errorf("invalid file type %v", fileExt)
			}
			a.input, err = filepath.Abs(a.input)
			if err != nil {
				return fmt.Errorf("%w failed to get absolute path for input file %s", err, a.input)
			}
			// make sure we mount a file and not a dir
			util.SourceMountPath = path.Join(util.SourceMountPath, filepath.Base(a.input))
			a.isFileInput = true
		}
	}
	err := a.CheckOverwriteOutput()
	if err != nil {
		return err
	}
	stat, err := os.Stat(a.output)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = os.MkdirAll(a.output, os.ModePerm)
			if err != nil {
				return fmt.Errorf("%w failed to create output dir %s", err, a.output)
			}
		} else {
			return fmt.Errorf("failed to stat output directory %s", a.output)
		}
	}
	if stat != nil && !stat.IsDir() {
		return fmt.Errorf("output path %s is not a directory", a.output)
	}
	if len(a.depFolders) != 0 {
		for i := range a.depFolders {
			stat, err := os.Stat(a.depFolders[i])
			if err != nil {
				return fmt.Errorf("%w failed to stat dependency folder %v", err, a.depFolders[i])
			}
			if stat != nil && !stat.IsDir() {
				return fmt.Errorf("depdendecy folder %v is not a directory", a.depFolders[i])
			}
		}
	}
	if a.mode != string(provider.FullAnalysisMode) &&
		a.mode != string(provider.SourceOnlyAnalysisMode) {
		return fmt.Errorf("mode must be one of 'full' or 'source-only'")
	}
	if _, err := os.Stat(a.mavenSettingsFile); a.mavenSettingsFile != "" && err != nil {
		return fmt.Errorf("%w failed to stat maven settings file at path %s", err, a.mavenSettingsFile)
	}
	// try to get abs path, if not, continue with relative path
	if absPath, err := filepath.Abs(a.output); err == nil {
		a.output = absPath
	}
	if absPath, err := filepath.Abs(a.input); err == nil {
		a.input = absPath
	}
	if absPath, err := filepath.Abs(a.mavenSettingsFile); a.mavenSettingsFile != "" && err == nil {
		a.mavenSettingsFile = absPath
	}
	if !a.enableDefaultRulesets && len(a.rules) == 0 {
		return fmt.Errorf("must specify rules if default rulesets are not enabled")
	}
	return nil
}

func (a *analyzeCommand) CheckOverwriteOutput() error {
	// default overwrite to false so check for already existing output dir
	stat, err := os.Stat(a.output)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if a.bulk {
		lockStat, _ := os.Stat(filepath.Join(a.output, "analysis.log"))
		if lockStat != nil {
			return fmt.Errorf("output dir %v already contains 'analysis.log', it was used for single application analysis or there is running --bulk analysis, try another output dir", a.output)
		}
		sameInputStat, _ := os.Stat(fmt.Sprintf("%s.%s", filepath.Join(a.output, "output.yaml"), a.inputShortName()))
		if sameInputStat != nil {
			return fmt.Errorf("output dir %v already contains analysis report for provided input '%v', try another input or change output dir", a.output, a.inputShortName())
		}
	} else {
		if !a.overwrite && stat != nil {
			return fmt.Errorf("output dir %v already exists and --overwrite not set", a.output)
		}
	}
	if a.overwrite && stat != nil {
		err := os.RemoveAll(a.output)
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *analyzeCommand) validateProviders(providers []string) error {
	validProvs := []string{
		util.JavaProvider,
		util.PythonProvider,
		util.GoProvider,
		util.NodeJSProvider,
		util.DotnetProvider,
		util.DotnetFrameworkProvider,
	}
	for _, prov := range providers {
		//validate other providers
		if !slices.Contains(validProvs, prov) {
			return fmt.Errorf("provider %v not supported. Use --providerOverride or --provider option", prov)
		}
	}
	return nil
}

func (a *analyzeCommand) validateRulesPath(rulePath string) error {
	stat, err := os.Stat(rulePath)
	if err != nil {
		return err
	}
	if stat.IsDir() {
		return filepath.WalkDir(rulePath, func(path string, d fs.DirEntry, err error) error {
			if d.IsDir() {
				return nil
			}
			ext := filepath.Ext(path)
			if ext != ".yaml" && ext != ".yml" {
				a.log.Error(fmt.Errorf("rule must be a yaml file %s", path), "skipping invalid rule")
			}
			return nil
		})
	} else {
		ext := filepath.Ext(rulePath)
		if ext != ".yaml" && ext != ".yml" {
			a.log.Error(fmt.Errorf("rule must be a yaml file %s", rulePath), "skipping invalid rule")
		}
	}
	return nil
}

func (a *analyzeCommand) needDefaultRules() {
	needDefaultRulesets := false
	for prov := range a.providersMap {
		// default rulesets may have been disabled by user
		if prov == util.JavaProvider && a.enableDefaultRulesets {
			needDefaultRulesets = true
			break
		}
	}
	if !needDefaultRulesets {
		a.enableDefaultRulesets = false
	}
}

func (a *analyzeCommand) ListAllProviders() {
	supportedProvsContainer := []string{
		"java",
		"python",
		"go",
		"dotnet",
		"nodejs",
	}
	supportedProvsContainerless := []string{
		"java",
	}
	fmt.Println("container analysis supported providers:")
	for _, prov := range supportedProvsContainer {
		fmt.Fprintln(os.Stdout, prov)
	}
	fmt.Println("containerless analysis supported providers (default):")
	for _, prov := range supportedProvsContainerless {
		fmt.Fprintln(os.Stdout, prov)
	}
}

func (a *analyzeCommand) ListLabels(ctx context.Context) error {
	return a.fetchLabels(ctx, a.listSources, a.listTargets, os.Stdout)
}

func (a *analyzeCommand) fetchLabels(ctx context.Context, listSources, listTargets bool, out io.Writer) error {
	// reserved labels
	sourceLabel := outputv1.SourceTechnologyLabel
	targetLabel := outputv1.TargetTechnologyLabel
	runMode := "RUN_MODE"
	runModeContainer := "container"
	rulePath := "RULE_PATH"
	customRulePath := ""

	if os.Getenv(runMode) == runModeContainer {
		if listSources {
			sourceSlice, err := a.readRuleFilesForLabels(sourceLabel)
			if err != nil {
				a.log.Error(err, "failed to read rule labels")
				return err
			}
			util.ListOptionsFromLabels(sourceSlice, sourceLabel, out)
			return nil
		}
		if listTargets {
			targetsSlice, err := a.readRuleFilesForLabels(targetLabel)
			if err != nil {
				a.log.Error(err, "failed to read rule labels")
				return err
			}
			util.ListOptionsFromLabels(targetsSlice, targetLabel, out)
			return nil
		}
	} else {
		volumes, err := a.getRulesVolumes()
		if err != nil {
			a.log.Error(err, "failed getting rules volumes")
			return err
		}

		if len(a.rules) > 0 {
			customRulePath = filepath.Join(util.CustomRulePath, a.tempRuleDir)
		}
		args := []string{"analyze", "--run-local=false"}
		if listSources {
			args = append(args, "--list-sources")
		} else {
			args = append(args, "--list-targets")
		}
		err = container.NewContainer().Run(
			ctx,
			container.WithImage(Settings.RunnerImage),
			container.WithLog(a.log.V(1)),
			container.WithEnv(runMode, runModeContainer),
			container.WithEnv(rulePath, customRulePath),
			container.WithVolumes(volumes),
			container.WithEntrypointBin(fmt.Sprintf("/usr/local/bin/%s", Settings.RootCommandName)),
			container.WithContainerToolBin(Settings.ContainerBinary),
			container.WithEntrypointArgs(args...),
			container.WithStdout(out),
			container.WithCleanup(a.cleanup),
			container.WithProxy(a.httpProxy, a.httpsProxy, a.noProxy),
		)
		if err != nil {
			a.log.Error(err, "failed listing labels")
			return err
		}
	}
	return nil
}

func (a *analyzeCommand) readRuleFilesForLabels(label string) ([]string, error) {
	labelsSlice := []string{}
	err := filepath.WalkDir(util.RulesetPath, util.WalkRuleSets(util.RulesetPath, label, &labelsSlice))
	if err != nil {
		return nil, err
	}
	rulePath := os.Getenv("RULE_PATH")
	if rulePath != "" {
		err := filepath.WalkDir(rulePath, util.WalkRuleSets(rulePath, label, &labelsSlice))
		if err != nil {
			return nil, err
		}
	}
	return labelsSlice, nil
}

func (a *analyzeCommand) getDepsFolders() (map[string]string, []string) {
	vols := map[string]string{}
	dependencyFolders := []string{}
	if len(a.depFolders) != 0 {
		for i := range a.depFolders {
			newDepPath := path.Join(util.InputPath, fmt.Sprintf("deps%v", i))
			vols[a.depFolders[i]] = newDepPath
			dependencyFolders = append(dependencyFolders, newDepPath)
		}
		return vols, dependencyFolders
	}

	return vols, dependencyFolders
}

func (a *analyzeCommand) getConfigVolumes() (map[string]string, error) {
	tempDir, err := os.MkdirTemp("", "analyze-config-")
	if err != nil {
		a.log.V(1).Error(err, "failed creating temp dir", "dir", tempDir)
		return nil, err
	}
	a.log.V(1).Info("created directory for provider settings", "dir", tempDir)
	a.tempDirs = append(a.tempDirs, tempDir)

	javaTargetPaths, err := kantraProvider.WalkJavaPathForTarget(a.log, a.isFileInput, a.input)
	if err != nil {
		// allow for duplicate incidents rather than failing analysis
		a.log.Error(err, "error getting target subdir in Java project - some duplicate incidents may occur")
	}

	var provConfig []provider.Config
	_, depsFolders := a.getDepsFolders()
	configInput := kantraProvider.ConfigInput{
		IsFileInput:             a.isFileInput,
		InputPath:               a.input,
		OutputPath:              a.output,
		MavenSettingsFile:       a.mavenSettingsFile,
		Log:                     a.log,
		Mode:                    a.mode,
		Port:                    6734,
		TmpDir:                  tempDir,
		JvmMaxMem:               Settings.JvmMaxMem,
		DepsFolders:             depsFolders,
		JavaExcludedTargetPaths: javaTargetPaths,
		DisableMavenSearch:      a.disableMavenSearch,
		JavaBundleLocation:      JavaBundlesLocation,
	}
	var builtinProvider = kantraProvider.BuiltinProvider{}
	var config, _ = builtinProvider.GetConfigVolume(configInput)
	provConfig = append(provConfig, config)

	settingsVols := map[string]string{
		tempDir: util.ConfigMountPath,
	}
	if !a.needsBuiltin {
		vols, _ := a.getDepsFolders()
		if len(vols) != 0 {
			maps.Copy(settingsVols, vols)
		}
		for provName, provInfo := range a.providersMap {
			configInput.Port = a.providersMap[provName].port
			var volConfig, err = provInfo.provider.GetConfigVolume(configInput)
			if err != nil {
				a.log.V(1).Error(err, "failed creating volume configs")
				return nil, err
			}
			provConfig = append(provConfig, volConfig)
		}

		// Set proxy to providers
		if a.httpProxy != "" || a.httpsProxy != "" {
			proxy := provider.Proxy{
				HTTPProxy:  a.httpProxy,
				HTTPSProxy: a.httpsProxy,
				NoProxy:    a.noProxy,
			}
			for i := range provConfig {
				provConfig[i].Proxy = &proxy
			}
		}

		for prov := range a.providersMap {
			err = a.getProviderOptions(tempDir, provConfig, prov)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					a.log.V(5).Info("provider options config not found, using default options")
					err := a.writeProvConfig(tempDir, provConfig)
					if err != nil {
						return nil, err
					}
				} else {
					return nil, err
				}
			}
		}
	}
	err = a.writeProvConfig(tempDir, provConfig)
	if err != nil {
		return nil, err
	}

	// attempt to create a .m2 directory we can use to speed things a bit
	// this will be shared between analyze and dep command containers
	// TODO: when this is fixed on mac and windows for podman machine volume access remove this check.
	if runtime.GOOS == "linux" {
		m2Dir, err := os.MkdirTemp("", "m2-repo-")
		if err != nil {
			a.log.V(1).Error(err, "failed to create m2 repo", "dir", m2Dir)
		} else {
			settingsVols[m2Dir] = util.M2Dir
			a.log.V(1).Info("created directory for maven repo", "dir", m2Dir)
			a.tempDirs = append(a.tempDirs, m2Dir)
		}
	}

	return settingsVols, nil
}

func (a *analyzeCommand) getRulesVolumes() (map[string]string, error) {
	if a.rules == nil || len(a.rules) == 0 {
		return nil, nil
	}
	rulesVolumes := make(map[string]string)
	tempDir, err := os.MkdirTemp("", "analyze-rules-")
	a.tempRuleDir = filepath.Base(tempDir)
	if err != nil {
		a.log.V(1).Error(err, "failed to create temp dir", "path", tempDir)
		return nil, err
	}
	a.log.V(1).Info("created directory for rules", "dir", tempDir)
	a.tempDirs = append(a.tempDirs, tempDir)
	for i, r := range a.rules {
		stat, err := os.Stat(r)
		if err != nil {
			a.log.V(1).Error(err, "failed to stat rules", "path", r)
			return nil, err
		}
		// move rules files passed into dir to mount
		if !stat.IsDir() {
			destFile := filepath.Join(tempDir, fmt.Sprintf("rules%d.yaml", i))
			err := util.CopyFileContents(r, destFile)
			if err != nil {
				a.log.V(1).Error(err, "failed to move rules file", "src", r, "dest", destFile)
				return nil, err
			}
			a.log.V(5).Info("copied file to rule dir", "added file", r, "destFile", destFile)
			err = a.createTempRuleSet(tempDir, "custom-ruleset")
			if err != nil {
				return nil, err
			}
		} else {
			a.log.V(5).Info("copying dir", "directory", r)
			err = filepath.WalkDir(r, func(path string, d fs.DirEntry, err error) error {
				if path == r {
					return nil
				}
				if d.IsDir() {
					// This will create the new dir
					a.handleDir(path, tempDir, r)
				} else {
					// If we are unable to get the file attributes, probably safe to assume this is not a
					// valid rule or ruleset and lets skip it for now.
					if isHidden, err := hiddenfile.IsHidden(path); isHidden || err != nil {
						a.log.V(5).Info("skipping hidden file", "path", path, "error", err)
						return nil
					}
					relpath, err := filepath.Rel(r, path)
					if err != nil {
						return err
					}
					destFile := filepath.Join(tempDir, relpath)
					a.log.V(5).Info("copying file main", "source", path, "dest", destFile)
					err = util.CopyFileContents(path, destFile)
					if err != nil {
						a.log.V(1).Error(err, "failed to move rules file", "src", r, "dest", destFile)
						return err
					}
				}
				return nil
			})
			if err != nil {
				a.log.V(1).Error(err, "failed to move rules file", "src", r)
				return nil, err
			}
		}
	}
	rulesVolumes[tempDir] = path.Join(util.CustomRulePath, filepath.Base(tempDir))

	return rulesVolumes, nil
}

// extractDefaultRulesets extracts default rulesets from the kantra container to the host
// This allows hybrid mode to use default rulesets without bundling them separately
func (a *analyzeCommand) extractDefaultRulesets(ctx context.Context) (string, error) {
	if !a.enableDefaultRulesets {
		return "", nil
	}

	rulesetsDir := filepath.Join(a.output, ".rulesets")

	// Check if rulesets already extracted (cached from previous run)
	if _, err := os.Stat(rulesetsDir); os.IsNotExist(err) {
		a.log.Info("extracting default rulesets from container to host", "dir", rulesetsDir)

		// Create temp container to extract rulesets
		tempName := fmt.Sprintf("ruleset-extract-%v", container.RandomName())
		createCmd := exec.CommandContext(ctx, Settings.ContainerBinary,
			"create", "--name", tempName, Settings.RunnerImage)
		createCmd.Stdout = os.Stdout
		createCmd.Stderr = os.Stderr
		if err := createCmd.Run(); err != nil {
			return "", fmt.Errorf("failed to create temp container for ruleset extraction: %w", err)
		}

		// Ensure temp container is removed
		defer func() {
			rmCmd := exec.CommandContext(ctx, Settings.ContainerBinary, "rm", tempName)
			rmCmd.Run()
		}()

		// Copy rulesets from container to host
		copyCmd := exec.CommandContext(ctx, Settings.ContainerBinary,
			"cp", fmt.Sprintf("%s:/opt/rulesets", tempName), rulesetsDir)
		copyCmd.Stdout = os.Stdout
		copyCmd.Stderr = os.Stderr
		if err := copyCmd.Run(); err != nil {
			return "", fmt.Errorf("failed to copy rulesets from container: %w", err)
		}

		a.log.Info("extracted default rulesets to host", "dir", rulesetsDir)
	} else {
		a.log.V(1).Info("using cached default rulesets", "dir", rulesetsDir)
	}

	return rulesetsDir, nil
}

// createHybridProviderSettings generates provider configs for hybrid mode
// where providers run in containers but analyzer runs on host
func (a *analyzeCommand) createHybridProviderSettings(excludedTargetPaths []interface{}) ([]provider.Config, error) {
	configs := []provider.Config{}

	// Add containerized providers (Java, Go, Python, etc.)
	for provName, provInit := range a.providersMap {
		config := provider.Config{
			Name:    provName,
			Address: fmt.Sprintf("localhost:%d", provInit.port),
			InitConfig: []provider.InitConfig{
				{
					Location:     a.input,
					AnalysisMode: provider.AnalysisMode(a.mode),
				},
			},
		}

		// Set proxy if configured
		if a.httpProxy != "" || a.httpsProxy != "" || a.noProxy != "" {
			config.InitConfig[0].Proxy = &provider.Proxy{
				HTTPProxy:  a.httpProxy,
				HTTPSProxy: a.httpsProxy,
				NoProxy:    a.noProxy,
			}
		}

		// Provider-specific configuration
		providerConfig := map[string]interface{}{}

		switch provName {
		case util.JavaProvider:
			providerConfig[provider.LspServerPathConfigKey] = "/jdtls/bin/jdtls"
			if a.mavenSettingsFile != "" {
				providerConfig["mavenSettingsFile"] = a.mavenSettingsFile
			}

		case util.GoProvider:
			providerConfig["lspServerName"] = "generic"
			providerConfig[provider.LspServerPathConfigKey] = "/usr/local/bin/gopls"
			providerConfig["workspaceFolders"] = []string{fmt.Sprintf("file://%s", util.SourceMountPath)}
			providerConfig["dependencyProviderPath"] = "/usr/local/bin/golang-dependency-provider"

		case util.PythonProvider:
			providerConfig["lspServerName"] = "generic"
			providerConfig[provider.LspServerPathConfigKey] = "/usr/local/bin/pylsp"
			providerConfig["workspaceFolders"] = []string{fmt.Sprintf("file://%s", util.SourceMountPath)}

		case util.NodeJSProvider:
			providerConfig["lspServerName"] = "nodejs"
			providerConfig[provider.LspServerPathConfigKey] = "/usr/local/bin/typescript-language-server"
			providerConfig["lspServerArgs"] = []string{"--stdio"}
			providerConfig["workspaceFolders"] = []string{fmt.Sprintf("file://%s", util.SourceMountPath)}

		case util.DotnetProvider:
			providerConfig[provider.LspServerPathConfigKey] = "C:/Users/ContainerAdministrator/.dotnet/tools/csharp-ls.exe"
		}

		config.InitConfig[0].ProviderSpecificConfig = providerConfig
		configs = append(configs, config)
	}

	// Add builtin provider (runs in-process with analyzer, not containerized)
	builtinConfig := provider.Config{
		Name: "builtin",
		InitConfig: []provider.InitConfig{
			{
				Location:     a.input,
				AnalysisMode: provider.AnalysisMode(a.mode),
				ProviderSpecificConfig: map[string]interface{}{},
			},
		},
	}

	// Set proxy for builtin if configured
	if a.httpProxy != "" || a.httpsProxy != "" || a.noProxy != "" {
		builtinConfig.InitConfig[0].Proxy = &provider.Proxy{
			HTTPProxy:  a.httpProxy,
			HTTPSProxy: a.httpsProxy,
			NoProxy:    a.noProxy,
		}
	}

	// Add excluded dirs for builtin (e.g., Java target paths)
	if len(excludedTargetPaths) > 0 {
		builtinConfig.InitConfig[0].ProviderSpecificConfig["excludedDirs"] = excludedTargetPaths
	}

	configs = append(configs, builtinConfig)

	return configs, nil
}

// RunProvidersHostNetwork starts provider containers with port publishing
// This is used in hybrid mode where providers run in containers but analyzer runs on host
func (a *analyzeCommand) RunProvidersHostNetwork(ctx context.Context, volName string, retry int) error {
	volumes := map[string]string{
		volName: util.SourceMountPath,
	}

	if a.mavenSettingsFile != "" {
		configVols, err := a.getConfigVolumes()
		if err != nil {
			a.log.V(1).Error(err, "failed to get config volumes for analysis")
			return err
		}
		maps.Copy(volumes, configVols)
	}

	vols, _ := a.getDepsFolders()
	if len(vols) != 0 {
		maps.Copy(volumes, vols)
	}

	for prov, init := range a.providersMap {
		args := []string{fmt.Sprintf("--port=%v", init.port)}

		// Publish port so it's accessible on macOS host (podman runs in VM)
		portMapping := fmt.Sprintf("%d:%d", init.port, init.port)

		a.log.Info("starting provider with port publishing", "provider", prov, "port", init.port)
		con := container.NewContainer()
		err := con.Run(
			ctx,
			container.WithImage(init.image),
			container.WithLog(a.log.V(1)),
			container.WithVolumes(volumes),
			container.WithContainerToolBin(Settings.ContainerBinary),
			container.WithEntrypointArgs(args...),
			container.WithPortPublish(portMapping),
			container.WithDetachedMode(true),
			container.WithCleanup(false),
			container.WithName(fmt.Sprintf("provider-%v", container.RandomName())),
			container.WithProxy(a.httpProxy, a.httpsProxy, a.noProxy),
		)
		if err != nil {
			return fmt.Errorf("failed to start provider %s: %w", prov, err)
		}

		a.providerContainerNames = append(a.providerContainerNames, con.Name)
		a.log.V(1).Info("provider started", "provider", prov, "container", con.Name)
	}

	return nil
}

// RunAnalysisHybrid runs analysis in hybrid mode:
// - Providers run in containers (for isolation/consistency)
// - Analyzer runs natively on host (for performance)
func (a *analyzeCommand) RunAnalysisHybrid(ctx context.Context) error {
	a.log.Info("running analysis in hybrid mode (analyzer on host, providers in containers)")

	// Validate analyzer binary exists
	analyzerBinary := "./konveyor-analyzer"
	if runtime.GOOS == "darwin" {
		analyzerBinary = "./konveyor-analyzer-macos"
	}

	if _, err := os.Stat(analyzerBinary); os.IsNotExist(err) {
		return fmt.Errorf("analyzer binary not found at %s. Please ensure the binary is in the current directory", analyzerBinary)
	}

	// Extract default rulesets if enabled
	rulesetsDir, err := a.extractDefaultRulesets(ctx)
	if err != nil {
		return fmt.Errorf("failed to extract default rulesets: %w", err)
	}

	// Get Java target paths to exclude from builtin provider
	// TODO: Implement getJavaTargetPaths() to auto-detect and exclude Java build directories
	excludedTargetPaths := []interface{}{}

	// Start containerized providers (if any)
	if len(a.providersMap) > 0 {
		// Create volume for provider containers
		volName, err := a.createContainerVolume(a.input)
		if err != nil {
			return fmt.Errorf("failed to create container volume: %w", err)
		}

		// Start providers with port publishing
		err = a.RunProvidersHostNetwork(ctx, volName, 5)
		if err != nil {
			return fmt.Errorf("failed to start providers: %w", err)
		}

		// Wait for providers to initialize
		// TODO: Replace with proper health checks
		a.log.Info("waiting for providers to initialize...")
		time.Sleep(4 * time.Second)
	} else {
		a.log.Info("no containerized providers needed, running builtin-only")
	}

	// Generate hybrid provider settings
	providerConfigs, err := a.createHybridProviderSettings(excludedTargetPaths)
	if err != nil {
		return fmt.Errorf("failed to create provider settings: %w", err)
	}

	// Write provider settings to output directory
	if err := a.writeProvConfig(a.output, providerConfigs); err != nil {
		return fmt.Errorf("failed to write provider config: %w", err)
	}

	// Build analyzer arguments
	args := []string{
		fmt.Sprintf("--provider-settings=%s", filepath.Join(a.output, "settings.json")),
		fmt.Sprintf("--output-file=%s", filepath.Join(a.output, "output.yaml")),
		fmt.Sprintf("--context-lines=%d", a.contextLines),
	}

	// Add default rulesets if extracted
	if rulesetsDir != "" {
		args = append(args, fmt.Sprintf("--rules=%s/", rulesetsDir))
	}

	// Add custom rules
	if len(a.rules) > 0 {
		for _, rule := range a.rules {
			args = append(args, fmt.Sprintf("--rules=%s", rule))
		}
	}

	// Add other flags
	if a.incidentSelector != "" {
		args = append(args, fmt.Sprintf("--incident-selector=%s", a.incidentSelector))
	}

	if a.jaegerEndpoint != "" {
		args = append(args, "--enable-jaeger", "--jaeger-endpoint", a.jaegerEndpoint)
	}

	if a.logLevel != nil {
		args = append(args, fmt.Sprintf("--verbose=%d", *a.logLevel))
	}

	labelSelector := a.getLabelSelector()
	if labelSelector != "" {
		args = append(args, fmt.Sprintf("--label-selector=%s", labelSelector))
	}

	if a.noDepRules {
		args = append(args, "--no-dependency-rules")
	}

	// Dependency analysis for Java and Go
	_, hasJava := a.providersMap[util.JavaProvider]
	_, hasGo := a.providersMap[util.GoProvider]
	if (hasJava || hasGo) && a.mode == string(provider.FullAnalysisMode) {
		if !a.analyzeKnownLibraries {
			args = append(args, fmt.Sprintf("--dep-label-selector=(!%s=open-source)", provider.DepSourceLabel))
		}
		a.log.Info("running dependency retrieval during analysis")
		args = append(args, fmt.Sprintf("--dep-output-file=%s", filepath.Join(a.output, "dependencies.yaml")))
	}

	// Run analyzer binary on host
	a.log.Info("starting analyzer", "binary", analyzerBinary, "args", strings.Join(args, " "))

	analysisLogPath := filepath.Join(a.output, "analysis.log")
	analysisLog, err := os.Create(analysisLogPath)
	if err != nil {
		return fmt.Errorf("failed to create analysis log: %w", err)
	}
	defer analysisLog.Close()

	cmd := exec.CommandContext(ctx, analyzerBinary, args...)
	cmd.Stdout = analysisLog
	cmd.Stderr = analysisLog

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("analyzer failed: %w (see %s for details)", err, analysisLogPath)
	}

	a.log.Info("hybrid analysis completed successfully")
	return nil
}

func (a *analyzeCommand) RunAnalysisOverrideProviderSettings(ctx context.Context) error {

	volumes := map[string]string{
		// output directory
		a.output:                   util.OutputPath,
		a.overrideProviderSettings: util.ProviderSettingsMountPath,
	}

	if len(a.rules) > 0 {
		ruleVols, err := a.getRulesVolumes()
		if err != nil {
			a.log.V(1).Error(err, "failed to get rule volumes for analysis")
			return err
		}
		maps.Copy(volumes, ruleVols)
	}

	args := []string{
		fmt.Sprintf("--provider-settings=%s", util.ProviderSettingsMountPath),
		fmt.Sprintf("--output-file=%s", util.AnalysisOutputMountPath),
		fmt.Sprintf("--context-lines=%d", a.contextLines),
	}

	if a.enableDefaultRulesets {
		args = append(args,
			fmt.Sprintf("--rules=%s/", util.RulesetPath))
	}

	if a.incidentSelector != "" {
		args = append(args,
			fmt.Sprintf("--incident-selector=%s", a.incidentSelector))
	}

	if len(a.rules) > 0 {
		args = append(args,
			fmt.Sprintf("--rules=%s/", util.CustomRulePath))
	}

	if a.jaegerEndpoint != "" {
		args = append(args, "--enable-jaeger")
		args = append(args, "--jaeger-endpoint")
		args = append(args, a.jaegerEndpoint)
	}
	if !a.analyzeKnownLibraries {
		args = append(args,
			fmt.Sprintf("--dep-label-selector=(!%s=open-source)", provider.DepSourceLabel))
	}
	if a.logLevel != nil {
		args = append(args, fmt.Sprintf("--verbose=%d", *a.logLevel))
	}
	labelSelector := a.getLabelSelector()
	if labelSelector != "" {
		args = append(args, fmt.Sprintf("--label-selector=%s", labelSelector))
	}
	if a.mode == string(provider.FullAnalysisMode) {
		a.log.Info("running dependency retrieval during analysis")
		args = append(args, fmt.Sprintf("--dep-output-file=%s", util.DepsOutputMountPath))
	}

	analysisLogFilePath := filepath.Join(a.output, "analysis.log")
	// create log files
	analysisLog, err := os.Create(analysisLogFilePath)
	if err != nil {
		return fmt.Errorf("failed creating analysis log file at %s", analysisLogFilePath)
	}
	defer analysisLog.Close()

	a.log.Info("running source code analysis", "log", analysisLogFilePath,
		"input", a.input, "output", a.output, "args", strings.Join(args, " "), "volumes", volumes)
	a.log.Info("generating analysis log in file", "file", analysisLogFilePath)
	// TODO (pgaikwad): run analysis & deps in parallel

	c := container.NewContainer()
	err = c.Run(
		ctx,
		container.WithImage(Settings.RunnerImage),
		container.WithLog(a.log.V(1)),
		container.WithVolumes(volumes),
		container.WithStdout(analysisLog),
		container.WithStderr(analysisLog),
		container.WithEntrypointArgs(args...),
		container.WithName(fmt.Sprintf("analyzer-%v", container.RandomName())),
		container.WithEntrypointBin("/usr/local/bin/konveyor-analyzer"),
		container.WithNetwork("host"),
		container.WithContainerToolBin(Settings.ContainerBinary),
		container.WithCleanup(a.cleanup),
		container.WithProxy(a.httpProxy, a.httpsProxy, a.noProxy),
	)
	if err != nil {
		return err
	}
	err = a.getProviderLogs(ctx)
	if err != nil {
		a.log.Error(err, "failed to get provider container logs")
	}

	return nil
}

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
	err = os.WriteFile(filepath.Join(a.output, "output.json"), jsonData, os.ModePerm)
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
	err = os.WriteFile(filepath.Join(a.output, "dependencies.json"), jsonDataDep, os.ModePerm)
	if err != nil {
		a.log.V(1).Error(err, "failed to write json dependencies output", "dir", a.output, "file", "dependencies.json")
		return err
	}

	return nil
}

func (a *analyzeCommand) GenerateStaticReport(ctx context.Context) error {
	if a.skipStaticReport {
		return nil
	}
	// it's possible for dependency analysis to fail
	// in this case we still want to generate a static report for successful source analysis
	_, noDepFileErr := os.Stat(filepath.Join(a.output, "dependencies.yaml"))
	if errors.Is(noDepFileErr, os.ErrNotExist) {
		a.log.Info("unable to get dependency output in static report. generating static report from source analysis only")
	} else if noDepFileErr != nil && !errors.Is(noDepFileErr, os.ErrNotExist) {
		return noDepFileErr
	}

	volumes := map[string]string{
		a.input:  util.SourceMountPath,
		a.output: util.OutputPath,
	}

	args := []string{}
	staticReportArgs := []string{"/usr/local/bin/js-bundle-generator",
		fmt.Sprintf("--output-path=%s", path.Join("/usr/local/static-report/output.js"))}
	// Prepare report args list with single input analysis
	applicationNames := []string{filepath.Base(a.input)}
	outputAnalyses := []string{util.AnalysisOutputMountPath}
	outputDeps := []string{util.DepsOutputMountPath}

	if a.bulk {
		a.moveResults()
		// Scan all available analysis output files to be reported
		applicationNames = nil
		outputAnalyses = nil
		outputDeps = nil
		outputFiles, err := filepath.Glob(filepath.Join(a.output, "output.yaml.*"))
		// optional
		depFiles, _ := filepath.Glob(filepath.Join(a.output, "dependencies.yaml.*"))
		if err != nil {
			return err
		}
		for i := range outputFiles {
			outputName := filepath.Base(outputFiles[i])
			applicationName := strings.SplitN(outputName, "output.yaml.", 2)[1]
			applicationNames = append(applicationNames, applicationName)
			outputAnalyses = append(outputAnalyses, strings.ReplaceAll(outputFiles[i], a.output, util.OutputPath)) // re-map paths to container mounts
			outputDeps = append(outputDeps, fmt.Sprintf("%s.%s", util.DepsOutputMountPath, applicationName))
		}

		if !errors.Is(noDepFileErr, os.ErrNotExist) {
			for i := range depFiles {
				_, depErr := os.Stat(depFiles[i])
				if a.mode != string(provider.FullAnalysisMode) || depErr != nil {
					// Remove not existing dependency files from statis report generator list
					outputDeps[i] = ""
				}
			}
			staticReportArgs = append(staticReportArgs,
				fmt.Sprintf("--deps-output-list=%s", strings.Join(outputDeps, ",")))
		}
	}

	staticReportArgs = append(staticReportArgs,
		fmt.Sprintf("--analysis-output-list=%s", strings.Join(outputAnalyses, ",")),
		fmt.Sprintf("--application-name-list=%s", strings.Join(applicationNames, ",")))

	// as of now, only java and go providers has dep capability
	if !a.bulk && !errors.Is(noDepFileErr, os.ErrNotExist) {
		_, hasJava := a.providersMap[util.JavaProvider]
		_, hasGo := a.providersMap[util.GoProvider]
		if (hasJava || hasGo) && a.mode == string(provider.FullAnalysisMode) && len(a.providersMap) == 1 {
			staticReportArgs = append(staticReportArgs,
				fmt.Sprintf("--deps-output-list=%s", util.DepsOutputMountPath))
		}
	}

	cpArgs := []string{"&& cp -r",
		"/usr/local/static-report", util.OutputPath}

	args = append(args, staticReportArgs...)
	args = append(args, cpArgs...)

	joinedArgs := strings.Join(args, " ")
	staticReportCmd := []string{joinedArgs}

	c := container.NewContainer()
	a.log.Info("generating static report",
		"output", a.output, "args", strings.Join(staticReportCmd, " "))
	err := c.Run(
		ctx,
		container.WithImage(Settings.RunnerImage),
		container.WithLog(a.log.V(1)),
		container.WithEntrypointBin("/bin/sh"),
		container.WithContainerToolBin(Settings.ContainerBinary),
		container.WithEntrypointArgs(staticReportCmd...),
		container.WithVolumes(volumes),
		container.WithcFlag(true),
		container.WithCleanup(a.cleanup),
	)
	if err != nil {
		return err
	}
	uri := uri.File(filepath.Join(a.output, "static-report", "index.html"))
	a.log.Info("Static report created. Access it at this URL:", "URL", string(uri))

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
	if errors.Is(noDepFileErr, os.ErrNotExist) && a.mode == string(provider.FullAnalysisMode) {
		return noDepFileErr
	}
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

func (a *analyzeCommand) getLabelSelector() string {
	if a.labelSelector != "" {
		return a.labelSelector
	}
	if (a.sources == nil || len(a.sources) == 0) &&
		(a.targets == nil || len(a.targets) == 0) {
		return ""
	}
	// default labels are applied everytime either a source or target is specified
	defaultLabels := []string{"discovery"}
	targets := []string{}
	for _, target := range a.targets {
		targets = append(targets,
			fmt.Sprintf("%s=%s", outputv1.TargetTechnologyLabel, target))
	}
	sources := []string{}
	for _, source := range a.sources {
		sources = append(sources,
			fmt.Sprintf("%s=%s", outputv1.SourceTechnologyLabel, source))
	}
	targetExpr := ""
	if len(targets) > 0 {
		targetExpr = fmt.Sprintf("(%s)", strings.Join(targets, " || "))
	}
	sourceExpr := ""
	if len(sources) > 0 {
		sourceExpr = fmt.Sprintf("(%s)", strings.Join(sources, " || "))
	}
	if targetExpr != "" {
		if sourceExpr != "" {
			// when both targets and sources are present, AND them
			return fmt.Sprintf("(%s && %s) || (%s)",
				targetExpr, sourceExpr, strings.Join(defaultLabels, " || "))
		} else {
			// when target is specified, but source is not
			// return target expression OR'd with default labels
			return fmt.Sprintf("%s || (%s)",
				targetExpr, strings.Join(defaultLabels, " || "))
		}
	}
	if sourceExpr != "" {
		// when only source is specified, OR them all
		return fmt.Sprintf("%s || (%s)",
			sourceExpr, strings.Join(defaultLabels, " || "))
	}
	return ""
}

func (a *analyzeCommand) writeProvConfig(tempDir string, config []provider.Config) error {
	jsonData, err := json.MarshalIndent(&config, "", "	")
	if err != nil {
		a.log.V(1).Error(err, "failed to marshal provider config")
		return err
	}
	err = os.WriteFile(filepath.Join(tempDir, "settings.json"), jsonData, os.ModePerm)
	if err != nil {
		a.log.V(1).Error(err,
			"failed to write provider config", "dir", tempDir, "file", "settings.json")
		return err
	}
	return nil
}

func (a *analyzeCommand) getProviderOptions(tempDir string, provConfig []provider.Config, prov string) error {
	var confDir string
	var set bool
	ops := runtime.GOOS
	if ops == "linux" {
		confDir, set = os.LookupEnv("XDG_CONFIG_HOME")
	}
	if ops != "linux" || confDir == "" || !set {
		// on Unix, including macOS, this returns the $HOME environment variable. On Windows, it returns %USERPROFILE%
		var err error
		confDir, err = os.UserHomeDir()
		if err != nil {
			return err
		}
	}
	// get provider options from provider settings file
	data, err := os.ReadFile(filepath.Join(confDir, ".kantra", fmt.Sprintf("%v.json", prov)))
	if err != nil {
		return err
	}
	optionsConfig := &[]provider.Config{}
	err = yaml.Unmarshal(data, optionsConfig)
	if err != nil {
		a.log.V(1).Error(err, "failed to unmarshal provider options file")
		return err
	}
	mergedConfig, err := a.mergeProviderConfig(provConfig, *optionsConfig, tempDir)
	if err != nil {
		return err
	}
	err = a.writeProvConfig(tempDir, mergedConfig)
	if err != nil {
		return err
	}
	return nil
}

func (a *analyzeCommand) mergeProviderConfig(defaultConf, optionsConf []provider.Config, tempDir string) ([]provider.Config, error) {
	merged := []provider.Config{}
	seen := map[string]*provider.Config{}

	// find options used for supported providers
	for idx, conf := range defaultConf {
		seen[conf.Name] = &defaultConf[idx]
	}

	for _, conf := range optionsConf {
		if _, ok := seen[conf.Name]; !ok {
			continue
		}
		// set provider config options
		if conf.ContextLines != 0 {
			seen[conf.Name].ContextLines = conf.ContextLines
		}
		if conf.Proxy != nil {
			seen[conf.Name].Proxy = conf.Proxy
		}
		// set init config options
		for i, init := range conf.InitConfig {
			if len(init.AnalysisMode) != 0 {
				seen[conf.Name].InitConfig[i].AnalysisMode = init.AnalysisMode
			}
			if len(init.ProviderSpecificConfig) != 0 {
				provSpecificConf, err := a.mergeProviderSpecificConfig(init.ProviderSpecificConfig, seen[conf.Name].InitConfig[i].ProviderSpecificConfig, tempDir)
				if err != nil {
					return nil, err
				}
				seen[conf.Name].InitConfig[i].ProviderSpecificConfig = provSpecificConf
			}
		}
	}
	for _, v := range seen {
		merged = append(merged, *v)
	}
	return merged, nil
}

func (a *analyzeCommand) mergeProviderSpecificConfig(optionsConf, seenConf map[string]interface{}, tempDir string) (map[string]interface{}, error) {
	for k, v := range optionsConf {
		switch {
		case optionsConf[k] == "":
			continue
		// special case for maven settings file to mount correctly
		case k == util.MavenSettingsFile:
			// validate maven settings file
			if _, err := os.Stat(v.(string)); err != nil {
				return nil, fmt.Errorf("%w failed to stat maven settings file at path %s", err, v)
			}
			if absPath, err := filepath.Abs(v.(string)); err == nil {
				seenConf[k] = absPath
			}
			// copy file to mount path
			err := util.CopyFileContents(v.(string), filepath.Join(tempDir, "settings.xml"))
			if err != nil {
				a.log.V(1).Error(err, "failed copying maven settings file", "path", v)
				return nil, err
			}
			seenConf[k] = fmt.Sprintf("%s/%s", util.ConfigMountPath, "settings.xml")
			continue
		// we don't want users to override these options here
		// use --overrideProviderSettings to do so
		case k != util.LspServerPath && k != util.LspServerName && k != util.WorkspaceFolders && k != util.DependencyProviderPath:
			seenConf[k] = v
		}
	}
	return seenConf, nil
}

func (a *analyzeCommand) getProviderLogs(ctx context.Context) error {
	if len(a.providerContainerNames) == 0 || a.needsBuiltin {
		return nil
	}
	providerLogFilePath := filepath.Join(a.output, "provider.log")
	providerLog, err := os.Create(providerLogFilePath)
	if err != nil {
		return fmt.Errorf("failed creating provider log file at %s", providerLogFilePath)
	}
	defer providerLog.Close()
	for i := range a.providerContainerNames {
		a.log.V(1).Info("getting provider container logs",
			"container", a.providerContainerNames[i])

		cmd := exec.CommandContext(
			ctx,
			Settings.ContainerBinary,
			"logs",
			a.providerContainerNames[i])

		cmd.Stdout = providerLog
		cmd.Stderr = providerLog
		return cmd.Run()
	}

	return nil
}

func (a *analyzeCommand) analyzeDotnetFramework(ctx context.Context) error {
	if runtime.GOOS != "windows" {
		err := fmt.Errorf("Unsupported OS")
		a.log.Error(err, "Analysis of .NET Framework projects is only supported on Windows")
		return err
	}

	if a.bulk {
		err := fmt.Errorf("Unsupported option")
		a.log.Error(err, "Bulk analysis not supported for .NET Framework projects")
		return err
	}

	if a.mode == string(provider.FullAnalysisMode) {
		a.log.V(1).Info("Only source mode analysis is supported")
		a.mode = string(provider.SourceOnlyAnalysisMode)
	}

	var err error

	// Check configuration
	var systemInfo struct {
		Plugins struct {
			Network []string `json:"network"`
		} `json:"plugins"`
	}
	cmd := exec.Command(Settings.ContainerBinary, []string{"system", "info", "--format=json"}...)
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	if err = json.Unmarshal(out, &systemInfo); err != nil {
		return err
	}
	a.log.V(5).Info("container network plugins", "plugins", systemInfo)
	if !slices.Contains(systemInfo.Plugins.Network, "nat") {
		err := fmt.Errorf("Unsupported container client configuration")
		a.log.Error(err, ".NET Framework projects must be analyzed using docker configured to run Windows containers")
		return err
	}

	// Create network
	networkName := container.RandomName()
	cmd = exec.Command(Settings.ContainerBinary, []string{"network", "create", "-d", "nat", networkName}...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return err
	}
	a.log.V(1).Info("created container network", "network", networkName)
	a.networkName = networkName
	// end create network

	// Create volume
	// opts aren't supported on Windows
	// containerVolName, err := a.createContainerVolume()
	// if err != nil {
	// 	a.log.Error(err, "failed to create container volume")
	// 	return err
	// }

	// Run provider
	//foundProviders := []string{dotnetFrameworkProvider}
	//providerPorts, err := a.RunProviders(ctx, networkName, containerVolName, foundProviders, 5)
	//if err != nil {
	//	a.log.Error(err, "failed to run provider")
	//	return err
	//}
	input, err := filepath.Abs(a.input)
	if err != nil {
		return err
	}
	port, err := freeport.GetFreePort()
	if err != nil {
		return err
	}
	a.log.V(1).Info("Starting dotnet-external-provider")
	providerContainer := container.NewContainer()
	err = providerContainer.Run(
		ctx,
		container.WithImage(Settings.DotnetProviderImage),
		container.WithLog(a.log.V(1)),
		container.WithVolumes(map[string]string{
			input: "C:" + filepath.FromSlash(util.SourceMountPath),
		}),
		container.WithContainerToolBin(Settings.ContainerBinary),
		container.WithEntrypointArgs([]string{fmt.Sprintf("--port=%v", port)}...),
		container.WithDetachedMode(true),
		container.WithName(fmt.Sprintf("provider-%v", container.RandomName())),
		container.WithCleanup(a.cleanup),
		container.WithNetwork(networkName),
		container.WithProxy(a.httpProxy, a.httpsProxy, a.noProxy),
	)
	if err != nil {
		return err
	}
	a.providerContainerNames = append(a.providerContainerNames, providerContainer.Name)
	a.log.V(1).Info("Provider started")
	// end run provider

	// Run analysis
	// err = a.RunAnalysis(ctx, "", containerVolName, foundProviders, providerPorts)
	// if err != nil {
	// 	a.log.Error(err, "failed to run analysis")
	// 	return err
	// }
	tempDir, err := os.MkdirTemp("", "analyze-config-")
	if err != nil {
		a.log.V(1).Error(err, "failed creating temp dir", "dir", tempDir)
		return err
	}
	a.log.V(1).Info("created directory for provider settings", "dir", tempDir)
	a.tempDirs = append(a.tempDirs, tempDir)

	// Set the IP!!!
	provConfig := []provider.Config{
		{
			Name: "builtin",
			InitConfig: []provider.InitConfig{
				{
					Location:     "C:" + filepath.FromSlash(util.SourceMountPath),
					AnalysisMode: provider.AnalysisMode(a.mode),
				},
			},
		},
		{
			Name:    util.DotnetProvider,
			Address: fmt.Sprintf("%v:%v", providerContainer.Name, port),
			InitConfig: []provider.InitConfig{
				{
					Location:     "C:" + filepath.FromSlash(util.SourceMountPath),
					AnalysisMode: provider.AnalysisMode(a.mode),
					ProviderSpecificConfig: map[string]interface{}{
						provider.LspServerPathConfigKey: "C:/Users/ContainerAdministrator/.dotnet/tools/csharp-ls.exe",
					},
				},
			},
		},
	}

	jsonData, err := json.MarshalIndent(&provConfig, "", "	")
	if err != nil {
		a.log.V(1).Error(err, "failed to marshal provider config")
		return err
	}
	err = os.WriteFile(filepath.Join(tempDir, "settings.json"), jsonData, os.ModePerm)
	if err != nil {
		a.log.V(1).Error(err,
			"failed to write provider config", "dir", tempDir, "file", "settings.json")
		return err
	}

	volumes := map[string]string{
		tempDir:  "C:" + filepath.FromSlash(util.ConfigMountPath),
		input:    "C:" + filepath.FromSlash(util.SourceMountPath),
		a.output: "C:" + filepath.FromSlash(util.OutputPath),
	}

	args := []string{
		fmt.Sprintf("--provider-settings=%s", "C:"+filepath.FromSlash(util.ProviderSettingsMountPath)),
		fmt.Sprintf("--output-file=%s", "C:"+filepath.FromSlash(util.AnalysisOutputMountPath)),
		fmt.Sprintf("--context-lines=%d", a.contextLines),
	}

	if a.enableDefaultRulesets {
		args = append(args, fmt.Sprintf("--rules=C:%s", filepath.FromSlash(util.RulesetPath)))
	}

	if len(a.rules) > 0 {
		ruleVols, err := a.getRulesVolumes()
		if err != nil {
			a.log.V(1).Error(err, "failed to get rule volumes for analysis")
			return err
		}
		for key, value := range ruleVols {
			volumes[key] = "C:" + filepath.FromSlash(value)
		}

		args = append(args, fmt.Sprintf("--rules=C:%s", filepath.FromSlash(util.CustomRulePath)))
	}

	if a.jaegerEndpoint != "" {
		args = append(args, "--enable-jaeger")
		args = append(args, "--jaeger-endpoint")
		args = append(args, a.jaegerEndpoint)
	}

	if a.logLevel != nil {
		args = append(args, fmt.Sprintf("--verbose=%d", *a.logLevel))
	}
	labelSelector := a.getLabelSelector()
	if labelSelector != "" {
		args = append(args, fmt.Sprintf("--label-selector=%s", labelSelector))
	}

	analysisLogFilePath := filepath.Join(a.output, "analysis.log")
	// create log files
	analysisLog, err := os.Create(analysisLogFilePath)
	if err != nil {
		return fmt.Errorf("failed creating analysis log file at %s", analysisLogFilePath)
	}
	defer analysisLog.Close()

	a.log.Info("running source code analysis", "log", analysisLogFilePath,
		"input", a.input, "output", a.output, "args", strings.Join(args, " "), "volumes", volumes)
	a.log.Info("generating analysis log in file", "file", analysisLogFilePath)

	c := container.NewContainer()
	err = c.Run(
		ctx,
		container.WithImage(Settings.RunnerImage),
		container.WithLog(a.log.V(1)),
		container.WithVolumes(volumes),
		container.WithName(fmt.Sprintf("analyzer-%v", container.RandomName())),
		container.WithStdout(analysisLog),
		container.WithStderr(analysisLog),
		container.WithEntrypointArgs(args...),
		container.WithEntrypointBin(`C:\app\konveyor-analyzer.exe`),
		container.WithNetwork(networkName),
		container.WithContainerToolBin(Settings.ContainerBinary),
		container.WithCleanup(a.cleanup),
		container.WithProxy(a.httpProxy, a.httpsProxy, a.noProxy),
	)
	if err != nil {
		return err
	}
	err = a.getProviderLogs(ctx)
	if err != nil {
		a.log.Error(err, "failed to get provider container logs")
	}
	// end run analysis

	// Create json output
	err = a.CreateJSONOutput()
	if err != nil {
		a.log.Error(err, "failed to create json output file")
		return err
	}

	// Generate Static Report
	if a.skipStaticReport {
		return nil
	}

	err = container.NewContainer().Run(
		ctx,
		container.WithImage(Settings.RunnerImage),
		container.WithLog(a.log.V(1)),
		container.WithContainerToolBin(Settings.ContainerBinary),
		container.WithEntrypointBin("powershell"),
		container.WithEntrypointArgs("Copy-Item", `C:\app\static-report\`, "-Recurse", filepath.FromSlash(util.OutputPath)),
		container.WithVolumes(volumes),
		container.WithCleanup(a.cleanup),
	)
	if err != nil {
		return err
	}

	staticReportArgs := []string{
		fmt.Sprintf(`-output-path=C:\%s\static-report\output.js`, filepath.FromSlash(util.OutputPath)),
		fmt.Sprintf("-analysis-output-list=C:%s", filepath.FromSlash(util.AnalysisOutputMountPath)),
		fmt.Sprintf("-application-name-list=%s", filepath.Base(a.input)),
	}

	//staticReportContainer := container.NewContainer()
	a.log.Info("generating static report", "output", a.output, "args", staticReportArgs)
	err = container.NewContainer().Run(
		ctx,
		container.WithImage(Settings.RunnerImage),
		container.WithLog(a.log.V(1)),
		container.WithContainerToolBin(Settings.ContainerBinary),
		container.WithEntrypointBin(`C:\app\js-bundle-generator`),
		container.WithEntrypointArgs(staticReportArgs...),
		container.WithVolumes(volumes),
		container.WithCleanup(a.cleanup),
	)
	if err != nil {
		return err
	}

	uri := uri.File(filepath.Join(a.output, "static-report", "index.html"))
	a.log.Info("Static report created. Access it at this URL:", "URL", string(uri))

	return nil
}

func (a *analyzeCommand) detectJavaProviderFallback() (bool, error) {
	a.log.V(7).Info("language files not found. Using fallback")
	pomPath := filepath.Join(a.input, "pom.xml")
	_, err := os.Stat(pomPath)
	// some other error
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err == nil {
		return true, nil
	}
	// try gradle next
	gradlePath := filepath.Join(a.input, "build.gradle")
	_, err = os.Stat(gradlePath)
	// some other error
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err

		// java project not found
	} else if err != nil && errors.Is(err, os.ErrNotExist) {
		a.log.V(7).Info("language files not found. Only starting builtin provider")
		return false, nil

	} else if err == nil {
		return true, nil
	}

	return false, nil
}

func listLanguages(languages []model.Language, input string) error {
	switch {
	case len(languages) == 0:
		return fmt.Errorf("failed to detect application language(s)")
	default:
		fmt.Fprintln(os.Stdout, "found languages for input application:", input)
		for _, l := range languages {
			fmt.Fprintln(os.Stdout, l.Name)
		}
		fmt.Fprintln(os.Stdout, "run --list-providers to view supported language providers")
	}
	return nil
}
