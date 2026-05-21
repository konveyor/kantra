package analyze

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-logr/logr"
)

func Test_labelsLister_listTargets(t *testing.T) {
	tmp := t.TempDir()
	rulesDir := filepath.Join(tmp, "rulesets", "targets")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "rules.yaml"),
		[]byte("labels:\n- konveyor.io/target=cloud-readiness\nruleID: r1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := &analyzeCommand{
		runLocal: true,
		AnalyzeCommandContext: AnalyzeCommandContext{
			log:       logr.Discard(),
			kantraDir: tmp,
		},
	}

	var out bytes.Buffer
	if err := a.labelsLister().ListTargets(context.Background(), &out); err != nil {
		t.Fatalf("ListTargets() error = %v", err)
	}
	if !strings.Contains(out.String(), "cloud-readiness") {
		t.Fatalf("output = %q", out.String())
	}
}
