package analyze

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/devfile/alizer/pkg/apis/model"
)

func Test_listLanguages_success(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	errCh := make(chan error, 1)
	go func() {
		errCh <- listLanguages([]model.Language{{Name: "Java"}, {Name: "Python"}}, "/tmp/app")
		w.Close()
	}()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	os.Stdout = old
	if err := <-errCh; err != nil {
		t.Fatalf("listLanguages() error = %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		"found languages for input application:",
		"/tmp/app",
		"Java",
		"Python",
		"kantra provider list",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func Test_listLanguages_noLanguages(t *testing.T) {
	err := listLanguages(nil, "/tmp/app")
	if err == nil {
		t.Fatal("expected error when no languages detected")
	}
}
