package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bombsimon/logrusr/v3"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/sirupsen/logrus"
)

func TestValidateFlags(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	tests := []struct {
		name                 string
		analysisOutputPaths  []string
		appNames            []string
		depsOutputs         []string
		expectError         bool
		expectedAppCount    int
	}{
		{
			name:        "empty analysis paths",
			expectError: true,
		},
		{
			name:                "empty app names",
			analysisOutputPaths: []string{"path1"},
			expectError:         true,
		},
		{
			name:                "valid inputs",
			analysisOutputPaths: []string{"path1", "path2"},
			appNames:           []string{"app1", "app2"},
			depsOutputs:        []string{"deps1", "deps2"},
			expectError:        false,
			expectedAppCount:   2,
		},
		{
			name:                "valid inputs without deps",
			analysisOutputPaths: []string{"path1"},
			appNames:           []string{"app1"},
			expectError:        false,
			expectedAppCount:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apps, err := validateFlags(tt.analysisOutputPaths, tt.appNames, tt.depsOutputs, logger)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tt.expectError && len(apps) != tt.expectedAppCount {
				t.Errorf("Expected %d apps, got %d", tt.expectedAppCount, len(apps))
			}
		})
	}
}

func TestValidateFlags_AppProperties(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	analysisOutputPaths := []string{"path1", "path2"}
	appNames := []string{"app1", "app2"}
	depsOutputs := []string{"deps1", "deps2"}

	apps, err := validateFlags(analysisOutputPaths, appNames, depsOutputs, logger)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(apps) != 2 {
		t.Fatalf("Expected 2 apps, got %d", len(apps))
	}

	// Test first app
	if apps[0].Id != "0000" {
		t.Errorf("Expected first app ID to be '0000', got '%s'", apps[0].Id)
	}
	if apps[0].Name != "app1" {
		t.Errorf("Expected first app name to be 'app1', got '%s'", apps[0].Name)
	}
	if apps[0].analysisPath != "path1" {
		t.Errorf("Expected first app analysis path to be 'path1', got '%s'", apps[0].analysisPath)
	}
	if apps[0].depsPath != "deps1" {
		t.Errorf("Expected first app deps path to be 'deps1', got '%s'", apps[0].depsPath)
	}

	// Test second app
	if apps[1].Id != "0001" {
		t.Errorf("Expected second app ID to be '0001', got '%s'", apps[1].Id)
	}
	if apps[1].Name != "app2" {
		t.Errorf("Expected second app name to be 'app2', got '%s'", apps[1].Name)
	}
}

func TestLoadApplications(t *testing.T) {
	// Create temporary test files
	tempDir, err := os.MkdirTemp("", "static-report-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test analysis file
	analysisFile := filepath.Join(tempDir, "analysis.yaml")
	analysisContent := `
- name: test-ruleset
  violations:
    test-rule:
      description: "Test rule"
      incidents:
        - uri: "file:///test.java"
          message: "Test message"
          variables: {}
`
	err = os.WriteFile(analysisFile, []byte(analysisContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create analysis file: %v", err)
	}

	// Create a test dependencies file
	depsFile := filepath.Join(tempDir, "deps.yaml")
	depsContent := `
- provider: maven
  dependencies:
    - name: "test-dependency"
      version: "1.0.0"
      extras: {}
`
	err = os.WriteFile(depsFile, []byte(depsContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create deps file: %v", err)
	}

	// Test loading applications
	apps := []*Application{
		{
			Id:           "0000",
			Name:         "test-app",
			analysisPath: analysisFile,
			depsPath:     depsFile,
			Rulesets:     make([]konveyor.RuleSet, 0),
			DepItems:     make([]konveyor.DepsFlatItem, 0),
		},
	}

	err = loadApplications(apps)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify data was loaded
	if len(apps[0].Rulesets) == 0 {
		t.Error("Expected rulesets to be loaded")
	}
	if len(apps[0].DepItems) == 0 {
		t.Error("Expected dependencies to be loaded")
	}
}

func TestLoadApplications_FileNotFound(t *testing.T) {
	apps := []*Application{
		{
			Id:           "0000",
			Name:         "test-app",
			analysisPath: "/nonexistent/file.yaml",
			depsPath:     "",
			Rulesets:     make([]konveyor.RuleSet, 0),
			DepItems:     make([]konveyor.DepsFlatItem, 0),
		},
	}

	err := loadApplications(apps)
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestLoadApplications_InvalidYAML(t *testing.T) {
	// Create temporary test file with invalid YAML
	tempDir, err := os.MkdirTemp("", "static-report-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	analysisFile := filepath.Join(tempDir, "invalid.yaml")
	invalidContent := "invalid: yaml: content: ["
	err = os.WriteFile(analysisFile, []byte(invalidContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create invalid file: %v", err)
	}

	apps := []*Application{
		{
			Id:           "0000",
			Name:         "test-app",
			analysisPath: analysisFile,
			depsPath:     "",
			Rulesets:     make([]konveyor.RuleSet, 0),
			DepItems:     make([]konveyor.DepsFlatItem, 0),
		},
	}

	err = loadApplications(apps)
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestGenerateJSBundle(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	// Create temporary output file
	tempDir, err := os.MkdirTemp("", "static-report-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	outputFile := filepath.Join(tempDir, "output.js")

	// Create test applications
	apps := []*Application{
		{
			Id:       "0000",
			Name:     "test-app",
			Rulesets: []konveyor.RuleSet{},
			DepItems: []konveyor.DepsFlatItem{},
		},
	}

	err = generateJSBundle(apps, outputFile, logger)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Error("Expected output file to be created")
	}

	// Verify file contents
	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "window[\"apps\"]") {
		t.Error("Expected output to contain window assignment")
	}
	if !strings.Contains(contentStr, "test-app") {
		t.Error("Expected output to contain app name")
	}
}

func TestGenerateJSBundle_InvalidPath(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	apps := []*Application{}
	invalidPath := "/nonexistent/dir/output.js"

	err := generateJSBundle(apps, invalidPath, logger)
	if err == nil {
		t.Error("Expected error for invalid output path")
	}
}

func TestApplication_Structure(t *testing.T) {
	app := &Application{
		Id:       "test-id",
		Name:     "test-name",
		Rulesets: []konveyor.RuleSet{},
		DepItems: []konveyor.DepsFlatItem{},
	}

	if app.Id != "test-id" {
		t.Errorf("Expected Id to be 'test-id', got '%s'", app.Id)
	}
	if app.Name != "test-name" {
		t.Errorf("Expected Name to be 'test-name', got '%s'", app.Name)
	}
	if app.Rulesets == nil {
		t.Error("Expected Rulesets to be initialized")
	}
	if app.DepItems == nil {
		t.Error("Expected DepItems to be initialized")
	}
}

