package kai

import "testing"

func TestDefaultContextWindow(t *testing.T) {
	tests := []struct {
		model      string
		wantWindow int64
		wantOK     bool
	}{
		{"claude-sonnet-4-5-20250929", 200000, true},
		{"claude-3-5-haiku-latest", 200000, true},
		{"gpt-4o", 128000, true},
		{"gpt-4o-mini", 128000, true},
		{"gpt-4.1", 1000000, true},
		{"gpt-4-turbo", 128000, true},
		{"gpt-4", 8192, true},
		{"o3-mini", 200000, true},
		{"gemini-1.5-pro", 2000000, true},   // longest-prefix wins over gemini-1.5
		{"gemini-1.5-flash", 1000000, true}, // longest-prefix wins over gemini-1.5
		{"gemini-2.5-pro", 1048576, true},
		{"grok-4-latest", 256000, true},
		{"GPT-4O", 128000, true}, // case-insensitive
		{"  claude-opus-4  ", 200000, true},
		{"some-unknown-model", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got, ok := defaultContextWindow(tt.model)
			if ok != tt.wantOK {
				t.Fatalf("defaultContextWindow(%q) ok = %v, want %v", tt.model, ok, tt.wantOK)
			}
			if got != tt.wantWindow {
				t.Errorf("defaultContextWindow(%q) = %d, want %d", tt.model, got, tt.wantWindow)
			}
		})
	}
}
