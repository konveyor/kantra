package kai

import (
	"testing"

	agenticv1alpha1 "github.com/konveyor/agentic-controller/api/v1alpha1"
)

func TestParseParamFlags(t *testing.T) {
	got, err := parseParamFlags([]string{"A=1", "B=hello=world", " C =x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["A"] != "1" || got["B"] != "hello=world" || got["C"] != "x" {
		t.Errorf("unexpected parse result: %#v", got)
	}
	if _, err := parseParamFlags([]string{"noequals"}); err == nil {
		t.Error("expected error for flag without '='")
	}
}

func TestValidateParamValue(t *testing.T) {
	tests := []struct {
		name    string
		typ     string
		value   string
		wantErr bool
	}{
		{"string anything", "string", "whatever", false},
		{"number ok", "number", "3.14", false},
		{"number bad", "number", "abc", true},
		{"boolean ok", "boolean", "true", false},
		{"boolean bad", "boolean", "yesish", true},
		{"empty skipped", "number", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := agenticv1alpha1.AgentParam{Name: "P", Type: agenticv1alpha1.AgentParamType(tt.typ)}
			err := validateParamValue(p, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateParamValue() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestResolveRunParams_NonInteractive covers the flag-driven (non-terminal) path.
func TestResolveRunParams_NonInteractive(t *testing.T) {
	if isInteractive() {
		t.Skip("test assumes a non-interactive environment")
	}

	declared := []agenticv1alpha1.AgentParam{
		{Name: "REQUIRED", Type: "string", Required: true},
		{Name: "WITH_DEFAULT", Type: "string", Default: "def"},
		{Name: "OPTIONAL_EMPTY", Type: "string"},
	}

	// Missing a required param must fail.
	if _, err := resolveRunParams(declared, map[string]string{}); err == nil {
		t.Error("expected error when required param missing")
	}

	// All provided/defaulted: required from flags, default applied, empty optional dropped.
	out, err := resolveRunParams(declared, map[string]string{"REQUIRED": "val"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := map[string]string{}
	for _, p := range out {
		got[p.Name] = p.Value
	}
	if got["REQUIRED"] != "val" {
		t.Errorf("expected REQUIRED=val, got %q", got["REQUIRED"])
	}
	if got["WITH_DEFAULT"] != "def" {
		t.Errorf("expected WITH_DEFAULT=def, got %q", got["WITH_DEFAULT"])
	}
	if _, ok := got["OPTIONAL_EMPTY"]; ok {
		t.Errorf("expected empty optional param to be dropped, got %q", got["OPTIONAL_EMPTY"])
	}
}
