package analyze

import (
	"os"
	"strings"
	"testing"
)

func TestShouldFilterLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		filtered bool
	}{
		{
			name:     "exact Windows watcher message",
			line:     "Windows system assumed buffer larger than it is, events have likely been missed",
			filtered: true,
		},
		{
			name:     "message with timestamp prefix",
			line:     "2024/12/09 11:39:26 Error in Watcher Error channel: Windows system assumed buffer larger than it is, events have likely been missed",
			filtered: true,
		},
		{
			name:     "normal log line passes through",
			line:     `time="2024-12-09T11:36:08-08:00" level=info msg="running source analysis"`,
			filtered: false,
		},
		{
			name:     "empty line passes through",
			line:     "",
			filtered: false,
		},
		{
			name:     "partial match does not filter",
			line:     "Windows system assumed buffer",
			filtered: false,
		},
		{
			name:     "unrelated error passes through",
			line:     "Error in Watcher Error channel: some other error",
			filtered: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldFilterLine(tt.line)
			if got != tt.filtered {
				t.Errorf("shouldFilterLine(%q) = %v, want %v", tt.line, got, tt.filtered)
			}
		})
	}
}

func TestFilterStderr(t *testing.T) {
	// Create pipes to simulate reading and writing
	inputR, inputW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outputR, outputW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	// Write test lines and close the write end
	lines := []string{
		"normal line 1",
		"2024/12/09 11:39:26 Error in Watcher Error channel: Windows system assumed buffer larger than it is, events have likely been missed",
		"normal line 2",
		"Windows system assumed buffer larger than it is, events have likely been missed",
		"normal line 3",
	}
	go func() {
		for _, line := range lines {
			inputW.WriteString(line + "\n")
		}
		inputW.Close()
	}()

	// Run the filter
	filterStderr(inputR, outputW)
	outputW.Close()

	// Read the filtered output
	buf := make([]byte, 4096)
	n, _ := outputR.Read(buf)
	output := string(buf[:n])
	outputR.Close()
	inputR.Close()

	// Verify only non-filtered lines remain
	if strings.Contains(output, "Windows system assumed buffer larger than it is") {
		t.Error("filtered pattern should not appear in output")
	}
	for _, expected := range []string{"normal line 1", "normal line 2", "normal line 3"} {
		if !strings.Contains(output, expected) {
			t.Errorf("expected %q in output, got: %q", expected, output)
		}
	}
}

func TestFilterStderrEmptyInput(t *testing.T) {
	inputR, inputW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outputR, outputW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	// Close immediately to simulate empty input
	inputW.Close()

	filterStderr(inputR, outputW)
	outputW.Close()

	buf := make([]byte, 4096)
	n, _ := outputR.Read(buf)
	if n != 0 {
		t.Errorf("expected no output for empty input, got %d bytes: %q", n, string(buf[:n]))
	}

	outputR.Close()
	inputR.Close()
}

func TestInstallStderrFilter_ReturnsRestoreFunc(t *testing.T) {
	restore := installStderrFilter()
	if restore == nil {
		t.Fatal("installStderrFilter should return a non-nil restore function")
	}
	// Calling restore should not panic
	restore()
}

func TestInstallStderrFilter_RestoreIsIdempotent(t *testing.T) {
	restore := installStderrFilter()
	restore()
	// Calling restore a second time should not panic
	restore()
}
