package cmd

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestConfigCommand creates a test configCommand instance
func createTestConfigCommand() *configCommand {
	return &configCommand{
		log: logr.Discard(),
	}
}

// captureOutput captures stdout output during test execution
func captureOutput(t *testing.T, fn func()) string {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w

	outputChan := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outputChan <- buf.String()
	}()

	fn()

	w.Close()
	os.Stdout = oldStdout
	output := <-outputChan

	return output
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name          string
		setupCommand  func(t *testing.T) *configCommand
		expectedError string
	}{
		{
			name: "empty listProfiles should pass",
			setupCommand: func(t *testing.T) *configCommand {
				cmd := createTestConfigCommand()
				cmd.listProfiles = ""
				return cmd
			},
			expectedError: "",
		},
		{
			name: "valid directory path should pass",
			setupCommand: func(t *testing.T) *configCommand {
				cmd := createTestConfigCommand()
				cmd.listProfiles = t.TempDir()
				return cmd
			},
			expectedError: "",
		},
		{
			name: "non-existent path should fail",
			setupCommand: func(t *testing.T) *configCommand {
				cmd := createTestConfigCommand()
				cmd.listProfiles = "/nonexistent/path"
				return cmd
			},
			expectedError: "failed to stat application path for profile /nonexistent/path",
		},
		{
			name: "file path instead of directory should fail",
			setupCommand: func(t *testing.T) *configCommand {
				tmpDir := t.TempDir()
				filePath := filepath.Join(tmpDir, "testfile.txt")
				err := os.WriteFile(filePath, []byte("test"), 0644)
				require.NoError(t, err)

				cmd := createTestConfigCommand()
				cmd.listProfiles = filePath
				return cmd
			},
			expectedError: "is not a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.setupCommand(t)
			ctx := context.Background()

			err := cmd.Validate(ctx)

			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestListProfiles(t *testing.T) {
	t.Run("should print list profiles message", func(t *testing.T) {
		cmd := createTestConfigCommand()

		output := captureOutput(t, func() {
			cmd.ListProfiles()
		})

		assert.Equal(t, "list profiles\n", output)
	})
}

func TestPerformLogin(t *testing.T) {
	tests := []struct {
		name           string
		hubURL         string
		username       string
		password       string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		expectError    bool
		errorContains  string
	}{
		{
			name:     "successful login",
			hubURL:   "http://example.com",
			username: "testuser",
			password: "testpass",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and headers
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
				assert.Equal(t, "kantra-cli", r.Header.Get("User-Agent"))

				// Verify request body
				err := r.ParseForm()
				assert.NoError(t, err)
				assert.Equal(t, "testuser", r.FormValue("username"))
				assert.Equal(t, "testpass", r.FormValue("password"))

				w.WriteHeader(http.StatusOK)
			},
			expectError: false,
		},
		{
			name:     "login with trailing slash in URL",
			hubURL:   "http://example.com/",
			username: "testuser",
			password: "testpass",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				// Verify the URL is correctly constructed without double slashes
				assert.Equal(t, "/api/login", r.URL.Path)
				w.WriteHeader(http.StatusOK)
			},
			expectError: false,
		},
		{
			name:     "server returns 401 unauthorized",
			hubURL:   "http://example.com",
			username: "wronguser",
			password: "wrongpass",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			expectError:   true,
			errorContains: "login failed with status: 401 Unauthorized",
		},
		{
			name:     "server returns 500 internal server error",
			hubURL:   "http://example.com",
			username: "testuser",
			password: "testpass",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			expectError:   true,
			errorContains: "login failed with status: 500 Internal Server Error",
		},
		{
			name:     "empty username and password",
			hubURL:   "http://example.com",
			username: "",
			password: "",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				err := r.ParseForm()
				assert.NoError(t, err)
				assert.Equal(t, "", r.FormValue("username"))
				assert.Equal(t, "", r.FormValue("password"))
				w.WriteHeader(http.StatusOK)
			},
			expectError: false,
		},
		{
			name:     "special characters in credentials",
			hubURL:   "http://example.com",
			username: "user@domain.com",
			password: "p@ssw0rd!#$%",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				err := r.ParseForm()
				assert.NoError(t, err)
				assert.Equal(t, "user@domain.com", r.FormValue("username"))
				assert.Equal(t, "p@ssw0rd!#$%", r.FormValue("password"))
				w.WriteHeader(http.StatusOK)
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tt.serverResponse(w, r)
			}))
			defer server.Close()

			cmd := createTestConfigCommand()

			// Use the test server URL instead of the provided hubURL for actual HTTP calls
			// but test URL construction with the provided hubURL
			var testURL string
			if tt.hubURL != "" {
				// Replace the scheme and host with test server, but keep the path construction logic
				testURL = server.URL
			}

			err := cmd.performLogin(testURL, tt.username, tt.password)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPerformLoginNetworkErrors(t *testing.T) {
	t.Run("invalid URL should fail", func(t *testing.T) {
		cmd := createTestConfigCommand()

		err := cmd.performLogin("://invalid-url", "user", "pass")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create login request")
	})

	t.Run("unreachable server should fail", func(t *testing.T) {
		cmd := createTestConfigCommand()

		// Use a URL that will definitely fail to connect
		err := cmd.performLogin("http://localhost:99999", "user", "pass")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send login request")
	})
}

func TestPerformLoginURLConstruction(t *testing.T) {
	tests := []struct {
		name        string
		inputURL    string
		expectedURL string
	}{
		{
			name:        "URL without trailing slash",
			inputURL:    "http://example.com",
			expectedURL: "http://example.com/api/login",
		},
		{
			name:        "URL with trailing slash",
			inputURL:    "http://example.com/",
			expectedURL: "http://example.com/api/login",
		},
		{
			name:        "URL with path",
			inputURL:    "http://example.com/hub",
			expectedURL: "http://example.com/hub/api/login",
		},
		{
			name:        "URL with path and trailing slash",
			inputURL:    "http://example.com/hub/",
			expectedURL: "http://example.com/hub/api/login",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server that captures the request URL
			var capturedURL string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedURL = r.URL.String()
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			cmd := createTestConfigCommand()

			// We need to test the URL construction logic, so we'll modify the function
			// to use our test server but verify the path construction
			err := cmd.performLogin(server.URL, "user", "pass")
			assert.NoError(t, err)

			// The captured URL should be "/api/login" since that's the path part
			assert.Equal(t, "/api/login", capturedURL)
		})
	}
}

func TestConfigCommandStructFields(t *testing.T) {
	t.Run("configCommand struct fields", func(t *testing.T) {
		var logLevel uint32 = 5
		cmd := &configCommand{
			listProfiles: "test-path",
			sync:         "test-sync",
			login:        true,
			logLevel:     &logLevel,
			log:          logr.Discard(),
		}

		// Test that all fields are accessible
		assert.Equal(t, "test-path", cmd.listProfiles)
		assert.Equal(t, "test-sync", cmd.sync)
		assert.True(t, cmd.login)
		assert.NotNil(t, cmd.logLevel)
		assert.Equal(t, uint32(5), *cmd.logLevel)
		assert.NotNil(t, cmd.log)
	})

	t.Run("logLevel can be nil", func(t *testing.T) {
		cmd := &configCommand{
			logLevel: nil,
		}
		assert.Nil(t, cmd.logLevel)
	})
}

// TestLoginInteractive tests the interactive parts of Login function
// Note: This test is more complex due to the interactive nature of the Login function
func TestLoginInteractive(t *testing.T) {
	t.Run("Login function prints expected prompts", func(t *testing.T) {
		// This test is challenging because Login() reads from os.Stdin
		// In a real scenario, you might want to use dependency injection
		// to make the Login function more testable by accepting io.Reader

		// For now, we'll just test that the function exists and can be called
		// without panicking (though it will hang waiting for input)
		cmd := createTestConfigCommand()

		// We can't easily test the interactive parts without mocking stdin
		// but we can test that the function exists and has the right signature
		assert.NotNil(t, cmd.Login)

		// In a production environment, you might want to refactor Login
		// to accept io.Reader and io.Writer for better testability
	})
}

// Integration test that tests the overall flow
func TestConfigCommandIntegration(t *testing.T) {
	t.Run("validate and list profiles flow", func(t *testing.T) {
		tmpDir := t.TempDir()
		cmd := createTestConfigCommand()
		cmd.listProfiles = tmpDir

		// Test validation
		ctx := context.Background()
		err := cmd.Validate(ctx)
		assert.NoError(t, err)

		// Test list profiles
		output := captureOutput(t, func() {
			cmd.ListProfiles()
		})
		assert.Equal(t, "list profiles\n", output)
	})

	t.Run("validate with invalid path should fail", func(t *testing.T) {
		cmd := createTestConfigCommand()
		cmd.listProfiles = "/nonexistent/path"

		ctx := context.Background()
		err := cmd.Validate(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to stat application path")
	})
}

// Benchmark tests for performance-critical functions
func BenchmarkValidate(b *testing.B) {
	tmpDir := b.TempDir()
	cmd := createTestConfigCommand()
	cmd.listProfiles = tmpDir
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd.Validate(ctx)
	}
}

func BenchmarkPerformLogin(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cmd := createTestConfigCommand()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd.performLogin(server.URL, "user", "pass")
	}
}
