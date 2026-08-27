package kai

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"
)

// errAborted is returned when the user cancels an interactive wizard.
var errAborted = errors.New("aborted")

// isInteractive reports whether both stdin and stdout are attached to a
// terminal, so the caller can decide whether to prompt.
func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// requireTerminal returns an error when stdin/stdout is not an interactive
// terminal, so wizards fail with a clear message instead of a cryptic one.
func requireTerminal() error {
	if !isInteractive() {
		return fmt.Errorf("interactive input requires a terminal; provide values via flags instead")
	}
	return nil
}

// runForm runs the given huh fields as a single form group, mapping a user
// cancellation to errAborted.
func runForm(fields ...huh.Field) error {
	form := huh.NewForm(huh.NewGroup(fields...))
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return errAborted
		}
		return err
	}
	return nil
}

// confirm asks a yes/no question, defaulting to def.
func confirm(title string, def bool) (bool, error) {
	if err := requireTerminal(); err != nil {
		return false, err
	}
	v := def
	err := runForm(huh.NewConfirm().Title(title).Value(&v))
	if err != nil {
		return false, err
	}
	return v, nil
}

// selectField builds a single-select field bound to val.
func selectField(title string, options []string, val *string) huh.Field {
	return huh.NewSelect[string]().
		Title(title).
		Options(huh.NewOptions(options...)...).
		Value(val)
}

// multiSelectField builds a multi-select field bound to val.
func multiSelectField(title string, options []string, val *[]string) huh.Field {
	return huh.NewMultiSelect[string]().
		Title(title).
		Options(huh.NewOptions(options...)...).
		Value(val)
}

// inputField builds a text input bound to val with an optional validator.
func inputField(title, placeholder string, val *string, validate func(string) error) huh.Field {
	f := huh.NewInput().Title(title).Value(val)
	if placeholder != "" {
		f = f.Placeholder(placeholder)
	}
	if validate != nil {
		f = f.Validate(validate)
	}
	return f
}

// requiredValidator returns a validator that rejects empty/whitespace input.
func requiredValidator(field string) func(string) error {
	return func(s string) error {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("%s is required", field)
		}
		return nil
	}
}

// passwordField builds a masked text input (password echo) bound to val, so
// secret values never render in the terminal.
func passwordField(title string, val *string, validate func(string) error) huh.Field {
	f := huh.NewInput().Title(title).EchoMode(huh.EchoModePassword).Value(val)
	if validate != nil {
		f = f.Validate(validate)
	}
	return f
}

// readableFileValidator rejects paths that are empty or not readable files.
// Used for credential fields supplied by file path (the path is not sensitive).
func readableFileValidator(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("file path is required")
	}
	info, err := os.Stat(s)
	if err != nil {
		return fmt.Errorf("cannot read file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("%q is a directory, not a file", s)
	}
	return nil
}
