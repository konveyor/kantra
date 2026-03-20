package analyze

import (
	"bufio"
	"os"
	"strings"
)

// stderrFilterPatterns contains substrings to filter from stderr output.
// Lines containing any of these patterns will be silently dropped.
var stderrFilterPatterns = []string{
	"Windows system assumed buffer larger than it is, events have likely been missed",
}

// filterStderr reads lines from r, drops lines matching any pattern in
// stderrFilterPatterns, and writes the rest to dest.
func filterStderr(r *os.File, dest *os.File) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if shouldFilterLine(line) {
			continue
		}
		dest.WriteString(line + "\n")
	}
}

func shouldFilterLine(line string) bool {
	for _, pattern := range stderrFilterPatterns {
		if strings.Contains(line, pattern) {
			return true
		}
	}
	return false
}
