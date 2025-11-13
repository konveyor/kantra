package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	kantraProvider "github.com/konveyor-ecosystem/kantra/pkg/provider"
	"github.com/konveyor-ecosystem/kantra/pkg/util"

	"github.com/bombsimon/logrusr/v3"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	java "github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	outputv1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/parser"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"github.com/konveyor/analyzer-lsp/tracing"
	"github.com/sirupsen/logrus"
	"go.lsp.dev/uri"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/yaml.v2"
)

type ConsoleHook struct {
	Level logrus.Level
	Log   logr.Logger
}

func (hook *ConsoleHook) Fire(entry *logrus.Entry) error {
	_, err := entry.String()
	if err != nil {
		return nil // Ignore the error
	}

	if entry.Data["logger"] == "process-rule" {
		hook.Log.Info("processing rule", "ruleID", entry.Data["ruleID"])
	}
	return nil
}

func (hook *ConsoleHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (a *analyzeCommand) RunAnalysisContainerless(ctx context.Context) error {
	startTotal := time.Now()
	a.log.V(1).Info("[TIMING] Containerless analysis starting")

	err := a.ValidateContainerless(ctx)
	if err != nil {
		a.log.Error(err, "failed to validate flags")
		return err
	}

	if a.reqMap == nil {
		a.reqMap = make(map[string]string)
	}

	defer os.Remove(filepath.Join(a.output, "settings.json"))

	analysisLogFilePath := filepath.Join(a.output, "analysis.log")
	analysisLog, err := os.Create(analysisLogFilePath)
	if err != nil {
		return fmt.Errorf("failed creating provider log file at %s", analysisLogFilePath)
	}
	defer analysisLog.Close()

	// clean jdtls dirs after analysis
	defer func() {
		if err := a.cleanlsDirs(); err != nil {
			a.log.Error(err, "failed to clean language server directories")
		}
	}()

	// log output from analyzer to file
	logrusAnalyzerLog := logrus.New()
	logrusAnalyzerLog.SetOutput(analysisLog)
	logrusAnalyzerLog.SetFormatter(&logrus.TextFormatter{})
	logrusAnalyzerLog.SetLevel(logrus.Level(logLevel))

	// add log hook, print the rule processing to the console
	consoleHook := &ConsoleHook{Level: logrus.InfoLevel, Log: a.log}
	logrusAnalyzerLog.AddHook(consoleHook)

	analyzeLog := logrusr.New(logrusAnalyzerLog)

	// log kantra errs to stderr
	logrusErrLog := logrus.New()
	logrusErrLog.SetOutput(os.Stderr)
	errLog := logrusr.New(logrusErrLog)

	a.log.Info("running source analysis")
	labelSelectors := a.getLabelSelector()

	selectors := []engine.RuleSelector{}
	if labelSelectors != "" {
		selector, err := labels.NewLabelSelector[*engine.RuleMeta](labelSelectors, nil)
		if err != nil {
			errLog.Error(err, "failed to create label selector from expression", "selector", labelSelectors)
			os.Exit(1)
		}
		selectors = append(selectors, selector)
	}

	var dependencyLabelSelector *labels.LabelSelector[*konveyor.Dep]
	depLabel := fmt.Sprintf("!%v=open-source", provider.DepSourceLabel)
	if !a.analyzeKnownLibraries {
		dependencyLabelSelector, err = labels.NewLabelSelector[*konveyor.Dep](depLabel, nil)
		if err != nil {
			errLog.Error(err, "failed to create label selector from expression", "selector", depLabel)
			os.Exit(1)
		}
	}

	err = a.setBinMapContainerless()
	if err != nil {
		a.log.Error(err, "unable to find kantra dependencies")
		os.Exit(1)
	}

	providers := map[string]provider.InternalProviderClient{}
	providerLocations := []string{}

	startJavaProvider := time.Now()
	a.log.V(1).Info("[TIMING] Starting Java provider setup")
	javaProvider, javaLocations, additionalBuiltinConfigs, err := a.setupJavaProvider(ctx, analyzeLog)
	if err != nil {
		errLog.Error(err, "unable to start Java provider")
		os.Exit(1)
	}
	providers[util.JavaProvider] = javaProvider
	providerLocations = append(providerLocations, javaLocations...)
	a.log.V(1).Info("[TIMING] Java provider setup complete", "duration_ms", time.Since(startJavaProvider).Milliseconds())

	//scopes := []engine.Scope{}
	javaTargetPaths, err := kantraProvider.WalkJavaPathForTarget(a.log, a.isFileInput, a.input)
	if err != nil {
		// allow for duplicate incidents rather than failing analysis
		a.log.Error(err, "error getting target subdir in Java project - some duplicate incidents may occur")
	}

	startBuiltinProvider := time.Now()
	a.log.V(1).Info("[TIMING] Starting builtin provider setup")
	builtinProvider, builtinLocations, err := a.setupBuiltinProvider(ctx, javaTargetPaths, additionalBuiltinConfigs, analyzeLog)
	if err != nil {
		errLog.Error(err, "unable to start builtin provider")
		os.Exit(1)
	}
	providers["builtin"] = builtinProvider
	providerLocations = append(providerLocations, builtinLocations...)
	a.log.V(1).Info("[TIMING] Builtin provider setup complete", "duration_ms", time.Since(startBuiltinProvider).Milliseconds())

	engineCtx, engineSpan := tracing.StartNewSpan(ctx, "rule-engine")
	//start up the rule eng
	eng := engine.CreateRuleEngine(engineCtx,
		10,
		analyzeLog,
		engine.WithContextLines(a.contextLines),
		engine.WithIncidentSelector(a.incidentSelector),
		engine.WithLocationPrefixes(providerLocations),
	)

	parser := parser.RuleParser{
		ProviderNameToClient: providers,
		Log:                  analyzeLog.WithName("parser"),
		NoDependencyRules:    a.noDepRules,
		DepLabelSelector:     dependencyLabelSelector,
	}

	ruleSets := []engine.RuleSet{}
	needProviders := map[string]provider.InternalProviderClient{}

	if a.enableDefaultRulesets {
		a.rules = append(a.rules, filepath.Join(a.kantraDir, RulesetsLocation))
	}

	startRuleLoading := time.Now()
	a.log.V(1).Info("[TIMING] Starting rule loading")
	for _, f := range a.rules {
		a.log.Info("parsing rules for analysis", "rules", f)

		internRuleSet, internNeedProviders, err := parser.LoadRules(f)
		if err != nil {
			a.log.Error(err, "unable to parse all the rules for ruleset", "file", f)
		}
		ruleSets = append(ruleSets, internRuleSet...)
		for k, v := range internNeedProviders {
			needProviders[k] = v
		}
	}
	a.log.V(1).Info("[TIMING] Rule loading complete", "duration_ms", time.Since(startRuleLoading).Milliseconds())

	// start dependency analysis for full analysis mode only
	wg := &sync.WaitGroup{}
	var depSpan trace.Span
	if a.mode == string(provider.FullAnalysisMode) {
		var depCtx context.Context
		depCtx, depSpan = tracing.StartNewSpan(ctx, "dep")
		wg.Add(1)

		a.log.Info("running depencency analysis")
		go a.DependencyOutputContainerless(depCtx, providers, "dependencies.yaml", wg)
	}

	// This will already wait
	startRuleExecution := time.Now()
	a.log.V(1).Info("[TIMING] Starting rule execution")
	a.log.Info("evaluating rules for violations. see analysis.log for more info")
	rulesets := eng.RunRules(ctx, ruleSets, selectors...)
	engineSpan.End()
	wg.Wait()
	if depSpan != nil {
		depSpan.End()
	}
	eng.Stop()

	for _, provider := range needProviders {
		provider.Stop()
	}
	a.log.V(1).Info("[TIMING] Rule execution complete", "duration_ms", time.Since(startRuleExecution).Milliseconds())

	sort.SliceStable(rulesets, func(i, j int) bool {
		return rulesets[i].Name < rulesets[j].Name
	})

	// Write results out to CLI
	startWriting := time.Now()
	a.log.V(1).Info("[TIMING] Starting output writing")
	a.log.Info("writing analysis results to output", "output", a.output)
	b, err := yaml.Marshal(rulesets)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(a.output, "output.yaml"), b, 0644)
	if err != nil {
		os.Exit(1) // Treat the error as a fatal error
	}

	err = a.CreateJSONOutput()
	if err != nil {
		a.log.Error(err, "failed to create json output file")
		return err
	}
	a.log.V(1).Info("[TIMING] Output writing complete", "duration_ms", time.Since(startWriting).Milliseconds())

	// Ensure analysis log is closed before creating static-report (needed for bulk on Windows)
	analysisLog.Close()

	startStaticReport := time.Now()
	a.log.V(1).Info("[TIMING] Starting static report generation")
	err = a.GenerateStaticReportContainerless(ctx)
	if err != nil {
		a.log.Error(err, "failed to generate static report")
		return err
	}
	a.log.V(1).Info("[TIMING] Static report generation complete", "duration_ms", time.Since(startStaticReport).Milliseconds())

	a.log.V(1).Info("[TIMING] Containerless analysis complete", "total_duration_ms", time.Since(startTotal).Milliseconds())
	return nil
}

func (a *analyzeCommand) ValidateContainerless(ctx context.Context) error {
	// validate input app is not the current dir
	// .metadata cannot initialize in the app root
	currentDir, err := os.Getwd()
	if err != nil {
		return err
	}
	if a.input == currentDir {
		return fmt.Errorf("input path %s cannot be the current directory", a.input)
	}

	// validate mvn and openjdk install
	_, mvnErr := exec.LookPath("mvn")
	if mvnErr != nil {
		return fmt.Errorf("%w cannot find requirement maven; ensure maven is installed", mvnErr)

	}
	cmd := exec.Command("java", "-version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w cannot execute required command java; ensure java is installed", err)
	}
	if strings.Contains(string(output), "openjdk") {
		re := regexp.MustCompile(`openjdk version "(.*?)"`)
		match := re.FindStringSubmatch(string(output))
		jdkVersionStr := strings.Split(match[1], ".")
		jdkVersionInt, err := strconv.Atoi(jdkVersionStr[0])
		if err != nil {
			return fmt.Errorf("%w cannot parse java version", err)
		}
		if jdkVersionInt < 17 {
			return fmt.Errorf("cannot find requirement openjdk17+; ensure openjdk17+ is installed")
		}
	}
	if os.Getenv("JAVA_HOME") == "" {
		return fmt.Errorf("JAVA_HOME is not set; ensure JAVA_HOME is set")
	}

	// Validate .kantra in home directory and its content (containerless)
	requiredDirs := []string{a.kantraDir, filepath.Join(a.kantraDir, RulesetsLocation), filepath.Join(a.kantraDir, JavaBundlesLocation),
		filepath.Join(a.kantraDir, JDTLSBinLocation), filepath.Join(a.kantraDir, "fernflower.jar")}
	for _, path := range requiredDirs {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			a.log.Error(err, "cannot open required path, ensure that container-less dependencies are installed")
			return err
		}
	}

	return nil
}

func (a *analyzeCommand) listLabelsContainerless(ctx context.Context) error {
	return a.fetchLabelsContainerless(ctx, a.listSources, a.listTargets, os.Stdout)
}

func (a *analyzeCommand) fetchLabelsContainerless(ctx context.Context, listSources, listTargets bool, out io.Writer) error {
	// reserved labels
	sourceLabel := outputv1.SourceTechnologyLabel
	targetLabel := outputv1.TargetTechnologyLabel

	if listSources {
		sourceSlice, err := a.walkRuleFilesForLabelsContainerless(sourceLabel)
		if err != nil {
			a.log.Error(err, "failed to read rule labels")
			return err
		}
		util.ListOptionsFromLabels(sourceSlice, sourceLabel, out)
		return nil
	}
	if listTargets {
		targetsSlice, err := a.walkRuleFilesForLabelsContainerless(targetLabel)
		if err != nil {
			a.log.Error(err, "failed to read rule labels")
			return err
		}
		util.ListOptionsFromLabels(targetsSlice, targetLabel, out)
		return nil
	}

	return nil
}

func (a *analyzeCommand) walkRuleFilesForLabelsContainerless(label string) ([]string, error) {
	labelsSlice := []string{}
	path := filepath.Join(a.kantraDir, RulesetsLocation)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		a.log.Error(err, "cannot open provided path")
		return nil, err
	}
	err := filepath.WalkDir(path, util.WalkRuleSets(path, label, &labelsSlice))
	if err != nil {
		return nil, err
	}
	if len(a.rules) > 0 {
		for _, p := range a.rules {
			err := filepath.WalkDir(p, util.WalkRuleSets(p, label, &labelsSlice))
			if err != nil {
				return nil, err
			}
		}
	}
	return labelsSlice, nil
}

func (a *analyzeCommand) setKantraDir() error {
	var dir string
	var err error
	set := true
	reqs := []string{
		RulesetsLocation,
		"jdtls",
		"static-report",
	}
	// check current dir first for reqs
	dir, err = os.Getwd()
	if err != nil {
		return err
	}
	for _, v := range reqs {
		_, err := os.Stat(filepath.Join(dir, v))
		if err != nil {
			set = false
			a.log.V(7).Info("requirement not found in current dir. Checking $HOME/.kantra")
			break
		}
	}
	// all reqs found here
	if set {
		a.kantraDir = dir
		return nil
	}
	// fall back to $HOME/.kantra
	ops := runtime.GOOS
	if ops == "linux" {
		dir, set = os.LookupEnv("XDG_CONFIG_HOME")
	}
	if ops != "linux" || dir == "" || !set {
		// on Unix, including macOS, this returns the $HOME environment variable. On Windows, it returns %USERPROFILE%
		dir, err = os.UserHomeDir()
		if err != nil {
			return err
		}
	}
	a.kantraDir = filepath.Join(dir, ".kantra")
	return nil
}

func (a *analyzeCommand) setBinMapContainerless() error {
	a.reqMap["bundle"] = filepath.Join(a.kantraDir, JavaBundlesLocation)
	a.reqMap["jdtls"] = filepath.Join(a.kantraDir, JDTLSBinLocation)
	// validate
	for _, v := range a.reqMap {
		stat, err := os.Stat(v)
		if err != nil {
			return fmt.Errorf("%w failed to stat bin %s", err, v)
		}
		if stat.Mode().IsDir() {
			return fmt.Errorf("unable to find expected file at %s", v)
		}
	}
	return nil
}

func (a *analyzeCommand) makeBuiltinProviderConfig(excludedTargetPaths []interface{}) provider.Config {
	builtinConfig := provider.Config{
		Name: "builtin",
		InitConfig: []provider.InitConfig{
			{
				Location:               a.input,
				AnalysisMode:           provider.AnalysisMode(a.mode),
				ProviderSpecificConfig: map[string]interface{}{},
			},
		},
	}
	if len(excludedTargetPaths) > 0 {
		builtinConfig.InitConfig[0].ProviderSpecificConfig["excludedDirs"] = excludedTargetPaths
	}
	return builtinConfig
}

func (a *analyzeCommand) makeJavaProviderConfig() provider.Config {
	javaConfig := provider.Config{
		Name:       util.JavaProvider,
		BinaryPath: a.reqMap["jdtls"],
		InitConfig: []provider.InitConfig{
			{
				Location:     a.input,
				AnalysisMode: provider.AnalysisMode(a.mode),
				ProviderSpecificConfig: map[string]interface{}{
					"cleanExplodedBin":              true,
					"fernFlowerPath":                filepath.Join(a.kantraDir, "fernflower.jar"),
					"lspServerName":                 util.JavaProvider,
					"bundles":                       a.reqMap["bundle"],
					provider.LspServerPathConfigKey: a.reqMap["jdtls"],
					"depOpenSourceLabelsFile":       filepath.Join(a.kantraDir, "maven.default.index"),
					"mavenIndexPath":                filepath.Join(a.kantraDir),
					"disableMavenSearch":            a.disableMavenSearch,
					"gradleSourcesTaskFile":         filepath.Join(a.kantraDir, "task.gradle"),
				},
			},
		},
	}
	if a.mavenSettingsFile != "" {
		javaConfig.InitConfig[0].ProviderSpecificConfig["mavenSettingsFile"] = a.mavenSettingsFile
	}
	if Settings.JvmMaxMem != "" {
		javaConfig.InitConfig[0].ProviderSpecificConfig["jvmMaxMem"] = Settings.JvmMaxMem
	}
	return javaConfig
}

func (a *analyzeCommand) createProviderConfigsContainerless(excludedTargetPaths []interface{}) ([]provider.Config, error) {
	builtinConfig := a.makeBuiltinProviderConfig(excludedTargetPaths)
	javaConfig := a.makeJavaProviderConfig()

	provConfigs := []provider.Config{builtinConfig, javaConfig}

	for i := range provConfigs {
		// Set proxy to providers
		if a.httpProxy != "" || a.httpsProxy != "" {
			proxy := provider.Proxy{
				HTTPProxy:  a.httpProxy,
				HTTPSProxy: a.httpsProxy,
				NoProxy:    a.noProxy,
			}

			provConfigs[i].Proxy = &proxy
		}
		provConfigs[i].ContextLines = a.contextLines
	}

	jsonData, err := json.MarshalIndent(&provConfigs, "", "	")
	if err != nil {
		a.log.V(1).Error(err, "failed to marshal provider config")
		return nil, err
	}
	err = os.WriteFile(filepath.Join(a.output, "settings.json"), jsonData, os.ModePerm)
	if err != nil {
		a.log.V(1).Error(err,
			"failed to write provider config", "dir", a.output, "file", "settings.json")
		return nil, err
	}
	configs := a.setConfigsContainerless(provConfigs)
	return configs, nil
}

func (a *analyzeCommand) setConfigsContainerless(configs []provider.Config) []provider.Config {
	// we add builtin configs by default for all locations
	defaultBuiltinConfigs := []provider.InitConfig{}
	seenBuiltinConfigs := map[string]bool{}
	finalConfigs := []provider.Config{}
	for _, config := range configs {
		if config.Name != "builtin" {
			finalConfigs = append(finalConfigs, config)
		}
		for _, initConf := range config.InitConfig {
			builtinConf := provider.InitConfig{}
			_, ok := seenBuiltinConfigs[initConf.Location]
			if !ok {
				if initConf.Location != "" {
					if stat, err := os.Stat(initConf.Location); err == nil && stat.IsDir() {
						builtinLocation, err := filepath.Abs(initConf.Location)
						if err != nil {
							builtinLocation = initConf.Location
						}
						seenBuiltinConfigs[builtinLocation] = true
						builtinConf = provider.InitConfig{Location: builtinLocation}
						if config.Name == "builtin" {
							builtinConf.ProviderSpecificConfig = initConf.ProviderSpecificConfig
						}
						defaultBuiltinConfigs = append(defaultBuiltinConfigs, builtinConf)
					}
				}
			}
			//builtin config that already has location as other prov configs
			if config.Name == "builtin" && ok {
				builtinConf.ProviderSpecificConfig = initConf.ProviderSpecificConfig
				for i, c := range defaultBuiltinConfigs {
					if initConf.Location == c.Location {
						defaultBuiltinConfigs[i] = initConf
					}
				}
			}
		}
	}

	finalConfigs = append(finalConfigs, provider.Config{
		Name:       "builtin",
		InitConfig: defaultBuiltinConfigs,
	})

	return finalConfigs
}

func (a *analyzeCommand) setBuiltinProvider(config provider.Config, analysisLog logr.Logger) (provider.InternalProviderClient, error) {
	a.log.Info("setting provider from provider config", "provider", config.Name)
	config.ContextLines = a.contextLines

	// IF analysis mode is set from the CLI, then we will override this for each init config
	if a.mode != "" {
		inits := []provider.InitConfig{}
		for _, i := range config.InitConfig {
			i.AnalysisMode = provider.AnalysisMode(a.mode)
			inits = append(inits, i)
		}
		config.InitConfig = inits
	}

	prov, err := lib.GetProviderClient(config, analysisLog)
	if err != nil {
		a.log.Error(err, "failed to create builtin provider")
		return nil, err
	}

	return prov, nil
}

func (a *analyzeCommand) setJavaProvider(config provider.Config, analysisLog logr.Logger) provider.InternalProviderClient {
	a.log.Info("setting provider from provider config", "provider", config.Name)
	config.ContextLines = a.contextLines

	// If analysis mode is set from the CLI, then we will override this for each init config
	if a.mode != "" {
		inits := []provider.InitConfig{}
		for _, i := range config.InitConfig {
			i.AnalysisMode = provider.AnalysisMode(a.mode)
			inits = append(inits, i)
		}
		config.InitConfig = inits
	}

	return java.NewJavaProvider(analysisLog, "java", a.contextLines, config)
}

func (a *analyzeCommand) setupJavaProvider(ctx context.Context, analysisLog logr.Logger) (provider.InternalProviderClient, []string, []provider.InitConfig, error) {
	javaConfig := a.makeJavaProviderConfig()
	if a.httpProxy != "" || a.httpsProxy != "" {
		proxy := provider.Proxy{
			HTTPProxy:  a.httpProxy,
			HTTPSProxy: a.httpsProxy,
			NoProxy:    a.noProxy,
		}
		javaConfig.Proxy = &proxy
	}
	javaConfig.ContextLines = a.contextLines

	providerLocations := []string{}
	for _, ind := range javaConfig.InitConfig {
		providerLocations = append(providerLocations, ind.Location)
	}

	javaProvider := a.setJavaProvider(javaConfig, analysisLog)

	a.log.Info("starting provider", "provider", util.JavaProvider)
	initCtx, initSpan := tracing.StartNewSpan(ctx, "init",
		attribute.Key("provider").String(util.JavaProvider))
	additionalBuiltinConfs, err := javaProvider.ProviderInit(initCtx, nil)
	if err != nil {
		a.log.Error(err, "unable to init the providers", "provider", util.JavaProvider)
		initSpan.End()
		return nil, nil, nil, err
	}
	initSpan.End()

	return javaProvider, providerLocations, additionalBuiltinConfs, nil
}

func (a *analyzeCommand) setupBuiltinProvider(ctx context.Context, excludedTargetPaths []interface{}, additionalConfigs []provider.InitConfig, analysisLog logr.Logger) (provider.InternalProviderClient, []string, error) {
	a.log.Info("setting up builtin provider")
	builtinConfig := a.makeBuiltinProviderConfig(excludedTargetPaths)

	// Set proxy if configured
	if a.httpProxy != "" || a.httpsProxy != "" {
		proxy := provider.Proxy{
			HTTPProxy:  a.httpProxy,
			HTTPSProxy: a.httpsProxy,
			NoProxy:    a.noProxy,
		}
		builtinConfig.Proxy = &proxy
	}
	builtinConfig.ContextLines = a.contextLines

	providerLocations := []string{}
	for _, ind := range builtinConfig.InitConfig {
		providerLocations = append(providerLocations, ind.Location)
	}

	builtinProvider, err := a.setBuiltinProvider(builtinConfig, analysisLog)
	if err != nil {
		return nil, nil, err
	}

	a.log.Info("starting provider", "provider", "builtin")
	if _, err := builtinProvider.ProviderInit(ctx, additionalConfigs); err != nil {
		a.log.Error(err, "unable to init the builtin provider")
		return nil, nil, err
	}

	return builtinProvider, providerLocations, nil
}

func (a *analyzeCommand) setInternalProviders(finalConfigs []provider.Config, analysisLog logr.Logger) (map[string]provider.InternalProviderClient, []string) {
	providers := map[string]provider.InternalProviderClient{}
	providerLocations := []string{}

	for _, config := range finalConfigs {
		for _, ind := range config.InitConfig {
			providerLocations = append(providerLocations, ind.Location)
		}

		var prov provider.InternalProviderClient
		var err error

		// only create java and builtin providers
		if config.Name == util.JavaProvider {
			prov = a.setJavaProvider(config, analysisLog)
		} else if config.Name == "builtin" {
			prov, err = a.setBuiltinProvider(config, analysisLog)
			if err != nil {
				os.Exit(1)
			}
		}
		providers[config.Name] = prov
	}
	return providers, providerLocations
}

func (a *analyzeCommand) startProvidersContainerless(ctx context.Context, needProviders map[string]provider.InternalProviderClient) error {
	// Now that we have all the providers, we need to start them.
	additionalBuiltinConfigs := []provider.InitConfig{}
	for name, provider := range needProviders {
		a.log.Info("starting provider", "provider", name)
		switch name {
		// other providers can return additional configs for the builtin provider
		// therefore, we initiate builtin provider separately at the end
		case "builtin":
			continue
		default:
			initCtx, initSpan := tracing.StartNewSpan(ctx, "init",
				attribute.Key("provider").String(name))
			additionalBuiltinConfs, err := provider.ProviderInit(initCtx, nil)
			if err != nil {
				a.log.Error(err, "unable to init the providers", "provider", name)
				os.Exit(1)
			}
			if additionalBuiltinConfs != nil {
				additionalBuiltinConfigs = append(additionalBuiltinConfigs, additionalBuiltinConfs...)
			}
			initSpan.End()
		}
	}

	if builtinClient, ok := needProviders["builtin"]; ok {
		if _, err := builtinClient.ProviderInit(ctx, additionalBuiltinConfigs); err != nil {
			return err
		}
	}
	return nil
}

func (a *analyzeCommand) DependencyOutputContainerless(ctx context.Context, providers map[string]provider.InternalProviderClient, depOutputFile string, wg *sync.WaitGroup) {
	defer wg.Done()
	var depsFlat []konveyor.DepsFlatItem
	var depsTree []konveyor.DepsTreeItem
	var err error

	for _, prov := range providers {
		deps, err := prov.GetDependencies(ctx)
		if err != nil {
			a.log.Error(err, "failed to get list of dependencies for provider", "provider", "java")
		}
		for u, ds := range deps {
			newDeps := ds
			depsFlat = append(depsFlat, konveyor.DepsFlatItem{
				Provider:     "java",
				FileURI:      string(u),
				Dependencies: newDeps,
			})
		}

		if depsFlat == nil && depsTree == nil {
			a.log.V(4).Info("did not get dependencies from all given providers")
			return
		}
	}

	var by []byte
	// Sort depsFlat
	sort.SliceStable(depsFlat, func(i, j int) bool {
		if depsFlat[i].Provider == depsFlat[j].Provider {
			return depsFlat[i].FileURI < depsFlat[j].FileURI
		} else {
			return depsFlat[i].Provider < depsFlat[j].Provider
		}
	})

	by, err = yaml.Marshal(depsFlat)
	if err != nil {
		a.log.Error(err, "failed to marshal dependency data as yaml")
		return
	}

	err = os.WriteFile(filepath.Join(a.output, depOutputFile), by, 0644)
	if err != nil {
		a.log.Error(err, "failed to write dependencies to output file", "file", depOutputFile)
		return
	}

}

func (a *analyzeCommand) buildStaticReportFile(ctx context.Context, staticReportPath string, depsErr bool) error {
	// Prepare report args list with single input analysis
	applicationNames := []string{filepath.Base(a.input)}
	outputAnalyses := []string{filepath.Join(a.output, "output.yaml")}
	outputDeps := []string{filepath.Join(a.output, "dependencies.yaml")}
	outputJSPath := filepath.Join(staticReportPath, "output.js")

	if a.bulk {
		// Scan all available analysis output files to be reported
		applicationNames = nil
		outputAnalyses = nil
		outputDeps = nil
		outputFiles, err := filepath.Glob(filepath.Join(a.output, "output.yaml.*"))
		if err != nil {
			return err
		}
		for i := range outputFiles {
			outputName := filepath.Base(outputFiles[i])
			applicationName := strings.SplitN(outputName, "output.yaml.", 2)[1]
			applicationNames = append(applicationNames, applicationName)
			outputAnalyses = append(outputAnalyses, outputFiles[i])
			deps := fmt.Sprintf("%s.%s", filepath.Join(a.output, "dependencies.yaml"), applicationName)
			// If deps for given application are missing, empty the deps path allowing skip it in static-report
			if _, err := os.Stat(deps); errors.Is(err, os.ErrNotExist) {
				deps = ""
			}
			outputDeps = append(outputDeps, deps)
		}

	}

	if depsErr {
		outputDeps = []string{}
	}
	// create output.js file from analysis output.yaml
	apps, err := validateFlags(outputAnalyses, applicationNames, outputDeps, a.log)
	if err != nil {
		log.Fatalln("failed to validate flags", err)
	}

	err = loadApplications(apps)
	if err != nil {
		log.Fatalln("failed to load report data from analysis output", err)
	}

	err = generateJSBundle(apps, outputJSPath, a.log)
	if err != nil {
		log.Fatalln("failed to generate output.js file from template", err)
	}

	return nil
}

func (a *analyzeCommand) buildStaticReportOutput(ctx context.Context, log *os.File) error {
	outputFolderSrcPath := filepath.Join(a.kantraDir, "static-report")
	outputFolderDestPath := filepath.Join(a.output, "static-report")

	//copy static report files to output folder
	err := util.CopyFolderContents(outputFolderSrcPath, outputFolderDestPath)
	if err != nil {
		return err
	}
	return nil
}

func (a *analyzeCommand) GenerateStaticReportContainerless(ctx context.Context) error {
	if a.skipStaticReport {
		return nil
	}
	a.log.Info("generating static report")
	staticReportLogFilePath := filepath.Join(a.output, "static-report.log")
	staticReportLog, err := os.Create(staticReportLogFilePath)
	if err != nil {
		return fmt.Errorf("failed creating provider log file at %s", staticReportLogFilePath)
	}
	defer staticReportLog.Close()

	// it's possible for dependency analysis to fail
	// in this case we still want to generate a static report for successful source analysis
	_, noDepFileErr := os.Stat(filepath.Join(a.output, "dependencies.yaml"))
	if errors.Is(noDepFileErr, os.ErrNotExist) {
		a.log.Info("unable to get dependency output in static report. generating static report from source analysis only")

		// some other err
	} else if noDepFileErr != nil && !errors.Is(noDepFileErr, os.ErrNotExist) {
		return noDepFileErr
	}

	if a.bulk {
		a.moveResults()
	}

	staticReportAnalyzePath := filepath.Join(a.kantraDir, "static-report")
	err = a.buildStaticReportFile(ctx, staticReportAnalyzePath, errors.Is(noDepFileErr, os.ErrNotExist))
	if err != nil {
		return err
	}
	err = a.buildStaticReportOutput(ctx, staticReportLog)
	if err != nil {
		return err
	}
	uri := uri.File(filepath.Join(a.output, "static-report", "index.html"))
	a.log.Info("Static report created. Access it at this URL:", "URL", string(uri))

	return nil
}
