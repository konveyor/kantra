package kai

import "strings"

// The agentic-controller Gateway CRD requires spec.model.contextWindow
// (Minimum=1) but nothing in the controller or harness actually reads it today
// — goose derives each model's real context window internally. The field is
// currently inert metadata reserved for a future "do the always-loaded rules
// fit the budget?" validation. kantra therefore just needs to supply a
// plausible positive value so users don't have to think about it: we look the
// model up in this table and fall back to fallbackContextWindow when unknown.
//
// This table is kantra-owned and will need occasional updates as models ship;
// values are the models' documented maximum input context windows in tokens.
var modelContextWindows = []struct {
	prefix string
	window int64
}{
	// Anthropic Claude — 200K across the current lineup.
	{"claude-", 200000},

	// OpenAI.
	{"gpt-4.1", 1000000},
	{"gpt-4o", 128000},
	{"gpt-4-turbo", 128000},
	{"gpt-4", 8192},
	{"gpt-3.5", 16385},
	{"o1", 200000},
	{"o3", 200000},
	{"o4", 200000},

	// Google Gemini.
	{"gemini-1.5-pro", 2000000},
	{"gemini-1.5-flash", 1000000},
	{"gemini-1.5", 1000000},
	{"gemini-2.5", 1048576},
	{"gemini-2.0", 1048576},
	{"gemini-2", 1048576},
	{"gemini", 1000000},

	// xAI Grok.
	{"grok-4", 256000},
	{"grok-3", 131072},
	{"grok-2", 131072},
	{"grok", 131072},
}

// fallbackContextWindow is used when the model name matches no known prefix.
const fallbackContextWindow int64 = 200000

// defaultContextWindow returns the documented context window for the given
// model name, matching by longest known prefix. The boolean is false when the
// model is unrecognized (the caller should fall back and inform the user).
func defaultContextWindow(model string) (int64, bool) {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return 0, false
	}
	best := ""
	var window int64
	for _, e := range modelContextWindows {
		if strings.HasPrefix(m, e.prefix) && len(e.prefix) > len(best) {
			best = e.prefix
			window = e.window
		}
	}
	if best == "" {
		return 0, false
	}
	return window, true
}
