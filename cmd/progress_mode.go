package cmd

import (
	"fmt"
	"os"

	"github.com/go-logr/logr"
)

// ProgressMode encapsulates progress reporting behavior.
// It provides methods to conditionally execute UI operations based on whether
// progress reporting is enabled or disabled.
type ProgressMode struct {
	disabled bool
}

// NewProgressMode creates a ProgressMode from the noProgress flag.
func NewProgressMode(noProgress bool) *ProgressMode {
	return &ProgressMode{disabled: noProgress}
}

// IsDisabled returns true if progress reporting is disabled (--no-progress flag was set).
func (p *ProgressMode) IsDisabled() bool {
	return p.disabled
}

// IsEnabled returns true if progress reporting is enabled.
func (p *ProgressMode) IsEnabled() bool {
	return !p.disabled
}

// OperationalLogger returns either the given logger (if progress disabled) or logr.Discard() (if progress enabled).
// This prevents operational messages from interfering with progress bars when progress is enabled.
func (p *ProgressMode) OperationalLogger(log logr.Logger) logr.Logger {
	if p.disabled {
		return log
	}
	return logr.Discard()
}

// ShouldAddConsoleHook returns true if the console hook should be added to logrus.
// Console hooks are only added when progress is disabled to avoid interfering with progress bars.
func (p *ProgressMode) ShouldAddConsoleHook() bool {
	return p.disabled
}

// HideCursor hides the terminal cursor if progress is enabled.
func (p *ProgressMode) HideCursor() {
	if p.IsEnabled() {
		fmt.Fprintf(os.Stderr, "\033[?25l")
	}
}

// ShowCursor shows the terminal cursor if progress is enabled.
func (p *ProgressMode) ShowCursor() {
	if p.IsEnabled() {
		fmt.Fprintf(os.Stderr, "\033[?25h")
	}
}

// Printf writes formatted output to stderr if progress is enabled.
// When progress is disabled, this is a no-op to avoid cluttering the output.
func (p *ProgressMode) Printf(format string, args ...interface{}) {
	if p.IsEnabled() {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}

// Println writes a line to stderr if progress is enabled.
// When progress is disabled, this is a no-op to avoid cluttering the output.
func (p *ProgressMode) Println(args ...interface{}) {
	if p.IsEnabled() {
		fmt.Fprintln(os.Stderr, args...)
	}
}
