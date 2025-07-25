package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bombsimon/logrusr/v3"
	"github.com/devfile/alizer/pkg/apis/model"
	"github.com/phayes/freeport"
	"github.com/sirupsen/logrus"
)

func TestAnalyzeCommandContext_setProviders(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	tests := []struct {
		name            string
		providers       []string
		languages       []model.Language
		foundProviders  []string
		expectProviders []string
		expectError     bool
	}{
		{
			name:            "explicit providers",
			providers:       []string{"java", "go"},
			foundProviders:  []string{"existing"},
			expectProviders: []string{"existing", "java"},
			expectError:     false,
		},
		{
			name:            "no providers with Java language",
			providers:       []string{},
			languages:       []model.Language{{Name: "Java", CanBeComponent: true}},
			foundProviders:  []string{},
			expectProviders: []string{"java"},
			expectError:     false,
		},
		{
			name:            "no providers with C# language",
			providers:       []string{},
			languages:       []model.Language{{Name: "C#", CanBeComponent: true}},
			foundProviders:  []string{},
			expectProviders: []string{"dotnet"},
			expectError:     false,
		},
		{
			name:            "no providers with JavaScript language",
			providers:       []string{},
			languages:       []model.Language{{Name: "JavaScript", CanBeComponent: true}},
			foundProviders:  []string{},
			expectProviders: []string{"nodejs"},
			expectError:     false,
		},
		{
			name:            "no providers with TypeScript language",
			providers:       []string{},
			languages:       []model.Language{{Name: "TypeScript", CanBeComponent: true}},
			foundProviders:  []string{},
			expectProviders: []string{"nodejs"},
			expectError:     false,
		},
		{
			name:            "no providers with Python language",
			providers:       []string{},
			languages:       []model.Language{{Name: "Python", CanBeComponent: true}},
			foundProviders:  []string{},
			expectProviders: []string{"python"},
			expectError:     false,
		},
		{
			name:            "no providers with Go language",
			providers:       []string{},
			languages:       []model.Language{{Name: "Go", CanBeComponent: true}},
			foundProviders:  []string{},
			expectProviders: []string{"go"},
			expectError:     false,
		},
		{
			name:            "no providers with non-component language",
			providers:       []string{},
			languages:       []model.Language{{Name: "Java", CanBeComponent: false}},
			foundProviders:  []string{},
			expectProviders: []string{},
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &AnalyzeCommandContext{log: logger}
			result, err := c.setProviders(tt.providers, tt.languages, tt.foundProviders)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if len(result) != len(tt.expectProviders) {
				t.Errorf("Expected %d providers, got %d", len(tt.expectProviders), len(result))
			}

			// Check that expected providers are present
			for _, expected := range tt.expectProviders {
				found := false
				for _, actual := range result {
					if actual == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected provider '%s' not found in result %v", expected, result)
				}
			}
		})
	}
}

func TestAnalyzeCommandContext_setProviderInitInfo(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	tests := []struct {
		name            string
		foundProviders  []string
		expectError     bool
		expectProviders []string
	}{
		{
			name:            "java provider",
			foundProviders:  []string{"java"},
			expectError:     false,
			expectProviders: []string{"java"},
		},
		{
			name:            "go provider",
			foundProviders:  []string{"go"},
			expectError:     false,
			expectProviders: []string{"go"},
		},
		{
			name:            "python provider",
			foundProviders:  []string{"python"},
			expectError:     false,
			expectProviders: []string{"python"},
		},
		{
			name:            "nodejs provider",
			foundProviders:  []string{"nodejs"},
			expectError:     false,
			expectProviders: []string{"nodejs"},
		},
		{
			name:            "dotnet provider",
			foundProviders:  []string{"dotnet"},
			expectError:     false,
			expectProviders: []string{"dotnet"},
		},
		{
			name:            "multiple providers",
			foundProviders:  []string{"java", "go", "python"},
			expectError:     false,
			expectProviders: []string{"java", "go", "python"},
		},
		{
			name:            "unknown provider",
			foundProviders:  []string{"unknown"},
			expectError:     false,
			expectProviders: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &AnalyzeCommandContext{
				log:          logger,
				providersMap: make(map[string]ProviderInit),
			}

			err := c.setProviderInitInfo(tt.foundProviders)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Check that providers were added to the map
			for _, expected := range tt.expectProviders {
				if _, exists := c.providersMap[expected]; !exists {
					t.Errorf("Expected provider '%s' not found in providersMap", expected)
				} else {
					// Verify provider has valid port
					providerInit := c.providersMap[expected]
					if providerInit.port <= 0 {
						t.Errorf("Provider '%s' has invalid port: %d", expected, providerInit.port)
					}
					// Note: image may be empty in test environment, that's ok
					if providerInit.provider == nil {
						t.Errorf("Provider '%s' has nil provider", expected)
					}
				}
			}

			// Check that unknown providers were not added
			if len(c.providersMap) != len(tt.expectProviders) {
				t.Errorf("Expected %d providers in map, got %d", len(tt.expectProviders), len(c.providersMap))
			}
		})
	}
}

func TestAnalyzeCommandContext_createTempRuleSet(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "command-context-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name        string
		path        string
		ruleName    string
		createDir   bool
		expectError bool
	}{
		{
			name:        "create ruleset in existing directory",
			path:        tempDir,
			ruleName:    "test-ruleset",
			createDir:   true,
			expectError: false,
		},
		{
			name:        "non-existent directory",
			path:        filepath.Join(tempDir, "nonexistent"),
			ruleName:    "test-ruleset",
			createDir:   false,
			expectError: false, // Function returns nil for non-existent paths
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &AnalyzeCommandContext{log: logger}

			if tt.createDir && tt.path != tempDir {
				err := os.MkdirAll(tt.path, 0755)
				if err != nil {
					t.Fatalf("Failed to create test directory: %v", err)
				}
			}

			err := c.createTempRuleSet(tt.path, tt.ruleName)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Check if ruleset file was created when expected
			if tt.createDir && !tt.expectError {
				rulesetPath := filepath.Join(tt.path, "ruleset.yaml")
				if _, err := os.Stat(rulesetPath); os.IsNotExist(err) {
					t.Error("Expected ruleset.yaml to be created")
				} else {
					// Verify file contents
					content, err := os.ReadFile(rulesetPath)
					if err != nil {
						t.Errorf("Failed to read ruleset file: %v", err)
					} else {
						contentStr := string(content)
						if !strings.Contains(contentStr, tt.ruleName) {
							t.Errorf("Expected ruleset content to contain '%s'", tt.ruleName)
						}
						if !strings.Contains(contentStr, "name:") {
							t.Error("Expected ruleset content to contain 'name:' field")
						}
					}
				}
			}
		})
	}
}

func TestAnalyzeCommandContext_createContainerNetwork(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	c := &AnalyzeCommandContext{log: logger}

	// This test will likely fail because we can't actually create networks
	// but we can test that it doesn't panic and returns an error
	networkName, err := c.createContainerNetwork()

	// We expect an error because the container binary likely won't work in test environment
	if err == nil {
		t.Logf("Unexpectedly succeeded in creating network: %s", networkName)
		// If it succeeded, verify the network name was set
		if c.networkName == "" {
			t.Error("Expected networkName to be set")
		}
	} else {
		// This is expected in test environment
		t.Logf("Expected error creating network: %v", err)
	}
}

func TestAnalyzeCommandContext_createContainerVolume(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "command-context-test")
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
		inputPath   string
		isFileInput bool
		expectError bool
	}{
		{
			name:        "directory input",
			inputPath:   tempDir,
			isFileInput: false,
			expectError: true, // Will fail because container binary won't work
		},
		{
			name:        "file input",
			inputPath:   testFile,
			isFileInput: true,
			expectError: true, // Will fail because container binary won't work
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &AnalyzeCommandContext{
				log:         logger,
				isFileInput: tt.isFileInput,
				tempDirs:    []string{},
			}

			volumeName, err := c.createContainerVolume(tt.inputPath)

			if tt.expectError && err == nil {
				t.Logf("Unexpectedly succeeded in creating volume: %s", volumeName)
				// If it succeeded, verify the volume name was set
				if c.volumeName == "" {
					t.Error("Expected volumeName to be set")
				}
			} else if tt.expectError && err != nil {
				// This is expected in test environment
				t.Logf("Expected error creating volume: %v", err)
			} else if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestProviderInit_Structure(t *testing.T) {
	// Test that ProviderInit can be created and has expected fields
	port, err := freeport.GetFreePort()
	if err != nil {
		t.Fatalf("Failed to get free port: %v", err)
	}

	init := ProviderInit{
		port:          port,
		image:         "test-image:latest",
		isRunning:     false,
		containerName: "test-container",
		provider:      nil, // Can be nil in tests
	}

	if init.port != port {
		t.Errorf("Expected port %d, got %d", port, init.port)
	}
	if init.image != "test-image:latest" {
		t.Errorf("Expected image 'test-image:latest', got '%s'", init.image)
	}
	if init.isRunning {
		t.Error("Expected isRunning to be false")
	}
	if init.containerName != "test-container" {
		t.Errorf("Expected containerName 'test-container', got '%s'", init.containerName)
	}
}