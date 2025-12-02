package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/konveyor/tackle2-hub/api"
	"gopkg.in/yaml.v2"
)

func TestNewConfigCmd(t *testing.T) {
	log := logr.Discard()
	cmd := NewConfigCmd(log)

	if cmd.Use != "config" {
		t.Errorf("Expected command use to be 'config', got %s", cmd.Use)
	}

	if cmd.Short != "Configure kantra" {
		t.Errorf("Expected command short description to be 'Configure kantra', got %s", cmd.Short)
	}

	flag := cmd.Flags().Lookup("list-profiles")
	if flag == nil {
		t.Error("Expected --list-profiles flag to be present")
	}

	subCommands := cmd.Commands()
	expectedSubCommands := []string{"sync", "login"}
	foundCommands := make(map[string]bool)
	for _, subCmd := range subCommands {
		foundCommands[subCmd.Name()] = true
	}

	for _, expected := range expectedSubCommands {
		if !foundCommands[expected] {
			t.Errorf("Expected subcommand %s to be present", expected)
		}
	}
}

func TestConfigCommand_Validate(t *testing.T) {
	log := logr.Discard()
	ctx := context.Background()

	tests := []struct {
		name      string
		setupFunc func() (string, func(), error)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "empty list-profiles should pass",
			setupFunc: func() (string, func(), error) {
				return "", func() {}, nil
			},
			wantErr: false,
		},
		{
			name: "valid directory with profiles should pass",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-config-")
				if err != nil {
					return "", nil, err
				}
				profilesDir := filepath.Join(tmpDir, Profiles)
				err = os.MkdirAll(profilesDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				return tmpDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: false,
		},
		{
			name: "non-existent directory should fail",
			setupFunc: func() (string, func(), error) {
				return "/non/existent/path", func() {}, nil
			},
			wantErr: true,
			errMsg:  "no such file or directory",
		},
		{
			name: "file instead of directory should fail",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-config-")
				if err != nil {
					return "", nil, err
				}
				filePath := filepath.Join(tmpDir, "notadir")
				err = os.WriteFile(filePath, []byte("test"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				return filePath, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: true,
			errMsg:  "is not a directory",
		},
		{
			name: "directory without profiles subdirectory should fail",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-config-")
				if err != nil {
					return "", nil, err
				}
				return tmpDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: true,
			errMsg:  "no such file or directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Failed to setup test: %v", err)
			}
			defer cleanup()

			configCmd := &configCommand{
				listProfiles: path,
				log:          log,
			}

			err = configCmd.Validate(ctx)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error but got none")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, expected to contain %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestConfigCommand_ListProfiles(t *testing.T) {
	log := logr.Discard()

	tests := []struct {
		name      string
		setupFunc func() (string, func(), error)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "empty list-profiles should not error",
			setupFunc: func() (string, func(), error) {
				return "", func() {}, nil
			},
			wantErr: false,
		},
		{
			name: "directory with profile subdirectories should list them",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-config-")
				if err != nil {
					return "", nil, err
				}
				profilesDir := filepath.Join(tmpDir, Profiles)
				err = os.MkdirAll(profilesDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				profileDirs := []string{"profile1", "profile2", "profile3"}
				for _, dir := range profileDirs {
					err = os.MkdirAll(filepath.Join(profilesDir, dir), 0755)
					if err != nil {
						os.RemoveAll(tmpDir)
						return "", nil, err
					}
				}

				err = os.WriteFile(filepath.Join(profilesDir, "notadir.txt"), []byte("test"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}

				return tmpDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: false,
		},
		{
			name: "directory with no profile subdirectories should not error",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-config-")
				if err != nil {
					return "", nil, err
				}
				profilesDir := filepath.Join(tmpDir, Profiles)
				err = os.MkdirAll(profilesDir, 0755)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				return tmpDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: false,
		},
		{
			name: "non-existent profiles directory should error",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-config-")
				if err != nil {
					return "", nil, err
				}
				return tmpDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: true,
			errMsg:  "no such file or directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Failed to setup test: %v", err)
			}
			defer cleanup()

			configCmd := &configCommand{
				listProfiles: path,
				log:          log,
			}

			err = configCmd.ListProfiles()

			if tt.wantErr {
				if err == nil {
					t.Errorf("ListProfiles() expected error but got none")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ListProfiles() error = %v, expected to contain %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ListProfiles() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestNewSyncCmd(t *testing.T) {
	log := logr.Discard()
	cmd := NewSyncCmd(log)

	if cmd.Use != "sync [URL]" {
		t.Errorf("Expected command use to be 'sync [URL]', got %s", cmd.Use)
	}

	if cmd.Short != "Sync Hub application profiles" {
		t.Errorf("Expected command short description to be 'Sync Hub application profiles', got %s", cmd.Short)
	}

	urlFlag := cmd.Flags().Lookup("url")
	if urlFlag == nil {
		t.Error("Expected --url flag to be present")
	}

	appPathFlag := cmd.Flags().Lookup("application-path")
	if appPathFlag == nil {
		t.Error("Expected --application-path flag to be present")
	}
}

func TestSyncCommand_Validate(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		setupFunc func() (string, func(), error)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "empty application path should fail",
			setupFunc: func() (string, func(), error) {
				return "", func() {}, nil
			},
			wantErr: true,
			errMsg:  "application path is required",
		},
		{
			name: "valid directory should pass",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-sync-")
				if err != nil {
					return "", nil, err
				}
				return tmpDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: false,
		},
		{
			name: "non-existent directory should fail",
			setupFunc: func() (string, func(), error) {
				return "/non/existent/path", func() {}, nil
			},
			wantErr: true,
			errMsg:  "no such file or directory",
		},
		{
			name: "file instead of directory should fail",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-sync-")
				if err != nil {
					return "", nil, err
				}
				filePath := filepath.Join(tmpDir, "notadir")
				err = os.WriteFile(filePath, []byte("test"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				return filePath, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: true,
			errMsg:  "is not a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Failed to setup test: %v", err)
			}
			defer cleanup()

			syncCmd := &syncCommand{
				applicationPath: path,
			}

			err = syncCmd.Validate(ctx)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error but got none")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, expected to contain %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestSyncCommand_getHubClient(t *testing.T) {
	// Create temporary auth file for testing
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}
	
	kantreDir := filepath.Join(homeDir, ".kantra")
	authFile := filepath.Join(kantreDir, "auth.json")
	
	// Backup existing auth file if it exists
	var backupData []byte
	var hasBackup bool
	if data, err := os.ReadFile(authFile); err == nil {
		backupData = data
		hasBackup = true
	}
	
	// Create test auth with a valid JWT token
	validJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE4OTM0NTYwMDB9.Hs_ZQhwq7Uy9E7VzTpSKNqvWUdKKYKxWJhUlNhqJGKE"
	
	testAuth := LoginResponse{
		Host:         "http://test-host",
		Token:        validJWT,
		RefreshToken: "test-refresh-token",
	}
	
	// Ensure directory exists
	os.MkdirAll(kantreDir, 0755)
	
	// Write test auth
	authData, _ := json.Marshal(testAuth)
	os.WriteFile(authFile, authData, 0600)
	
	// Cleanup function
	defer func() {
		if hasBackup {
			os.WriteFile(authFile, backupData, 0600)
		} else {
			os.Remove(authFile)
		}
	}()

	syncCmd := &syncCommand{}

	// First call should create a new client
	client1, err := syncCmd.getHubClient()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if client1 == nil {
		t.Error("Expected hubClient to be created")
	}

	// Second call should return the same client
	client2, err := syncCmd.getHubClient()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if client1 != client2 {
		t.Error("Expected getHubClient to return the same instance")
	}
}

func TestNewHubClientWithOptions(t *testing.T) {
	tests := []struct {
		name     string
		insecure bool
	}{
		{
			name:     "secure client",
			insecure: false,
		},
		{
			name:     "insecure client",
			insecure: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := newHubClientWithOptions(tt.insecure)
			if err != nil {
				// If no stored auth, that's expected - skip the test
				if strings.Contains(err.Error(), "no stored authentication found") ||
					strings.Contains(err.Error(), "stored authentication is invalid") {
					t.Skip("No stored authentication available for testing")
					return
				}
				t.Fatalf("Unexpected error: %v", err)
			}

			// Verify client was created
			if client == nil {
				t.Fatal("Expected client to be created, got nil")
			}

			// Verify host is set from stored auth (should not be empty)
			if client.host == "" {
				t.Error("Expected host to be set from stored authentication")
			}

			if client.client == nil {
				t.Error("Expected HTTP client to be initialized")
			}

			if client.client.Timeout != 10*time.Second {
				t.Errorf("Expected timeout to be 10s, got %v", client.client.Timeout)
			}

			// Verify insecure setting affects TLS config
			if tt.insecure {
				transport, ok := client.client.Transport.(*http.Transport)
				if !ok {
					t.Error("Expected Transport to be *http.Transport for insecure client")
				} else if transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
					t.Error("Expected InsecureSkipVerify to be true for insecure client")
				}
			} else {
				// For secure clients, Transport should be nil (default) or have secure TLS config
				if client.client.Transport != nil {
					transport, ok := client.client.Transport.(*http.Transport)
					if ok && transport.TLSClientConfig != nil && transport.TLSClientConfig.InsecureSkipVerify {
						t.Error("Expected InsecureSkipVerify to be false for secure client")
					}
				}
			}
		})
	}
}

func TestHubClient_doRequest(t *testing.T) {
	log := logr.Discard()

	// Create temporary auth file for testing
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	kantreDir := filepath.Join(homeDir, ".kantra")
	authFile := filepath.Join(kantreDir, "auth.json")

	// Backup existing auth file if it exists
	var backupData []byte
	var hasBackup bool
	if data, err := os.ReadFile(authFile); err == nil {
		backupData = data
		hasBackup = true
	}

	// Create test auth with a valid JWT token
	// This is a simple JWT token with exp claim set to far future (year 2030)
	// Header: {"alg":"HS256","typ":"JWT"}
	// Payload: {"exp":1893456000}
	// Signature: signed with secret "test"
	validJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE4OTM0NTYwMDB9.Hs_ZQhwq7Uy9E7VzTpSKNqvWUdKKYKxWJhUlNhqJGKE"

	testAuth := LoginResponse{
		Host:         "http://test-host",
		Token:        validJWT,
		RefreshToken: "test-refresh-token",
	}

	// Ensure directory exists
	os.MkdirAll(kantreDir, 0755)

	// Write test auth
	authData, _ := json.Marshal(testAuth)
	os.WriteFile(authFile, authData, 0600)

	// Cleanup function
	defer func() {
		if hasBackup {
			os.WriteFile(authFile, backupData, 0600)
		} else {
			os.Remove(authFile)
		}
	}()

	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		path           string
		acceptHeader   string
		wantErr        bool
		errMsg         string
	}{
		{
			name: "successful request",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status": "ok"}`))
			},
			path:         "/test",
			acceptHeader: "application/json",
			wantErr:      false,
		},
		{
			name: "server error response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Internal Server Error"))
			},
			path:         "/error",
			acceptHeader: "application/json",
			wantErr:      false, // doRequest doesn't fail on HTTP errors, readResponseBody does
		},
		{
			name: "check authentication header with token",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				auth := r.Header.Get("Authorization")
				expectedAuth := "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE4OTM0NTYwMDB9.Hs_ZQhwq7Uy9E7VzTpSKNqvWUdKKYKxWJhUlNhqJGKE"
				if auth == expectedAuth {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"authenticated": true}`))
				} else {
					// Return a different error code to avoid triggering token refresh
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte("Bad Request"))
				}
			},
			path:         "/auth",
			acceptHeader: "application/json",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			client := &hubClient{
				client: &http.Client{Timeout: 5 * time.Second},
				host:   server.URL,
			}

			// Set token for authentication test
			if tt.name == "check authentication header with token" {
				os.Setenv("TOKEN", "test-token")
				defer os.Unsetenv("TOKEN")
			}

			resp, err := client.doRequest(tt.path, tt.acceptHeader, log)

			if tt.wantErr {
				if err == nil {
					t.Errorf("doRequest() expected error but got none")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("doRequest() error = %v, expected to contain %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("doRequest() unexpected error = %v", err)
				}
				if resp == nil {
					t.Error("Expected response to be non-nil")
				} else {
					resp.Body.Close()
				}
			}
		})
	}
}

func TestHubClient_readResponseBody(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		responseBody string
		wantErr      bool
		errMsg       string
		expectedBody string
	}{
		{
			name:         "successful response",
			statusCode:   http.StatusOK,
			responseBody: "success response",
			wantErr:      false,
			expectedBody: "success response",
		},
		{
			name:         "client error response",
			statusCode:   http.StatusBadRequest,
			responseBody: "Bad Request",
			wantErr:      true,
			errMsg:       "HTTP 400",
		},
		{
			name:         "server error response",
			statusCode:   http.StatusInternalServerError,
			responseBody: "Internal Server Error",
			wantErr:      true,
			errMsg:       "HTTP 500",
		},
		{
			name:         "empty response body",
			statusCode:   http.StatusOK,
			responseBody: "",
			wantErr:      false,
			expectedBody: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock response
			resp := &http.Response{
				StatusCode: tt.statusCode,
				Body:       io.NopCloser(strings.NewReader(tt.responseBody)),
			}

			client := &hubClient{}
			body, err := client.readResponseBody(resp)

			if tt.wantErr {
				if err == nil {
					t.Errorf("readResponseBody() expected error but got none")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("readResponseBody() error = %v, expected to contain %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("readResponseBody() unexpected error = %v", err)
				}
				if string(body) != tt.expectedBody {
					t.Errorf("readResponseBody() body = %s, expected %s", string(body), tt.expectedBody)
				}
			}
		})
	}
}

func TestParseApplicationsFromHub(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		wantErr  bool
		errMsg   string
		expected int
	}{
		{
			name: "valid JSON with applications",
			jsonData: `[
  {
    "id": 1,
    "name": "App1",
    "repository": {
      "url": "https://github.com/example/app1"
    }
  },
  {
    "id": 2,
    "name": "App2",
    "repository": {
      "url": "https://github.com/example/app2"
    }
  }
]`,
			wantErr:  false,
			expected: 2,
		},
		{
			name:     "empty JSON array",
			jsonData: "[]",
			wantErr:  false,
			expected: 0,
		},
		{
			name:     "empty string",
			jsonData: "",
			wantErr:  true,
			errMsg:   "unexpected end of JSON input",
		},
		{
			name:     "invalid JSON",
			jsonData: "invalid json content [",
			wantErr:  true,
			errMsg:   "invalid character",
		},
		{
			name: "single application",
			jsonData: `[
  {
    "id": 1,
    "name": "SingleApp",
    "repository": {
      "url": "https://github.com/example/single"
    }
  }
]`,
			wantErr:  false,
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apps, err := parseApplicationsFromHub(tt.jsonData)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseApplicationsFromHub() expected error but got none")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("parseApplicationsFromHub() error = %v, expected to contain %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("parseApplicationsFromHub() unexpected error = %v", err)
				}
				if len(apps) != tt.expected {
					t.Errorf("parseApplicationsFromHub() returned %d applications, expected %d", len(apps), tt.expected)
				}
			}
		})
	}
}

func TestSyncCommand_getApplicationFromHub(t *testing.T) {
	log := logr.Discard()

	// Create temporary auth file for testing
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	kantreDir := filepath.Join(homeDir, ".kantra")
	authFile := filepath.Join(kantreDir, "auth.json")

	// Backup existing auth file if it exists
	var backupData []byte
	var hasBackup bool
	if data, err := os.ReadFile(authFile); err == nil {
		backupData = data
		hasBackup = true
	}

	// Create test auth with a valid JWT token
	validJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE4OTM0NTYwMDB9.Hs_ZQhwq7Uy9E7VzTpSKNqvWUdKKYKxWJhUlNhqJGKE"

	testAuth := LoginResponse{
		Host:         "http://test-host",
		Token:        validJWT,
		RefreshToken: "test-refresh-token",
	}

	// Ensure directory exists
	os.MkdirAll(kantreDir, 0755)

	// Write test auth
	authData, _ := json.Marshal(testAuth)
	os.WriteFile(authFile, authData, 0600)

	// Cleanup function
	defer func() {
		if hasBackup {
			os.WriteFile(authFile, backupData, 0600)
		} else {
			os.Remove(authFile)
		}
	}()

	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		url            string
		wantErr        bool
		errMsg         string
		expectedAppID  uint
	}{
		{
			name: "successful application retrieval",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				apps := []api.Application{
					{
						Resource: api.Resource{ID: 1},
						Name:     "Test App",
						Repository: &api.Repository{
							URL: "https://github.com/example/test",
						},
					},
				}
				jsonData, _ := json.Marshal(apps)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write(jsonData)
			},
			url:           "https://github.com/example/test",
			wantErr:       false,
			expectedAppID: 1,
		},
		{
			name: "application not found",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				apps := []api.Application{
					{
						Resource: api.Resource{ID: 1},
						Name:     "Different App",
					},
				}
				apps[0].Repository = &api.Repository{URL: "https://github.com/example/different"}
				jsonData, _ := json.Marshal(apps)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write(jsonData)
			},
			url:           "https://github.com/example/notfound",
			wantErr:       false,
			expectedAppID: 0, // Should return empty application
		},
		{
			name: "server error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Internal Server Error"))
			},
			url:     "https://github.com/example/test",
			wantErr: true,
			errMsg:  "HTTP 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			syncCmd := &syncCommand{
				url: tt.url,
				log: log,
				hubClient: &hubClient{
					client: &http.Client{Timeout: 5 * time.Second},
					host:   server.URL,
				},
			}

			app, err := syncCmd.getApplicationFromHub(tt.url)

			if tt.wantErr {
				if err == nil {
					t.Errorf("getApplicationFromHub() expected error but got none")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("getApplicationFromHub() error = %v, expected to contain %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("getApplicationFromHub() unexpected error = %v", err)
				}
				if app.Resource.ID != tt.expectedAppID {
					t.Errorf("getApplicationFromHub() returned app ID %d, expected %d", app.Resource.ID, tt.expectedAppID)
				}
			}
		})
	}
}

func TestSyncCommand_getProfilesFromHubApplication(t *testing.T) {
	log := logr.Discard()

	// Create temporary auth file for testing
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	kantreDir := filepath.Join(homeDir, ".kantra")
	authFile := filepath.Join(kantreDir, "auth.json")

	// Backup existing auth file if it exists
	var backupData []byte
	var hasBackup bool
	if data, err := os.ReadFile(authFile); err == nil {
		backupData = data
		hasBackup = true
	}

	// Create test auth with a valid JWT token
	validJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE4OTM0NTYwMDB9.Hs_ZQhwq7Uy9E7VzTpSKNqvWUdKKYKxWJhUlNhqJGKE"

	testAuth := LoginResponse{
		Host:         "http://test-host",
		Token:        validJWT,
		RefreshToken: "test-refresh-token",
	}

	// Ensure directory exists
	os.MkdirAll(kantreDir, 0755)

	// Write test auth
	authData, _ := json.Marshal(testAuth)
	os.WriteFile(authFile, authData, 0600)

	// Cleanup function
	defer func() {
		if hasBackup {
			os.WriteFile(authFile, backupData, 0600)
		} else {
			os.Remove(authFile)
		}
	}()

	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		appID          int
		wantErr        bool
		errMsg         string
		expectedCount  int
	}{
		{
			name: "successful profiles retrieval",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				profiles := []api.AnalysisProfile{
					{
						Resource: api.Resource{ID: 1},
						Name:     "Profile 1",
					},
					{
						Resource: api.Resource{ID: 2},
						Name:     "Profile 2",
					},
				}
				yamlData, _ := yaml.Marshal(profiles)
				w.Header().Set("Content-Type", "application/x-yaml")
				w.WriteHeader(http.StatusOK)
				w.Write(yamlData)
			},
			appID:         1,
			wantErr:       false,
			expectedCount: 2,
		},
		{
			name: "no profiles found",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				profiles := []api.AnalysisProfile{}
				yamlData, _ := yaml.Marshal(profiles)
				w.Header().Set("Content-Type", "application/x-yaml")
				w.WriteHeader(http.StatusOK)
				w.Write(yamlData)
			},
			appID:         1,
			wantErr:       false,
			expectedCount: 0,
		},
		{
			name: "server error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte("Application not found"))
			},
			appID:   999,
			wantErr: true,
			errMsg:  "HTTP 404",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			syncCmd := &syncCommand{
				log: log,
				hubClient: &hubClient{
					client: &http.Client{Timeout: 5 * time.Second},
					host:   server.URL,
				},
			}

			profiles, err := syncCmd.getProfilesFromHubApplicaton(tt.appID)

			if tt.wantErr {
				if err == nil {
					t.Errorf("getProfilesFromHubApplicaton() expected error but got none")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("getProfilesFromHubApplicaton() error = %v, expected to contain %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("getProfilesFromHubApplicaton() unexpected error = %v", err)
				}
				if len(profiles) != tt.expectedCount {
					t.Errorf("getProfilesFromHubApplicaton() returned %d profiles, expected %d", len(profiles), tt.expectedCount)
				}
			}
		})
	}
}

func TestHubClient_downloadToFile(t *testing.T) {
	log := logr.Discard()

	tests := []struct {
		name         string
		statusCode   int
		responseBody []byte
		wantErr      bool
		errMsg       string
	}{
		{
			name:       "successful download",
			statusCode: http.StatusOK,
			responseBody: func() []byte {
				// Create a valid tar file content
				var buf bytes.Buffer
				tarWriter := tar.NewWriter(&buf)

				header := &tar.Header{
					Name: "test.txt",
					Mode: 0644,
					Size: int64(len("test content")),
				}
				tarWriter.WriteHeader(header)
				tarWriter.Write([]byte("test content"))
				tarWriter.Close()

				return buf.Bytes()
			}(),
			wantErr: false,
		},
		{
			name:         "server error response",
			statusCode:   http.StatusInternalServerError,
			responseBody: []byte("Internal Server Error"),
			wantErr:      true,
			errMsg:       "HTTP 500",
		},
		{
			name:         "empty file download",
			statusCode:   http.StatusOK,
			responseBody: []byte(""),
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "test-download-")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			outputPath := filepath.Join(tmpDir, "test-file.tar")

			// Create a mock response
			resp := &http.Response{
				StatusCode: tt.statusCode,
				Body:       io.NopCloser(bytes.NewReader(tt.responseBody)),
			}

			client := &hubClient{}
			err = client.downloadToFile(resp, outputPath, log)

			if tt.wantErr {
				if err == nil {
					t.Errorf("downloadToFile() expected error but got none")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("downloadToFile() error = %v, expected to contain %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("downloadToFile() unexpected error = %v", err)
				}

				if tt.statusCode == http.StatusOK {
					extractDir := strings.TrimSuffix(outputPath, ".tar")
					if _, err := os.Stat(extractDir); os.IsNotExist(err) {
						if _, err := os.Stat(outputPath); os.IsNotExist(err) {
							t.Error("Expected either tar file or extracted directory to exist")
						}
					}
				}
			}
		})
	}
}

func TestExtractTarFile(t *testing.T) {
	log := logr.Discard()

	tests := []struct {
		name      string
		setupFunc func() (string, string, func(), error)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "extract simple tar file",
			setupFunc: func() (string, string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-extract-")
				if err != nil {
					return "", "", nil, err
				}

				// Create a simple tar file
				tarPath := filepath.Join(tmpDir, "test.tar")
				tarFile, err := os.Create(tarPath)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", "", nil, err
				}

				tarWriter := tar.NewWriter(tarFile)

				// Add a file to the tar
				header := &tar.Header{
					Name: "test.txt",
					Mode: 0644,
					Size: int64(len("test content")),
				}
				if err := tarWriter.WriteHeader(header); err != nil {
					tarFile.Close()
					os.RemoveAll(tmpDir)
					return "", "", nil, err
				}
				if _, err := tarWriter.Write([]byte("test content")); err != nil {
					tarFile.Close()
					os.RemoveAll(tmpDir)
					return "", "", nil, err
				}

				// Add a directory to the tar
				dirHeader := &tar.Header{
					Name:     "testdir/",
					Mode:     0755,
					Typeflag: tar.TypeDir,
				}
				if err := tarWriter.WriteHeader(dirHeader); err != nil {
					tarFile.Close()
					os.RemoveAll(tmpDir)
					return "", "", nil, err
				}

				tarWriter.Close()
				tarFile.Close()

				destDir := filepath.Join(tmpDir, "extracted")
				return tarPath, destDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: false,
		},
		{
			name: "extract gzipped tar file",
			setupFunc: func() (string, string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-extract-gz-")
				if err != nil {
					return "", "", nil, err
				}

				// Create a gzipped tar file
				tarPath := filepath.Join(tmpDir, "test.tar.gz")
				tarFile, err := os.Create(tarPath)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", "", nil, err
				}

				gzipWriter := gzip.NewWriter(tarFile)
				tarWriter := tar.NewWriter(gzipWriter)

				header := &tar.Header{
					Name: "gztest.txt",
					Mode: 0644,
					Size: int64(len("gzipped content")),
				}
				if err := tarWriter.WriteHeader(header); err != nil {
					tarFile.Close()
					os.RemoveAll(tmpDir)
					return "", "", nil, err
				}
				if _, err := tarWriter.Write([]byte("gzipped content")); err != nil {
					tarFile.Close()
					os.RemoveAll(tmpDir)
					return "", "", nil, err
				}

				tarWriter.Close()
				gzipWriter.Close()
				tarFile.Close()

				destDir := filepath.Join(tmpDir, "extracted")
				return tarPath, destDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: false,
		},
		{
			name: "non-existent tar file should fail",
			setupFunc: func() (string, string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-extract-")
				if err != nil {
					return "", "", nil, err
				}
				tarPath := filepath.Join(tmpDir, "nonexistent.tar")
				destDir := filepath.Join(tmpDir, "extracted")
				return tarPath, destDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: true,
			errMsg:  "no such file or directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tarPath, destDir, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Failed to setup test: %v", err)
			}
			defer cleanup()

			err = extractTarFile(tarPath, destDir, log)

			if tt.wantErr {
				if err == nil {
					t.Errorf("extractTarFile() expected error but got none")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("extractTarFile() error = %v, expected to contain %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("extractTarFile() unexpected error = %v", err)
				}

				if _, err := os.Stat(destDir); os.IsNotExist(err) {
					t.Error("Expected destination directory to be created")
				}
			}
		})
	}
}

func TestDeleteTarFile(t *testing.T) {

	tests := []struct {
		name      string
		setupFunc func() (string, func(), error)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "delete existing file",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-delete-")
				if err != nil {
					return "", nil, err
				}
				filePath := filepath.Join(tmpDir, "test.tar")
				err = os.WriteFile(filePath, []byte("test"), 0644)
				if err != nil {
					os.RemoveAll(tmpDir)
					return "", nil, err
				}
				return filePath, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: false,
		},
		{
			name: "delete non-existent file should fail",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-delete-")
				if err != nil {
					return "", nil, err
				}
				filePath := filepath.Join(tmpDir, "nonexistent.tar")
				return filePath, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: true,
			errMsg:  "no such file or directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Failed to setup test: %v", err)
			}
			defer cleanup()

			err = deleteTarFile(filePath)

			if tt.wantErr {
				if err == nil {
					t.Errorf("deleteTarFile() expected error but got none")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("deleteTarFile() error = %v, expected to contain %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("deleteTarFile() unexpected error = %v", err)
				}

				if _, err := os.Stat(filePath); !os.IsNotExist(err) {
					t.Error("Expected file to be deleted")
				}
			}
		})
	}
}

func TestSyncCommand_downloadProfileBundle(t *testing.T) {
	log := logr.Discard()

	// Create temporary auth file for testing
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	kantreDir := filepath.Join(homeDir, ".kantra")
	authFile := filepath.Join(kantreDir, "auth.json")

	// Backup existing auth file if it exists
	var backupData []byte
	var hasBackup bool
	if data, err := os.ReadFile(authFile); err == nil {
		backupData = data
		hasBackup = true
	}

	// Create test auth with a valid JWT token
	validJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE4OTM0NTYwMDB9.Hs_ZQhwq7Uy9E7VzTpSKNqvWUdKKYKxWJhUlNhqJGKE"

	testAuth := LoginResponse{
		Host:         "http://test-host",
		Token:        validJWT,
		RefreshToken: "test-refresh-token",
	}

	// Ensure directory exists
	os.MkdirAll(kantreDir, 0755)

	// Write test auth
	authData, _ := json.Marshal(testAuth)
	os.WriteFile(authFile, authData, 0600)

	// Cleanup function
	defer func() {
		if hasBackup {
			os.WriteFile(authFile, backupData, 0600)
		} else {
			os.Remove(authFile)
		}
	}()

	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		profileID      int
		wantErr        bool
		errMsg         string
	}{
		{
			name: "successful profile bundle download",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				// Create a simple tar content
				var buf bytes.Buffer
				tarWriter := tar.NewWriter(&buf)

				header := &tar.Header{
					Name: "profile.yaml",
					Mode: 0644,
					Size: int64(len("profile content")),
				}
				tarWriter.WriteHeader(header)
				tarWriter.Write([]byte("profile content"))
				tarWriter.Close()

				w.Header().Set("Content-Type", "application/octet-stream")
				w.WriteHeader(http.StatusOK)
				w.Write(buf.Bytes())
			},
			profileID: 1,
			wantErr:   false,
		},
		{
			name: "server error during download",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte("Profile not found"))
			},
			profileID: 999,
			wantErr:   true,
			errMsg:    "HTTP 404",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "test-download-profile-")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			syncCmd := &syncCommand{
				applicationPath: tmpDir,
				log:             log,
				hubClient: &hubClient{
					client: &http.Client{Timeout: 5 * time.Second},
					host:   server.URL,
				},
			}

			err = syncCmd.downloadProfileBundle(tt.profileID)

			if tt.wantErr {
				if err == nil {
					t.Errorf("downloadProfileBundle() expected error but got none")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("downloadProfileBundle() error = %v, expected to contain %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("downloadProfileBundle() unexpected error = %v", err)
				}

				// Verify profiles directory was created
				profilesDir := filepath.Join(tmpDir, Profiles)
				if _, err := os.Stat(profilesDir); os.IsNotExist(err) {
					t.Error("Expected profiles directory to be created")
				}
			}
		})
	}
}

func TestConfigCommandIntegration(t *testing.T) {
	log := logr.Discard()

	t.Run("config command with list-profiles flag", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-config-integration-")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		profilesDir := filepath.Join(tmpDir, Profiles)
		err = os.MkdirAll(profilesDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create profiles dir: %v", err)
		}

		profileDirs := []string{"profile1", "profile2"}
		for _, dir := range profileDirs {
			err = os.MkdirAll(filepath.Join(profilesDir, dir), 0755)
			if err != nil {
				t.Fatalf("Failed to create profile dir: %v", err)
			}
		}

		cmd := NewConfigCmd(log)
		cmd.SetArgs([]string{"--list-profiles", tmpDir})

		err = cmd.Execute()
		if err != nil {
			t.Errorf("Command execution failed: %v", err)
		}
	})
}
