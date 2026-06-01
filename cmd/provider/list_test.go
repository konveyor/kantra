package provider

import (
	"bytes"
	"strings"
	"testing"
)

func TestListProviders(t *testing.T) {
	var buf bytes.Buffer
	if err := ListProviders(&buf); err != nil {
		t.Fatalf("ListProviders() error = %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"Container analysis supported providers:",
		"java",
		"python",
		"Containerless analysis supported providers (default):",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("ListProviders() output missing %q:\n%s", want, out)
		}
	}
}
