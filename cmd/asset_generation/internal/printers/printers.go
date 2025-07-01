package printers

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Output handles outputting content either to stdout or to files
type Output struct {
	out io.Writer
}

// NewOutput creates a new Output instance with the given writer
func NewOutput(out io.Writer) *Output {
	return &Output{out: out}
}

// ToStdout outputs content to stdout with YAML-style formatting including source comment
func (o *Output) ToStdout(filename, contents string) error {
	_, err := fmt.Fprint(o.out, contents)
	return err
}

// ToFile writes content to a file in the specified output directory
func ToFile(outputDir, filename, contents string) error {
	fn := filepath.Base(filename)
	dst := filepath.Join(outputDir, fn)
	// Add an extra line to make it yaml compliant since helm doesn't seem to do
	// it.
	if !strings.HasSuffix(contents, "\n") {
		contents = fmt.Sprintln(contents)
	}
	return os.WriteFile(dst, []byte(contents), 0644)
}

// ToStdoutWithHeader outputs content to stdout with a custom header
func (o *Output) ToStdoutWithHeader(header, contents string) error {
	fmt.Fprintf(o.out, "%s%s", header, contents)
	return nil
}
