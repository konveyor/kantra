package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bombsimon/logrusr/v3"
	"github.com/sirupsen/logrus"
)

func TestAnalyzeCommand_inputShortName(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple filename",
			input:    "/path/to/myapp",
			expected: "myapp",
		},
		{
			name:     "filename with extension",
			input:    "/path/to/myapp.jar",
			expected: "myapp.jar",
		},
		{
			name:     "current directory",
			input:    ".",
			expected: ".",
		},
		{
			name:     "empty input",
			input:    "",
			expected: ".",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &analyzeCommand{
				input:                 tt.input,
				AnalyzeCommandContext: AnalyzeCommandContext{log: logger},
			}

			result := cmd.inputShortName()
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestAnalyzeCommand_getRulesVolumes(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	// Create temporary directory with rule files
	tempDir, err := os.MkdirTemp("", "util-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test rule file
	ruleFile := filepath.Join(tempDir, "test.yaml")
	err = os.WriteFile(ruleFile, []byte("test rule content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create rule file: %v", err)
	}

	tests := []struct {
		name          string
		rules         []string
		expectVolumes bool
		expectErr     bool
	}{
		{
			name:          "no rules specified",
			rules:         []string{},
			expectVolumes: false,
			expectErr:     false,
		},
		{
			name:          "rules directory specified",
			rules:         []string{tempDir},
			expectVolumes: true,
			expectErr:     false,
		},
		{
			name:          "rules file specified",
			rules:         []string{ruleFile},
			expectVolumes: true,
			expectErr:     false,
		},
		{
			name:          "empty rule path",
			rules:         []string{""},
			expectVolumes: false,
			expectErr:     true, // Expect error for empty rule path
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &analyzeCommand{
				rules:                 tt.rules,
				AnalyzeCommandContext: AnalyzeCommandContext{log: logger},
			}

			volumes, err := cmd.getRulesVolumes()
			if err != nil && !tt.expectErr {
				t.Fatalf("Unexpected error: %v", err)
			}

			if tt.expectVolumes && len(volumes) == 0 {
				t.Error("Expected rule volumes but got none")
			}
			if !tt.expectVolumes && len(volumes) > 0 {
				t.Errorf("Expected no rule volumes but got %d", len(volumes))
			}
		})
	}
}

func TestAnalyzeCommand_getDepsFolders(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "util-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name        string
		depFolders  []string
		expectMount bool
	}{
		{
			name:        "no dependency folders",
			depFolders:  []string{},
			expectMount: false,
		},
		{
			name:        "valid dependency folder",
			depFolders:  []string{tempDir},
			expectMount: true,
		},
		{
			name:        "multiple dependency folders",
			depFolders:  []string{tempDir, tempDir},
			expectMount: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &analyzeCommand{
				depFolders:            tt.depFolders,
				AnalyzeCommandContext: AnalyzeCommandContext{log: logger},
			}

			volumes, _ := cmd.getDepsFolders()

			if tt.expectMount && len(volumes) == 0 {
				t.Error("Expected dependency volume mount but got none")
			}
			if !tt.expectMount && len(volumes) > 0 {
				t.Errorf("Expected no dependency volume mount but got %d", len(volumes))
			}
		})
	}
}
