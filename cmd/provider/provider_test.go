package provider

import (
	"bytes"
	"strings"
	"testing"

	"github.com/go-logr/logr"
)

func TestNewProviderCommand_list(t *testing.T) {
	cmd := NewProviderCommand(logr.Discard())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	out := cmd.OutOrStdout().(*bytes.Buffer).String()
	if !strings.Contains(out, "java") || !strings.Contains(out, "Container analysis supported providers:") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func Test_newListCommand(t *testing.T) {
	cmd := newListCommand()
	if cmd.Use != "list" {
		t.Fatalf("Use = %q", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Fatal("RunE must be set")
	}
}
