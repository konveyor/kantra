package rules

import (
	"testing"
	"strings"

	"github.com/go-logr/logr"
)

func TestNewRulesCommand(t *testing.T) {
	cmd := NewRulesCommand(logr.Discard())
	if cmd.Use != "rules" {
		t.Fatalf("Use = %q", cmd.Use)
	}

	names := make([]string, 0, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	for _, want := range []string{"list-sources", "list-targets", "test"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing subcommand %q, have %v", want, names)
		}
	}
}

func Test_newListSourcesCommand(t *testing.T) {
	cmd := newListSourcesCommand(logr.Discard())
	if cmd.Use != "list-sources" || cmd.RunE == nil {
		t.Fatalf("list-sources command not wired: %+v", cmd)
	}
}

func Test_newListTargetsCommand(t *testing.T) {
	cmd := newListTargetsCommand(logr.Discard())
	if cmd.Use != "list-targets" || cmd.RunE == nil {
		t.Fatalf("list-targets command not wired: %+v", cmd)
	}
}

func Test_newRulesTestCommand(t *testing.T) {
	cmd := newRulesTestCommand(logr.Discard())
	if cmd.Use != "test [paths...]" {
		t.Fatalf("Use = %q", cmd.Use)
	}
}

func TestNewLegacyTestCommand(t *testing.T) {
	cmd := NewLegacyTestCommand(logr.Discard())
	if cmd.Use != "test" {
		t.Fatalf("Use = %q", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Fatal("expected RunE to be set")
	}
	if !strings.Contains(cmd.Short, "DEPRECATED") {
		t.Fatalf("expected DEPRECATED in Short: %q", cmd.Short)
	}
}
