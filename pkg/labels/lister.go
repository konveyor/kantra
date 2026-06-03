package labels

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/container"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	outputv1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
)

// BundledRulesetsDir is the subdirectory under KantraDir that holds default rulesets.
const BundledRulesetsDir = "rulesets"

// Config configures listing of source/target technology labels from rulesets.
// When Hybrid is non-nil, listing runs via the kantra runner container; otherwise rules are walked on the host.
type Config struct {
	Log       logr.Logger
	KantraDir string
	Rules     []string
	Hybrid    *HybridConfig
}

// RulesVolumesFunc stages custom --rules paths for container mounts (implemented by cmd/analyze).
type RulesVolumesFunc func(log logr.Logger, rules []string) (volumes map[string]string, customRuleDir string, err error)

// HybridConfig is used when listing via the runner image (analyze hybrid mode).
type HybridConfig struct {
	Cleanup               bool
	HTTPProxy             string
	HTTPSProxy            string
	NoProxy              string
	ContainerRuntimeArgs []string
	RunnerImage           string
	RootCommandName       string
	ContainerBinary       string
	PrepareRulesVolumes   RulesVolumesFunc
}

// AnalyzeListerOptions configures rule label listing with the same settings as kantra analyze.
type AnalyzeListerOptions struct {
	Log       logr.Logger
	KantraDir string
	Rules     []string
	RunLocal  bool

	Cleanup               bool
	HTTPProxy             string
	HTTPSProxy            string
	NoProxy              string
	ContainerRuntimeArgs []string

	RunnerImage     string
	RootCommandName string
	ContainerBinary string

	PrepareRulesVolumes RulesVolumesFunc
}

// NewListerFromAnalyze builds a Lister from analyze command settings.
func NewListerFromAnalyze(o AnalyzeListerOptions) *Lister {
	cfg := Config{
		Log:       o.Log,
		KantraDir: o.KantraDir,
		Rules:     o.Rules,
	}
	if !o.RunLocal {
		cfg.Hybrid = &HybridConfig{
			Cleanup:               o.Cleanup,
			HTTPProxy:             o.HTTPProxy,
			HTTPSProxy:            o.HTTPSProxy,
			NoProxy:              o.NoProxy,
			ContainerRuntimeArgs: o.ContainerRuntimeArgs,
			RunnerImage:           o.RunnerImage,
			RootCommandName:       o.RootCommandName,
			ContainerBinary:       o.ContainerBinary,
			PrepareRulesVolumes:   o.PrepareRulesVolumes,
		}
	}
	return New(cfg)
}

// Lister lists konveyor.io/source and konveyor.io/target labels from bundled and custom rules.
type Lister struct {
	cfg Config
}

// New returns a Lister for cfg.
func New(cfg Config) *Lister {
	if cfg.Log.GetSink() == nil {
		cfg.Log = logr.Discard()
	}
	return &Lister{cfg: cfg}
}

// ListSources writes available source technologies to out.
func (l *Lister) ListSources(ctx context.Context, out io.Writer) error {
	return l.list(ctx, out, true, false)
}

// ListTargets writes available target technologies to out.
func (l *Lister) ListTargets(ctx context.Context, out io.Writer) error {
	return l.list(ctx, out, false, true)
}

func (l *Lister) list(ctx context.Context, out io.Writer, listSources, listTargets bool) error {
	if l.cfg.Hybrid != nil {
		return l.listHybrid(ctx, out, listSources, listTargets)
	}
	return l.listContainerless(out, listSources, listTargets)
}

func (l *Lister) listContainerless(out io.Writer, listSources, listTargets bool) error {
	sourceLabel := outputv1.SourceTechnologyLabel
	targetLabel := outputv1.TargetTechnologyLabel

	if listSources {
		sourceSlice, err := l.collectLabelsContainerless(sourceLabel)
		if err != nil {
			return err
		}
		ListOptionsFromLabels(sourceSlice, sourceLabel, out)
		return nil
	}
	if listTargets {
		targetsSlice, err := l.collectLabelsContainerless(targetLabel)
		if err != nil {
			return err
		}
		ListOptionsFromLabels(targetsSlice, targetLabel, out)
		return nil
	}
	return nil
}

func (l *Lister) collectLabelsContainerless(label string) ([]string, error) {
	labelsSlice := []string{}
	if err := util.CheckKantraSubpath(l.cfg.KantraDir, BundledRulesetsDir); err != nil {
		return nil, err
	}
	path := filepath.Join(l.cfg.KantraDir, BundledRulesetsDir)
	if err := filepath.WalkDir(path, WalkRuleSets(path, label, &labelsSlice)); err != nil {
		return nil, err
	}
	for _, p := range l.cfg.Rules {
		if err := filepath.WalkDir(p, WalkRuleSets(p, label, &labelsSlice)); err != nil {
			return nil, err
		}
	}
	return labelsSlice, nil
}

func (l *Lister) listHybrid(ctx context.Context, out io.Writer, listSources, listTargets bool) error {
	runMode := "RUN_MODE"
	runModeContainer := "container"
	rulePathEnv := "RULE_PATH"

	if os.Getenv(runMode) == runModeContainer {
		return l.listInsideRunnerContainer(out, listSources, listTargets)
	}

	runtimeArgs := l.cfg.Hybrid.ContainerRuntimeArgs

	var volumes map[string]string
	var customRuleDir string
	if l.cfg.Hybrid.PrepareRulesVolumes != nil {
		var volErr error
		volumes, customRuleDir, volErr = l.cfg.Hybrid.PrepareRulesVolumes(l.cfg.Log, l.cfg.Rules)
		if volErr != nil {
			l.cfg.Log.Error(volErr, "failed getting rules volumes")
			return volErr
		}
	}

	customRulePath := ""
	if customRuleDir != "" {
		customRulePath = filepath.Join(container.CustomRulePath, customRuleDir)
	}

	args := []string{"rules"}
	if listSources {
		args = append(args, "list-sources")
	} else {
		args = append(args, "list-targets")
	}

	h := l.cfg.Hybrid
	err := container.NewContainer().Run(
		ctx,
		container.WithImage(h.RunnerImage),
		container.WithLog(l.cfg.Log.V(1)),
		container.WithRuntimeArgs(runtimeArgs...),
		container.WithEnv(runMode, runModeContainer),
		container.WithEnv(rulePathEnv, customRulePath),
		container.WithVolumes(volumes),
		container.WithEntrypointBin(fmt.Sprintf("/usr/local/bin/%s", h.RootCommandName)),
		container.WithContainerToolBin(h.ContainerBinary),
		container.WithEntrypointArgs(args...),
		container.WithStdout(out),
		container.WithCleanup(h.Cleanup),
		container.WithProxy(h.HTTPProxy, h.HTTPSProxy, h.NoProxy),
	)
	if err != nil {
		l.cfg.Log.Error(err, "failed listing labels")
		return err
	}
	return nil
}

func (l *Lister) listInsideRunnerContainer(out io.Writer, listSources, listTargets bool) error {
	sourceLabel := outputv1.SourceTechnologyLabel
	targetLabel := outputv1.TargetTechnologyLabel

	if listSources {
		sourceSlice, err := l.collectLabelsFromMountedRules(sourceLabel)
		if err != nil {
			l.cfg.Log.Error(err, "failed to read rule labels")
			return err
		}
		ListOptionsFromLabels(sourceSlice, sourceLabel, out)
		return nil
	}
	if listTargets {
		targetsSlice, err := l.collectLabelsFromMountedRules(targetLabel)
		if err != nil {
			l.cfg.Log.Error(err, "failed to read rule labels")
			return err
		}
		ListOptionsFromLabels(targetsSlice, targetLabel, out)
		return nil
	}
	return nil
}

func (l *Lister) collectLabelsFromMountedRules(label string) ([]string, error) {
	labelsSlice := []string{}
	if err := filepath.WalkDir(container.RulesetPath, WalkRuleSets(container.RulesetPath, label, &labelsSlice)); err != nil {
		return nil, err
	}
	rulePath := os.Getenv("RULE_PATH")
	if rulePath != "" {
		if err := filepath.WalkDir(rulePath, WalkRuleSets(rulePath, label, &labelsSlice)); err != nil {
			return nil, err
		}
	}
	return labelsSlice, nil
}
