package analyze

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/devfile/alizer/pkg/apis/model"
	"github.com/konveyor-ecosystem/kantra/cmd/internal/settings"
	"github.com/konveyor-ecosystem/kantra/pkg/container"
	outputv1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
)

func (a *analyzeCommand) ListAllProviders() {
	supportedProvsContainer := []string{
		"java",
		"python",
		"go",
		"csharp",
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

// parseLabelLines splits container output into label lines, trimming whitespace
// and skipping empty lines so comparison works across Docker/Podman
func parseLabelLines(raw string) []string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		s := strings.TrimSpace(line)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
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
			ListOptionsFromLabels(sourceSlice, sourceLabel, out)
			return nil
		}
		if listTargets {
			targetsSlice, err := a.readRuleFilesForLabels(targetLabel)
			if err != nil {
				a.log.Error(err, "failed to read rule labels")
				return err
			}
			ListOptionsFromLabels(targetsSlice, targetLabel, out)
			return nil
		}
	} else {
		volumes, err := a.getRulesVolumes()
		if err != nil {
			a.log.Error(err, "failed getting rules volumes")
			return err
		}

		if len(a.rules) > 0 {
			customRulePath = filepath.Join(container.CustomRulePath, a.tempRuleDir)
		}
		args := []string{"analyze", "--run-local=false"}
		if listSources {
			args = append(args, "--list-sources")
		} else {
			args = append(args, "--list-targets")
		}
		err = container.NewContainer().Run(
			ctx,
			container.WithImage(settings.Settings.RunnerImage),
			container.WithLog(a.log.V(1)),
			container.WithRuntimeArgs(a.containerRuntimeArgs()...),
			container.WithEnv(runMode, runModeContainer),
			container.WithEnv(rulePath, customRulePath),
			container.WithVolumes(volumes),
			container.WithEntrypointBin(fmt.Sprintf("/usr/local/bin/%s", settings.Settings.RootCommandName)),
			container.WithContainerToolBin(settings.Settings.ContainerBinary),
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
	err := filepath.WalkDir(container.RulesetPath, WalkRuleSets(container.RulesetPath, label, &labelsSlice))
	if err != nil {
		return nil, err
	}
	rulePath := os.Getenv("RULE_PATH")
	if rulePath != "" {
		err := filepath.WalkDir(rulePath, WalkRuleSets(rulePath, label, &labelsSlice))
		if err != nil {
			return nil, err
		}
	}
	return labelsSlice, nil
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
		ListOptionsFromLabels(sourceSlice, sourceLabel, out)
		return nil
	}
	if listTargets {
		targetsSlice, err := a.walkRuleFilesForLabelsContainerless(targetLabel)
		if err != nil {
			a.log.Error(err, "failed to read rule labels")
			return err
		}
		ListOptionsFromLabels(targetsSlice, targetLabel, out)
		return nil
	}

	return nil
}

func (a *analyzeCommand) walkRuleFilesForLabelsContainerless(label string) ([]string, error) {
	labelsSlice := []string{}
	path := filepath.Join(a.kantraDir, settings.RulesetsLocation)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		a.log.Error(err, "cannot open provided path")
		return nil, err
	}
	err := filepath.WalkDir(path, WalkRuleSets(path, label, &labelsSlice))
	if err != nil {
		return nil, err
	}
	if len(a.rules) > 0 {
		for _, p := range a.rules {
			err := filepath.WalkDir(p, WalkRuleSets(p, label, &labelsSlice))
			if err != nil {
				return nil, err
			}
		}
	}
	return labelsSlice, nil
}

func WalkRuleSets(root string, label string, labelsSlice *[]string) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d == nil || d.IsDir() {
			return nil
		}
		var readErr error
		*labelsSlice, readErr = readRuleFile(path, labelsSlice, label)
		return readErr
	}
}

func readRuleFile(filePath string, labelsSlice *[]string, label string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanWords)

	for scanner.Scan() {
		// add source/target labels to slice
		label := getSourceOrTargetLabel(scanner.Text(), label)
		if len(label) > 0 && !slices.Contains(*labelsSlice, label) {
			*labelsSlice = append(*labelsSlice, label)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return *labelsSlice, nil
}

func getSourceOrTargetLabel(text string, label string) string {
	if strings.Contains(text, label) {
		return text
	}
	return ""
}

func ListOptionsFromLabels(sl []string, label string, out io.Writer) {
	var newSl []string
	l := label + "="

	for _, label := range sl {
		newSt := strings.TrimPrefix(label, l)

		if newSt != label {
			newSt = strings.TrimSuffix(newSt, "+")
			newSt = strings.TrimSuffix(newSt, "-")

			if !slices.Contains(newSl, newSt) {
				newSl = append(newSl, newSt)

			}
		}
	}
	sort.Strings(newSl)

	if label == outputv1.SourceTechnologyLabel {
		fmt.Fprintln(out, "available source technologies:")
	} else {
		fmt.Fprintln(out, "available target technologies:")
	}
	for _, tech := range newSl {
		fmt.Fprintln(out, tech)
	}
}
