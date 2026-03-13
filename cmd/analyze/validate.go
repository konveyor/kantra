package analyze

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

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
