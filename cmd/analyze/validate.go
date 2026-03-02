package analyze

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/konveyor-ecosystem/kantra/cmd/internal/settings"
	"github.com/konveyor-ecosystem/kantra/pkg/profile"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/spf13/cobra"
)

func (a *analyzeCommand) Validate(ctx context.Context, cmd *cobra.Command, foundProfile *profile.AnalysisProfile) error {
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
		knownSources := parseLabelLines(sourcesRaw.String())
		for _, source := range a.sources {
			found := false
			for _, knownSource := range knownSources {
				if source == knownSource {
					found = true
					break
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
		knownTargets := parseLabelLines(targetRaw.String())
		for _, target := range a.targets {
			found := false
			for _, knownTarget := range knownTargets {
				if target == knownTarget {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("unknown target: \"%s\"", target)
			}
		}
	}

	if a.input != "" {
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
			a.isFileInput = true
		}
	}
	// Compute the resolved source path inside the container.
	// For directory inputs this is the standard mount path; for file inputs
	// (binaries) it includes the filename so the provider sees the file.
	if a.isFileInput {
		a.sourceLocationPath = path.Join(util.SourceMountPath, filepath.Base(a.input))
	} else {
		a.sourceLocationPath = util.SourceMountPath
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
				return fmt.Errorf("dependency folder %v is not a directory", a.depFolders[i])
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

func (a *analyzeCommand) ValidateAndLoadProfile() (*profile.AnalysisProfile, error) {
	if a.profileDir != "" {
		stat, err := os.Stat(a.profileDir)
		if err != nil {
			return nil, fmt.Errorf("failed to stat profiles directory %s: %w", a.profileDir, err)
		}
		if !stat.IsDir() {
			return nil, fmt.Errorf("found profiles path %s is not a directory", a.profileDir)
		}
		a.profilePath = filepath.Join(a.profileDir, "profile.yaml")
	} else if a.input != "" {
		stat, err := os.Stat(a.input)
		if err != nil {
			return nil, err
		}
		if !stat.IsDir() {
			return nil, nil
		}
		profilesDir := filepath.Join(a.input, profile.Profiles)
		foundPath, err := profile.FindSingleProfile(profilesDir)
		if err != nil {
			return nil, err
		}
		if foundPath != "" {
			a.profilePath = foundPath
		}
	}
	if a.profilePath == "" {
		return nil, nil
	}
	foundProfile, err := profile.UnmarshalProfile(a.profilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load profile %s: %w", a.profilePath, err)
	}

	return foundProfile, nil
}

func (a *analyzeCommand) validateProviders(providers []string) error {
	validProvs := []string{
		util.JavaProvider,
		util.PythonProvider,
		util.GoProvider,
		util.NodeJSProvider,
		util.CsharpProvider,
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
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".yaml" && ext != ".yml" {
				a.log.V(1).Info("skipping non-YAML file in rules directory", "file", path)
			}
			return nil
		})
	} else {
		ext := strings.ToLower(filepath.Ext(rulePath))
		if ext != ".yaml" && ext != ".yml" {
			a.log.V(1).Info("skipping non-YAML rule file", "file", rulePath)
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
	requiredDirs := []string{a.kantraDir, filepath.Join(a.kantraDir, settings.RulesetsLocation), filepath.Join(a.kantraDir, settings.JavaBundlesLocation),
		filepath.Join(a.kantraDir, settings.JDTLSBinLocation), filepath.Join(a.kantraDir, "fernflower.jar")}
	for _, path := range requiredDirs {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			a.log.Error(err, "cannot open required path, ensure that container-less dependencies are installed")
			return err
		}
	}

	return nil
}

func (a *analyzeCommand) validateProviderConfig() error {
	// Validate override provider settings file if specified
	if a.overrideProviderSettings != "" {
		if _, err := os.Stat(a.overrideProviderSettings); err != nil {
			return fmt.Errorf(
				"Override provider settings file not found: %s\n"+
					"Specified with --override-provider-settings flag but file does not exist",
				a.overrideProviderSettings)
		}
		a.log.V(1).Info("Override provider settings file validated", "path", a.overrideProviderSettings)
	}

	// Check if any provider ports are already in use
	for provName, provInit := range a.providersMap {
		address := fmt.Sprintf("localhost:%d", provInit.port)
		listener, err := net.Listen("tcp", address)
		if err != nil {
			// Port is already in use
			return fmt.Errorf(
				"port %d required for %s provider is already in use\n"+
					"Troubleshooting:\n"+
					"  1. Check what's using the port: lsof -i :%d\n"+
					"  2. Stop old provider containers: podman stop $(podman ps -a | grep provider | awk '{print $1}')\n"+
					"  3. Kill the process using the port, or restart your system",
				provInit.port, provName, provInit.port)
		}
		listener.Close()
		a.log.V(2).Info("port is available", "provider", provName, "port", provInit.port)
	}

	a.log.V(1).Info("provider configuration validated successfully")
	return nil
}
