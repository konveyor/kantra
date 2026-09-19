package labels

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	outputv1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
)

func TestParseLabelLines(t *testing.T) {
	got := ParseLabelLines("  java \n\nspring-boot\n ")
	want := []string{"java", "spring-boot"}
	if len(got) != len(want) {
		t.Fatalf("ParseLabelLines() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ParseLabelLines()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWalkRuleSets_collectsTargetLabels(t *testing.T) {
	dir := t.TempDir()
	ruleFile := filepath.Join(dir, "rules.yaml")
	content := "labels:\n- " + outputv1.TargetTechnologyLabel + "=quarkus\n- " + outputv1.TargetTechnologyLabel + "=java\n"
	if err := os.WriteFile(ruleFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var labels []string
	if err := filepath.WalkDir(dir, WalkRuleSets(dir, outputv1.TargetTechnologyLabel, &labels)); err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
	if !containsAll(labels, outputv1.TargetTechnologyLabel+"=quarkus", outputv1.TargetTechnologyLabel+"=java") {
		t.Fatalf("labels = %v", labels)
	}
}

func TestListOptionsFromLabels_targets(t *testing.T) {
	var buf bytes.Buffer
	sl := []string{
		outputv1.TargetTechnologyLabel + "=java",
		outputv1.TargetTechnologyLabel + "=spring-boot",
		outputv1.TargetTechnologyLabel + "=java",
	}
	ListOptionsFromLabels(sl, outputv1.TargetTechnologyLabel, &buf)

	out := buf.String()
	if !strings.Contains(out, "available target technologies:") {
		t.Fatalf("missing header: %s", out)
	}
	if !strings.Contains(out, "java") || !strings.Contains(out, "spring-boot") {
		t.Fatalf("missing technologies: %s", out)
	}
}

func TestListOptionsFromLabels_sources(t *testing.T) {
	var buf bytes.Buffer
	ListOptionsFromLabels(
		[]string{outputv1.SourceTechnologyLabel + "=eap7+"},
		outputv1.SourceTechnologyLabel,
		&buf,
	)
	if !strings.Contains(buf.String(), "available source technologies:") {
		t.Fatalf("output = %s", buf.String())
	}
	if !strings.Contains(buf.String(), "eap7") {
		t.Fatalf("output = %s", buf.String())
	}
}

func TestDepLabelSelectorForAnalysis(t *testing.T) {
	t.Parallel()

	excludeOSS := "!" + provider.DepSourceLabel + "=open-source"
	includeAll := provider.DepSourceLabel + "=open-source || !" + provider.DepSourceLabel + "=open-source"

	tests := []struct {
		name                  string
		analyzeKnownLibraries bool
		want                  string
	}{
		{
			name:                  "exclude open source by default",
			analyzeKnownLibraries: false,
			want:                  excludeOSS,
		},
		{
			name:                  "include known libraries uses tautology",
			analyzeKnownLibraries: true,
			want:                  includeAll,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := DepLabelSelectorForAnalysis(tt.analyzeKnownLibraries); got != tt.want {
				t.Errorf("DepLabelSelectorForAnalysis() = %q, want %q", got, tt.want)
			}
		})
	}
}

func containsAll(slice []string, items ...string) bool {
	for _, item := range items {
		found := false
		for _, s := range slice {
			if s == item {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
