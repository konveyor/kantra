package labels

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/container"
	outputv1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
)

func writeRuleset(t *testing.T, kantraDir string, relPath, content string) {
	t.Helper()
	full := filepath.Join(kantraDir, BundledRulesetsDir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNewListerFromAnalyze_runLocal(t *testing.T) {
	tmp := t.TempDir()
	writeRuleset(t, tmp, "cloud/rules.yaml", `
labels:
- `+outputv1.TargetTechnologyLabel+`=cloud-readiness
ruleID: rule-1
`)

	lister := NewListerFromAnalyze(AnalyzeListerOptions{
		Log:       logr.Discard(),
		KantraDir: tmp,
		RunLocal:  true,
	})
	if lister.cfg.Hybrid != nil {
		t.Fatal("expected no hybrid config when RunLocal is true")
	}
}

func TestNewListerFromAnalyze_hybrid(t *testing.T) {
	lister := NewListerFromAnalyze(AnalyzeListerOptions{
		Log:         logr.Discard(),
		KantraDir:   t.TempDir(),
		RunLocal:    false,
		RunnerImage: "quay.io/konveyor/kantra:latest",
	})
	if lister.cfg.Hybrid == nil {
		t.Fatal("expected hybrid config when RunLocal is false")
	}
}

func TestLister_ListTargets_containerless(t *testing.T) {
	tmp := t.TempDir()
	writeRuleset(t, tmp, "java/rules.yaml", `
labels:
- `+outputv1.TargetTechnologyLabel+`=java
- `+outputv1.TargetTechnologyLabel+`=quarkus
ruleID: rule-1
`)

	lister := New(Config{Log: logr.Discard(), KantraDir: tmp})
	var out bytes.Buffer
	if err := lister.ListTargets(context.Background(), &out); err != nil {
		t.Fatalf("ListTargets() error = %v", err)
	}
	body := out.String()
	if !strings.Contains(body, "available target technologies:") {
		t.Fatalf("output = %q", body)
	}
	if !strings.Contains(body, "java") || !strings.Contains(body, "quarkus") {
		t.Fatalf("output = %q", body)
	}
}

func TestLister_ListSources_containerless(t *testing.T) {
	tmp := t.TempDir()
	writeRuleset(t, tmp, "sources/rules.yaml", `
labels:
- `+outputv1.SourceTechnologyLabel+`=eap7
ruleID: rule-1
`)

	lister := New(Config{Log: logr.Discard(), KantraDir: tmp})
	var out bytes.Buffer
	if err := lister.ListSources(context.Background(), &out); err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	if !strings.Contains(out.String(), "available source technologies:") {
		t.Fatalf("output = %q", out.String())
	}
	if !strings.Contains(out.String(), "eap7") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestLister_ListTargets_missingRulesetsDir(t *testing.T) {
	lister := New(Config{Log: logr.Discard(), KantraDir: t.TempDir()})
	err := lister.ListTargets(context.Background(), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error when rulesets dir is missing")
	}
	if !strings.Contains(err.Error(), "missing kantra dependency") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLister_listInsideRunnerContainer(t *testing.T) {
	if _, err := os.Stat(container.RulesetPath); err != nil {
		t.Skipf("bundled rulesets not at %s: %v", container.RulesetPath, err)
	}

	t.Setenv("RUN_MODE", "container")
	t.Cleanup(func() { os.Unsetenv("RUN_MODE") })

	lister := New(Config{Log: logr.Discard(), Hybrid: &HybridConfig{}})
	var out bytes.Buffer
	if err := lister.ListTargets(context.Background(), &out); err != nil {
		t.Fatalf("ListTargets() error = %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("expected non-empty output from bundled rulesets")
	}
}

func TestNew_nilLoggerUsesDiscard(t *testing.T) {
	lister := New(Config{})
	// Must not panic when logging during list operations.
	lister.cfg.Log.Info("init")
	if lister == nil {
		t.Fatal("expected lister")
	}
}
