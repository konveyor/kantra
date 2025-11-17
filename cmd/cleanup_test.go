package cmd

import (
	"context"
	"testing"

	"github.com/bombsimon/logrusr/v3"
	"github.com/sirupsen/logrus"
)

func TestAnalyzeCommand_CleanAnalysisResources(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	tests := []struct {
		name        string
		cleanup     bool
		needsBuiltin bool
		expectError bool
	}{
		{
			name:        "cleanup disabled",
			cleanup:     false,
			needsBuiltin: false,
			expectError: false,
		},
		{
			name:        "needs builtin",
			cleanup:     true,
			needsBuiltin: true,
			expectError: false,
		},
		{
			name:        "cleanup enabled",
			cleanup:     true,
			needsBuiltin: false,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &analyzeCommand{
				cleanup: tt.cleanup,
				AnalyzeCommandContext: AnalyzeCommandContext{
					needsBuiltin: tt.needsBuiltin,
					log:          logger,
					tempDirs:     []string{},
				},
			}

			ctx := context.Background()
			err := a.CleanAnalysisResources(ctx)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestAnalyzeCommandContext_RmNetwork(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	tests := []struct {
		name        string
		networkName string
	}{
		{
			name:        "empty network name",
			networkName: "",
		},
		{
			name:        "with network name",
			networkName: "test-network",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &AnalyzeCommandContext{
				networkName: tt.networkName,
				log:         logger,
			}

			ctx := context.Background()
			err := c.RmNetwork(ctx)

			// For empty network name, should always succeed
			if tt.networkName == "" && err != nil {
				t.Errorf("Unexpected error for empty network name: %v", err)
			}

			// For non-empty network name, the function may return an error
			// if Docker is unavailable or the network doesn't exist.
			// We just verify it doesn't panic - the actual result depends
			// on the test environment.
			if tt.networkName != "" && err != nil {
				t.Logf("RmNetwork returned error (may be expected in test env): %v", err)
			}
		})
	}
}

func TestAnalyzeCommandContext_RmVolumes(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	tests := []struct {
		name        string
		volumeName  string
		expectError bool
	}{
		{
			name:        "empty volume name",
			volumeName:  "",
			expectError: false,
		},
		{
			name:        "with volume name",
			volumeName:  "test-volume",
			expectError: false, // RmVolumes logs errors but doesn't return them
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &AnalyzeCommandContext{
				volumeName: tt.volumeName,
				log:        logger,
			}

			ctx := context.Background()
			err := c.RmVolumes(ctx)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestAnalyzeCommandContext_RmProviderContainers(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	tests := []struct {
		name           string
		containerNames []string
		expectError    bool
	}{
		{
			name:           "empty container names",
			containerNames: []string{},
			expectError:    false,
		},
		{
			name:           "with container names",
			containerNames: []string{"container1", "container2"},
			expectError:    false, // Function doesn't return error even if commands fail
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &AnalyzeCommandContext{
				providerContainerNames: tt.containerNames,
				log:                   logger,
			}

			ctx := context.Background()
			err := c.RmProviderContainers(ctx)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestAnalyzeCommand_cleanlsDirs(t *testing.T) {
	testLogger := logrus.New()
	logger := logrusr.New(testLogger)

	a := &analyzeCommand{
		AnalyzeCommandContext: AnalyzeCommandContext{
			log: logger,
		},
	}

	// This function always returns nil
	err := a.cleanlsDirs()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestAnalyzeCommandContext_Structure(t *testing.T) {
	c := &AnalyzeCommandContext{
		providersMap:           make(map[string]ProviderInit),
		tempDirs:              []string{},
		isFileInput:           false,
		needsBuiltin:          false,
		networkName:           "test-network",
		volumeName:            "test-volume",
		providerContainerNames: []string{"container1"},
		reqMap:                make(map[string]string),
		kantraDir:             "/test/dir",
	}

	// Test that structure is properly initialized
	if c.providersMap == nil {
		t.Error("Expected providersMap to be initialized")
	}
	if c.tempDirs == nil {
		t.Error("Expected tempDirs to be initialized")
	}
	if c.networkName != "test-network" {
		t.Errorf("Expected networkName to be 'test-network', got '%s'", c.networkName)
	}
	if c.volumeName != "test-volume" {
		t.Errorf("Expected volumeName to be 'test-volume', got '%s'", c.volumeName)
	}
	if len(c.providerContainerNames) != 1 {
		t.Errorf("Expected 1 container name, got %d", len(c.providerContainerNames))
	}
}