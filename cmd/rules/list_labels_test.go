package rules

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/container"
)

func TestNewRuleLabelsLister_RunModeContainer_usesBundledRulesets(t *testing.T) {
	t.Setenv(runModeEnv, runModeContainer)
	t.Cleanup(func() { os.Unsetenv(runModeEnv) })

	lister, err := newRuleLabelsLister(logr.Discard())
	if err != nil {
		t.Fatalf("newRuleLabelsLister() error = %v", err)
	}

	err = lister.ListTargets(context.Background(), &bytes.Buffer{})
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), ".kantra") {
		t.Fatalf("runner container listing should use bundled rulesets, not kantra dir: %v", err)
	}
	if !strings.Contains(err.Error(), container.RulesetPath) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewRuleLabelsLister_RunModeContainer_listsLabels(t *testing.T) {
	if _, err := os.Stat(container.RulesetPath); err != nil {
		t.Skipf("bundled rulesets not present at %s: %v", container.RulesetPath, err)
	}

	t.Setenv(runModeEnv, runModeContainer)
	t.Cleanup(func() { os.Unsetenv(runModeEnv) })

	lister, err := newRuleLabelsLister(logr.Discard())
	if err != nil {
		t.Fatalf("newRuleLabelsLister() error = %v", err)
	}

	var out bytes.Buffer
	if err := lister.ListTargets(context.Background(), &out); err != nil {
		t.Fatalf("ListTargets() error = %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("ListTargets() produced no output")
	}
}

func TestNewRuleLabelsLister_withoutRunMode_usesKantraDir(t *testing.T) {
	os.Unsetenv(runModeEnv)

	tmp := t.TempDir()
	for _, name := range []string{"rulesets", "jdtls", "static-report"} {
		if err := os.Mkdir(filepath.Join(tmp, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("KANTRA_DIR", tmp)
	t.Cleanup(func() { os.Unsetenv("KANTRA_DIR") })

	lister, err := newRuleLabelsLister(logr.Discard())
	if err != nil {
		t.Fatalf("newRuleLabelsLister() error = %v", err)
	}
	if lister == nil {
		t.Fatal("expected lister")
	}

	err = lister.ListTargets(context.Background(), &bytes.Buffer{})
	if err != nil && strings.Contains(err.Error(), container.RulesetPath) {
		t.Fatalf("host listing should use kantra dir rulesets, not %s: %v", container.RulesetPath, err)
	}
}
