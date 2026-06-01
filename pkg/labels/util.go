package labels

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"sort"
	"strings"

	outputv1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
)

// ParseLabelLines splits listing output into non-empty trimmed lines.
func ParseLabelLines(raw string) []string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		s := strings.TrimSpace(line)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

// WalkRuleSets returns a WalkDirFunc that collects konveyor source/target label strings from rule files.
func WalkRuleSets(root string, label string, labelsSlice *[]string) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d == nil || d.IsDir() {
			return nil
		}
		var readErr error
		*labelsSlice, readErr = readRuleFile(path, labelsSlice, label)
		return readErr
	}
}

func readRuleFile(filePath string, labelsSlice *[]string, label string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanWords)

	for scanner.Scan() {
		found := sourceOrTargetLabel(scanner.Text(), label)
		if len(found) > 0 && !slices.Contains(*labelsSlice, found) {
			*labelsSlice = append(*labelsSlice, found)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return *labelsSlice, nil
}

func sourceOrTargetLabel(text string, label string) string {
	if strings.Contains(text, label) {
		return text
	}
	return ""
}

// ListOptionsFromLabels prints sorted technology names for a label prefix.
func ListOptionsFromLabels(sl []string, label string, out io.Writer) {
	var newSl []string
	l := label + "="

	for _, label := range sl {
		newSt := strings.TrimPrefix(label, l)

		if newSt != label {
			newSt = strings.TrimSuffix(newSt, "+")
			newSt = strings.TrimSuffix(newSt, "-")

			if !slices.Contains(newSl, newSt) {
				newSl = append(newSl, newSt)
			}
		}
	}
	sort.Strings(newSl)

	if label == outputv1.SourceTechnologyLabel {
		fmt.Fprintln(out, "available source technologies:")
	} else {
		fmt.Fprintln(out, "available target technologies:")
	}
	for _, tech := range newSl {
		fmt.Fprintln(out, tech)
	}
}
