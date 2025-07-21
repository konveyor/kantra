package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/bombsimon/logrusr/v3"
	"github.com/sirupsen/logrus"
)

func TestNewWindupShimCommand(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	cmd := NewWindupShimCommand(logger)

	// Test command structure
	if cmd.Use != "rules" {
		t.Errorf("Expected Use to be 'rules', got '%s'", cmd.Use)
	}

	if cmd.Short != "Convert XML rules to YAML" {
		t.Errorf("Expected Short to be 'Convert XML rules to YAML', got '%s'", cmd.Short)
	}

	if cmd.RunE == nil {
		t.Error("Expected RunE function to be set")
	}

	if cmd.PreRunE == nil {
		t.Error("Expected PreRunE function to be set")
	}

	// Test that command has no subcommands
	if len(cmd.Commands()) != 0 {
		t.Errorf("Expected no subcommands, got %d", len(cmd.Commands()))
	}
}

func TestWindupShimCommand_Flags(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	cmd := NewWindupShimCommand(logger)

	// Test that flags are set up correctly
	flags := cmd.Flags()

	expectedFlags := []string{
		"input",
		"output",
	}

	for _, flagName := range expectedFlags {
		flag := flags.Lookup(flagName)
		if flag == nil {
			t.Errorf("Expected flag '%s' to be set", flagName)
		}
	}
}

func TestWindupShimCommand_PreRunE(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	// Create temporary directories for testing
	tempDir, err := os.MkdirTemp("", "shimconvert-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	inputDir := filepath.Join(tempDir, "input")
	outputDir := filepath.Join(tempDir, "output")
	err = os.Mkdir(inputDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create input dir: %v", err)
	}
	err = os.Mkdir(outputDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	// Create a test XML rule file
	xmlRule := filepath.Join(inputDir, "test.windup.xml")
	xmlContent := `<?xml version="1.0"?>
<ruleset id="test-rules">
    <metadata>
        <description>Test rules</description>
    </metadata>
    <rules>
        <rule id="test-rule">
            <when>
                <javaclass references="java.util.Vector"/>
            </when>
            <perform>
                <hint title="Replace Vector" effort="1" category-id="mandatory"/>
            </perform>
        </rule>
    </rules>
</ruleset>`
	err = os.WriteFile(xmlRule, []byte(xmlContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create XML rule file: %v", err)
	}

	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "valid input and output",
			args:        []string{"--input", inputDir, "--output", outputDir},
			expectError: false,
		},
		{
			name:        "missing input flag",
			args:        []string{"--output", outputDir},
			expectError: true,
		},
		{
			name:        "missing output flag",
			args:        []string{"--input", inputDir},
			expectError: true,
		},
		{
			name:        "non-existent input",
			args:        []string{"--input", "/nonexistent", "--output", outputDir},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewWindupShimCommand(logger)
			cmd.SetArgs(tt.args)

			// Parse flags first
			err := cmd.ParseFlags(tt.args)
			if err != nil {
				t.Fatalf("Failed to parse flags: %v", err)
			}

			// Run PreRunE function
			var preRunErr error
			if cmd.PreRunE != nil {
				preRunErr = cmd.PreRunE(cmd, tt.args)
			}

			if tt.expectError && preRunErr == nil {
				t.Error("Expected error in PreRunE but got none")
			}
			if !tt.expectError && preRunErr != nil {
				t.Errorf("Unexpected error in PreRunE: %v", preRunErr)
			}
		})
	}
}

func TestWindupShimCommand_RunE(t *testing.T) {
	testLogger := logrus.New()
	testLogger.SetOutput(bytes.NewBuffer(nil)) // Silence output
	logger := logrusr.New(testLogger)

	// Create temporary directories for testing
	tempDir, err := os.MkdirTemp("", "shimconvert-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	inputDir := filepath.Join(tempDir, "input")
	outputDir := filepath.Join(tempDir, "output")
	err = os.Mkdir(inputDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create input dir: %v", err)
	}
	err = os.Mkdir(outputDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	// Create a test XML rule file
	xmlRule := filepath.Join(inputDir, "test.windup.xml")
	xmlContent := `<?xml version="1.0"?>
<ruleset id="test-rules">
    <metadata>
        <description>Test rules</description>
    </metadata>
    <rules>
        <rule id="test-rule">
            <when>
                <javaclass references="java.util.Vector"/>
            </when>
            <perform>
                <hint title="Replace Vector" effort="1" category-id="mandatory"/>
            </perform>
        </rule>
    </rules>
</ruleset>`
	err = os.WriteFile(xmlRule, []byte(xmlContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create XML rule file: %v", err)
	}

	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "convert XML rules to YAML",
			args:        []string{"--input", inputDir, "--output", outputDir},
			expectError: false, // Should succeed with valid XML
		},
		{
			name:        "single file input",
			args:        []string{"--input", xmlRule, "--output", outputDir},
			expectError: false, // Should succeed with valid XML file
		},
		{
			name:        "invalid input path",
			args:        []string{"--input", "/nonexistent", "--output", outputDir},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewWindupShimCommand(logger)

			buf := &bytes.Buffer{}
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// If conversion succeeded, check that YAML files were created
			if !tt.expectError && err == nil {
				// Check if any YAML files were created in output directory
				entries, err := os.ReadDir(outputDir)
				if err != nil {
					t.Errorf("Failed to read output directory: %v", err)
				} else {
					yamlFound := false
					for _, entry := range entries {
						if filepath.Ext(entry.Name()) == ".yaml" || filepath.Ext(entry.Name()) == ".yml" {
							yamlFound = true
							break
						}
					}
					if !yamlFound {
						t.Error("Expected YAML files to be created in output directory")
					}
				}
			}
		})
	}
}

func TestWindupShimCommand_HelpCommand(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	cmd := NewWindupShimCommand(logger)

	// Test help command
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// Execute help
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	if err != nil {
		t.Errorf("Unexpected error running help: %v", err)
	}

	// Check that help contains expected sections
	expectedStrings := []string{
		"rules",
		"Convert XML rules to YAML",
		"input",
		"output",
	}

	for _, expected := range expectedStrings {
		if !bytes.Contains(buf.Bytes(), []byte(expected)) {
			t.Errorf("Expected help output to contain '%s'", expected)
		}
	}
}

func TestWindupShimCommand_Validation(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	// Create temporary directories and files for testing
	tempDir, err := os.MkdirTemp("", "shimconvert-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	inputDir := filepath.Join(tempDir, "input")
	outputDir := filepath.Join(tempDir, "output")
	err = os.Mkdir(inputDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create input dir: %v", err)
	}

	// Create an invalid XML file
	invalidXML := filepath.Join(inputDir, "invalid.xml")
	err = os.WriteFile(invalidXML, []byte("invalid xml content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create invalid XML file: %v", err)
	}

	tests := []struct {
		name        string
		input       string
		output      string
		expectError bool
	}{
		{
			name:        "valid input directory",
			input:       inputDir,
			output:      outputDir,
			expectError: false, // Should succeed even with invalid XML (just won't convert)
		},
		{
			name:        "non-existent input",
			input:       "/nonexistent/path",
			output:      outputDir,
			expectError: true,
		},
		{
			name:        "input is a file",
			input:       invalidXML,
			output:      outputDir,
			expectError: false, // Should succeed with file input
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewWindupShimCommand(logger)

			buf := &bytes.Buffer{}
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"--input", tt.input, "--output", tt.output})

			err := cmd.Execute()

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestWindupShimCommand_WithTransformCommand(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	// Test that rules command can be added to transform command
	transformCmd := NewTransformCommand(logger)
	
	// Verify rules is a subcommand of transform
	commands := transformCmd.Commands()
	found := false
	for _, cmd := range commands {
		if cmd.Use == "rules" {
			found = true
			break
		}
	}
	
	if !found {
		t.Error("Rules command was not found as subcommand of transform")
	}
}