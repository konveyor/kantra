package cmd

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/bombsimon/logrusr/v3"
	"github.com/sirupsen/logrus"
)

func Test_analyzeCommand_getLabelSelectorArgs(t *testing.T) {
	tests := []struct {
		name    string
		labelSelector string
		sources []string
		targets []string
		want    string
	}{
		{
			name: "neither sources nor targets must not create label selector",
		},
		{
			name:    "one target specified, return target, catch-all source and default labels",
			targets: []string{"test"},
			want:    "((konveyor.io/target=test) && konveyor.io/source) || (discovery)",
		},
		{
			name:    "one source specified, return source and default labels",
			sources: []string{"test"},
			want:    "(konveyor.io/source=test) || (discovery)",
		},
		{
			name:    "one source & one target specified, return source, target and default labels",
			sources: []string{"test"},
			targets: []string{"test"},
			want:    "((konveyor.io/target=test) && (konveyor.io/source=test)) || (discovery)",
		},
		{
			name:    "multiple sources specified, OR them all with default labels",
			sources: []string{"t1", "t2"},
			want:    "(konveyor.io/source=t1 || konveyor.io/source=t2) || (discovery)",
		},
		{
			name:    "multiple targets specified, OR them all, AND result with catch-all source label, finally OR with default labels",
			targets: []string{"t1", "t2"},
			want:    "((konveyor.io/target=t1 || konveyor.io/target=t2) && konveyor.io/source) || (discovery)",
		},
		{
			name:    "multiple sources & targets specified, OR them within each other, AND result with catch-all source label, finally OR with default labels",
			targets: []string{"t1", "t2"},
			sources: []string{"t1", "t2"},
			want:    "((konveyor.io/target=t1 || konveyor.io/target=t2) && (konveyor.io/source=t1 || konveyor.io/source=t2)) || (discovery)",
		},
		{
			name:    "return the labelSelector when specified",
			labelSelector: "example.io/target=foo",
			want:    "example.io/target=foo",
		},
		{
			name:    "labelSelector should win",
			targets: []string{"t1", "t2"},
			sources: []string{"t1", "t2"},
			labelSelector: "example.io/target=foo",
			want:    "example.io/target=foo",
		},
		{
			name:    "multiple sources & targets specified, OR them within each other, AND result with catch-all source label, finally OR with default labels",
			targets: []string{"t1", "t2"},
			sources: []string{"t1", "t2"},
			labelSelector: "",
			want:    "((konveyor.io/target=t1 || konveyor.io/target=t2) && (konveyor.io/source=t1 || konveyor.io/source=t2)) || (discovery)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &analyzeCommand{
				sources: tt.sources,
				targets: tt.targets,
				labelSelector: tt.labelSelector,
			}
			if got := a.getLabelSelector(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("analyzeCommand.getLabelSelectorArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAnalyzeCommand_Validate(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "analyze-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test file
	testFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name        string
		cmd         *analyzeCommand
		expectError bool
		errorContains string
	}{
		{
			name: "list sources should not validate input",
			cmd: &analyzeCommand{
				listSources: true,
				AnalyzeCommandContext: AnalyzeCommandContext{log: logger},
			},
			expectError: false,
		},
		{
			name: "list targets should not validate input",
			cmd: &analyzeCommand{
				listTargets: true,
				AnalyzeCommandContext: AnalyzeCommandContext{log: logger},
			},
			expectError: false,
		},
		{
			name: "list providers should not validate input",
			cmd: &analyzeCommand{
				listProviders: true,
				AnalyzeCommandContext: AnalyzeCommandContext{log: logger},
			},
			expectError: false,
		},
		{
			name: "list languages with valid directory",
			cmd: &analyzeCommand{
				listLanguages: true,
				input:         tempDir,
				AnalyzeCommandContext: AnalyzeCommandContext{log: logger},
			},
			expectError: false,
		},
		{
			name: "list languages with valid file",
			cmd: &analyzeCommand{
				listLanguages: true,
				input:         testFile,
				AnalyzeCommandContext: AnalyzeCommandContext{
					log:         logger,
					isFileInput: false, // Will be set to true by validation
				},
			},
			expectError: false,
		},
		{
			name: "list languages with non-existent path",
			cmd: &analyzeCommand{
				listLanguages: true,
				input:         "/nonexistent/path",
				AnalyzeCommandContext: AnalyzeCommandContext{log: logger},
			},
			expectError: true,
			errorContains: "failed to stat input path",
		},
		{
			name: "label selector with sources should error",
			cmd: &analyzeCommand{
				labelSelector: "test=value",
				sources:       []string{"java"},
				AnalyzeCommandContext: AnalyzeCommandContext{log: logger},
			},
			expectError: true,
			errorContains: "must not specify label-selector and sources or targets",
		},
		{
			name: "label selector with targets should error",
			cmd: &analyzeCommand{
				labelSelector: "test=value",
				targets:       []string{"quarkus"},
				AnalyzeCommandContext: AnalyzeCommandContext{log: logger},
			},
			expectError: true,
			errorContains: "must not specify label-selector and sources or targets",
		},
		{
			name: "non-existent rules path should error",
			cmd: &analyzeCommand{
				rules: []string{"/nonexistent/rules.yaml"},
				AnalyzeCommandContext: AnalyzeCommandContext{log: logger},
			},
			expectError: true,
			errorContains: "failed to stat rules at path",
		},
		{
			name: "empty rules path should be ignored",
			cmd: &analyzeCommand{
				rules: []string{""},
				output: tempDir,
				overwrite: true,
				mode: "full",
				AnalyzeCommandContext: AnalyzeCommandContext{log: logger},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := tt.cmd.Validate(ctx)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if tt.expectError && err != nil && tt.errorContains != "" {
				if !analyzeContains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain '%s', got '%s'", tt.errorContains, err.Error())
				}
			}

			// Test that isFileInput is set correctly for file inputs
			if tt.cmd.listLanguages && tt.cmd.input == testFile && err == nil {
				if !tt.cmd.isFileInput {
					t.Error("Expected isFileInput to be true for file input")
				}
			}
		})
	}
}

func TestAnalyzeCommand_CheckOverwriteOutput(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	// Create temporary directories for testing
	tempDir, err := os.MkdirTemp("", "analyze-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	existingDir := filepath.Join(tempDir, "existing")
	err = os.Mkdir(existingDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create existing dir: %v", err)
	}

	nonExistentDir := filepath.Join(tempDir, "nonexistent")

	tests := []struct {
		name        string
		output      string
		overwrite   bool
		expectError bool
	}{
		{
			name:        "non-existent output directory",
			output:      nonExistentDir,
			overwrite:   false,
			expectError: false,
		},
		{
			name:        "existing output directory without overwrite",
			output:      existingDir,
			overwrite:   false,
			expectError: true,
		},
		{
			name:        "existing output directory with overwrite",
			output:      existingDir,
			overwrite:   true,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &analyzeCommand{
				output:    tt.output,
				overwrite: tt.overwrite,
				AnalyzeCommandContext: AnalyzeCommandContext{log: logger},
			}

			err := cmd.CheckOverwriteOutput()

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestAnalyzeCommand_validateProviders(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	tests := []struct {
		name        string
		providers   []string
		expectError bool
	}{
		{
			name:        "valid providers",
			providers:   []string{"java", "go", "python"},
			expectError: false,
		},
		{
			name:        "empty providers",
			providers:   []string{},
			expectError: false,
		},
		{
			name:        "providers with empty string",
			providers:   []string{"java", "", "go"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &analyzeCommand{
				AnalyzeCommandContext: AnalyzeCommandContext{log: logger},
			}

			err := cmd.validateProviders(tt.providers)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestAnalyzeCommand_needDefaultRules(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	tests := []struct {
		name                     string
		enableDefaultRulesets    bool
		providersMap             map[string]ProviderInit
		expectedDefaultRulesets  bool
	}{
		{
			name:                    "default rulesets enabled with java provider",
			enableDefaultRulesets:   true,
			providersMap:            map[string]ProviderInit{"java": {}},
			expectedDefaultRulesets: true,
		},
		{
			name:                    "default rulesets disabled",
			enableDefaultRulesets:   false,
			providersMap:            map[string]ProviderInit{"java": {}},
			expectedDefaultRulesets: false,
		},
		{
			name:                    "no java provider",
			enableDefaultRulesets:   true,
			providersMap:            map[string]ProviderInit{"go": {}},
			expectedDefaultRulesets: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &analyzeCommand{
				enableDefaultRulesets: tt.enableDefaultRulesets,
				AnalyzeCommandContext: AnalyzeCommandContext{
					log:          logger,
					providersMap: tt.providersMap,
				},
			}

			cmd.needDefaultRules()

			if cmd.enableDefaultRulesets != tt.expectedDefaultRulesets {
				t.Errorf("Expected enableDefaultRulesets to be %t, got %t", tt.expectedDefaultRulesets, cmd.enableDefaultRulesets)
			}
		})
	}
}

// Helper function for string contains check
func analyzeContains(s, substr string) bool {
	return strings.Contains(s, substr)
}
