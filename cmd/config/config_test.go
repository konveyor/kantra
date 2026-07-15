package config

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/profile"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	hubapi "github.com/konveyor/tackle2-hub/shared/api"
	hubfilter "github.com/konveyor/tackle2-hub/shared/binding/filter"
)

const testPAT = "test-hub-personal-access-token"

// setupTempAuth creates a temporary HOME directory and writes auth data to ~/.kantra/auth.json.
func setupTempAuth(t *testing.T, auth *AuthConfig) string {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	// GetKantraDir prefers XDG_CONFIG_HOME on Linux; pin the config dir for tests.
	t.Setenv("XDG_CONFIG_HOME", "")

	kantreDir := filepath.Join(tempHome, util.ConfigDirBasename())
	t.Setenv(util.KantraDirEnv, kantreDir)
	if err := os.MkdirAll(kantreDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	// Write auth data if provided
	if auth != nil {
		authFile := filepath.Join(kantreDir, "auth.json")
		authData, err := json.Marshal(auth)
		if err != nil {
			t.Fatalf("Failed to marshal auth data: %v", err)
		}
		err = os.WriteFile(authFile, authData, 0600)
		if err != nil {
			t.Fatalf("Failed to write auth file: %v", err)
		}
	}

	return tempHome
}

func mustTestHubClient(t *testing.T, serverURL, token string) *hubClient {
	t.Helper()
	hc, err := newHubClient(serverURL, token, false)
	if err != nil {
		t.Fatalf("newHubClient() error = %v", err)
	}
	return hc
}

func mustTestHubClientNoAuth(t *testing.T, serverURL string) *hubClient {
	t.Helper()
	hc, err := newHubClientNoAuth(serverURL, false)
	if err != nil {
		t.Fatalf("newHubClientNoAuth() error = %v", err)
	}
	return hc
}

func TestNewConfigCmd(t *testing.T) {
	log := logr.Discard()
	cmd := NewConfigCmd(log)

	if cmd.Use != "config" {
		t.Errorf("Expected command use to be 'config', got %s", cmd.Use)
	}

	if cmd.Short != "Configure kantra" {
		t.Errorf("Expected command short description to be 'Configure kantra', got %s", cmd.Short)
	}

	// Check that --list-profiles flag is no longer present
	flag := cmd.Flags().Lookup("list-profiles")
	if flag != nil {
		t.Error("Expected --list-profiles flag to be removed")
	}

	subCommands := cmd.Commands()
	expectedSubCommands := []string{"sync", "login", "list"}
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

	// Test that the main config command validation now always passes
	// since list-profiles functionality moved to subcommand
	configCmd := &configCommand{
		log: log,
	}

	err := configCmd.Validate(ctx)
	if err != nil {
		t.Errorf("Validate() unexpected error = %v", err)
	}
}

func TestNewListCmd(t *testing.T) {
	log := logr.Discard()
	cmd := NewListCmd(log)

	if cmd.Use != "list" {
		t.Errorf("Expected command use to be 'list', got %s", cmd.Use)
	}

	if cmd.Short != "List local Hub profiles in the application" {
		t.Errorf("Expected command short description to be 'List local Hub profiles in the application', got %s", cmd.Short)
	}

	profileDirFlag := cmd.Flags().Lookup("profile-dir")
	if profileDirFlag == nil {
		t.Error("Expected --profile-dir flag to be present")
	}
}

func TestListCommand_Validate(t *testing.T) {
	log := logr.Discard()
	ctx := context.Background()

	tests := []struct {
		name      string
		setupFunc func() (string, func(), error)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "empty profile-dir should fail",
			setupFunc: func() (string, func(), error) {
				return "", func() {}, nil
			},
			wantErr: true,
			errMsg:  "--profile-dir is required",
		},
		{
			name: "valid directory with profiles should pass",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-list-")
				if err != nil {
					return "", nil, err
				}
				profilesDir := filepath.Join(tmpDir, profile.Profiles)
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
				tmpDir, err := os.MkdirTemp("", "test-list-")
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
				tmpDir, err := os.MkdirTemp("", "test-list-")
				if err != nil {
					return "", nil, err
				}
				return tmpDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: true,
			errMsg:  "profiles directory not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Failed to setup test: %v", err)
			}
			defer cleanup()

			listCmd := &listCommand{
				profileDir: path,
				log:        log,
			}

			err = listCmd.Validate(ctx)

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

func TestListCommand_ListProfiles(t *testing.T) {
	log := logr.Discard()

	tests := []struct {
		name      string
		setupFunc func() (string, func(), error)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "directory with profile subdirectories should list them",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-list-")
				if err != nil {
					return "", nil, err
				}
				profilesDir := filepath.Join(tmpDir, profile.Profiles)
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
				tmpDir, err := os.MkdirTemp("", "test-list-")
				if err != nil {
					return "", nil, err
				}
				profilesDir := filepath.Join(tmpDir, profile.Profiles)
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
				tmpDir, err := os.MkdirTemp("", "test-list-")
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

			listCmd := &listCommand{
				profileDir: path,
				log:        log,
			}

			err = listCmd.ListProfiles()

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
	hostFlag := cmd.Flags().Lookup("host")
	if hostFlag == nil {
		t.Error("Expected --host flag to be present")
	}
	binaryFlag := cmd.Flags().Lookup("binary")
	if binaryFlag == nil {
		t.Error("Expected --binary flag to be present")
	}
	profilePathFlag := cmd.Flags().Lookup("profile-path")
	if profilePathFlag == nil {
		t.Error("Expected --profile-path flag to be present")
	}
}

func TestSyncCommand_Validate(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		url       string
		binary    string
		setupFunc func() (string, func(), error)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "empty application path should use default (current directory)",
			url:  "https://example.com/repo",
			setupFunc: func() (string, func(), error) {
				return "", func() {}, nil
			},
			wantErr: false,
		},
		{
			name: "valid directory should pass",
			url:  "https://example.com/repo",
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
			url:  "https://example.com/repo",
			setupFunc: func() (string, func(), error) {
				return "/non/existent/path", func() {}, nil
			},
			wantErr: true,
			errMsg:  "no such file or directory",
		},
		{
			name: "file instead of directory should fail (repository mode)",
			url:  "https://example.com/repo",
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
		{
			name: "neither url nor binary should fail",
			setupFunc: func() (string, func(), error) {
				return "", func() {}, nil
			},
			wantErr: true,
			errMsg:  "either --url",
		},
		{
			name:   "binary with empty profile path should fail",
			binary: "/path/to/app.war",
			setupFunc: func() (string, func(), error) {
				return "", func() {}, nil
			},
			wantErr: true,
			errMsg:  "--profile-path is required when syncing a binary",
		},
		{
			name:   "binary with directory path should pass",
			binary: "app.war",
			setupFunc: func() (string, func(), error) {
				tmpDir, err := os.MkdirTemp("", "test-sync-")
				if err != nil {
					return "", nil, err
				}
				return tmpDir, func() { os.RemoveAll(tmpDir) }, nil
			},
			wantErr: false,
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
				url:             tt.url,
				binary:          tt.binary,
				applicationPath: path,
				profilePath:     path,
			}
			if tt.binary != "" {
				syncCmd.applicationPath = ""
			} else {
				syncCmd.profilePath = ""
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
	setupTempAuth(t, &AuthConfig{
		Host:  "http://test-host",
		Token: testPAT,
	})

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

func TestNewHubClientFromAuth(t *testing.T) {
	setupTempAuth(t, &AuthConfig{
		Host:  "http://test-host",
		Token: testPAT,
	})

	tests := []struct {
		name     string
		insecure bool
	}{
		{name: "secure client", insecure: false},
		{name: "insecure client", insecure: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := newHubClientFromAuth(tt.insecure)
			if err != nil {
				t.Fatalf("newHubClientFromAuth() error = %v", err)
			}
			if client == nil {
				t.Fatal("expected hub client")
			}
			if client.host != "http://test-host" {
				t.Errorf("host = %q, want http://test-host", client.host)
			}
			if client.binding == nil {
				t.Error("expected binding client")
			}
			if tt.insecure {
				tr := client.binding.Client.Transport()
				if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
					t.Error("expected insecure TLS config")
				}
			}
		})
	}
}

func TestNewHubClientNoAuth(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		insecure bool
	}{
		{
			name:     "secure client with http host",
			host:     "http://test-hub.example.com",
			insecure: false,
		},
		{
			name:     "secure client with https host",
			host:     "https://test-hub.example.com",
			insecure: false,
		},
		{
			name:     "host without scheme should add http",
			host:     "test-hub.example.com",
			insecure: false,
		},
		{
			name:     "insecure client",
			host:     "https://test-hub.example.com",
			insecure: true,
		},
		{
			name:     "host with trailing slash should be trimmed",
			host:     "http://test-hub.example.com/",
			insecure: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := newHubClientNoAuth(tt.host, tt.insecure)
			if err != nil {
				t.Fatalf("newHubClientNoAuth() unexpected error: %v", err)
			}

			if client == nil {
				t.Fatal("Expected client to be created, got nil")
			}

			if strings.HasSuffix(client.host, "/") {
				t.Error("Expected host to not have trailing slash")
			}
			if !strings.HasPrefix(client.host, "http://") && !strings.HasPrefix(client.host, "https://") {
				t.Error("Expected host to have http:// or https:// prefix")
			}
			if client.binding == nil {
				t.Error("expected binding client")
			}
			if tt.insecure {
				tr := client.binding.Client.Transport()
				if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
					t.Error("expected insecure TLS config")
				}
			}
		})
	}
}

func TestHubClient_listApplications_noAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("expected no Authorization header, got %q", auth)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.URL.Path != hubAPIPath(hubapi.ApplicationsRoute) {
			t.Errorf("path = %q, want %q", r.URL.Path, hubAPIPath(hubapi.ApplicationsRoute))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]hubapi.Application{})
	}))
	defer server.Close()

	client := mustTestHubClientNoAuth(t, server.URL)
	if _, err := client.listApplications(hubfilter.Filter{}); err != nil {
		t.Fatalf("listApplications() error = %v", err)
	}
}

func TestHubClient_listApplications_bearer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer "+testPAT {
			t.Errorf("Authorization = %q, want Bearer %q", auth, testPAT)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]hubapi.Application{})
	}))
	defer server.Close()

	client := mustTestHubClient(t, server.URL, testPAT)
	if _, err := client.listApplications(hubfilter.Filter{}); err != nil {
		t.Fatalf("listApplications() error = %v", err)
	}
}

func TestSyncCommand_getHubClientWithHost(t *testing.T) {
	setupTempAuth(t, nil)

	syncCmd := &syncCommand{host: "http://test-hub.example.com"}
	client, err := syncCmd.getHubClient()
	if err != nil {
		t.Fatalf("getHubClient() error = %v", err)
	}
	if client.host != "http://test-hub.example.com" {
		t.Errorf("host = %q, want http://test-hub.example.com", client.host)
	}
	if client.binding == nil {
		t.Error("expected binding client")
	}
}

func TestSyncCommand_getHubClientWithHostUsesStoredAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer "+testPAT {
			t.Errorf("Authorization = %q, want Bearer %q", auth, testPAT)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]hubapi.Application{})
	}))
	defer server.Close()

	setupTempAuth(t, &AuthConfig{Host: server.URL, Token: testPAT})

	syncCmd := &syncCommand{host: server.URL}
	client, err := syncCmd.getHubClient()
	if err != nil {
		t.Fatalf("getHubClient() error = %v", err)
	}
	if _, err := client.listApplications(hubfilter.Filter{}); err != nil {
		t.Fatalf("listApplications() error = %v", err)
	}
}

func TestSyncCommand_getApplicationFromHub(t *testing.T) {
	log := logr.Discard()
	setupTempAuth(t, &AuthConfig{Host: "http://test-host", Token: testPAT})

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
				apps := []hubapi.Application{
					{
						Resource: hubapi.Resource{ID: 1},
						Name:     "Test App",
						Repository: &hubapi.Repository{
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
			name: "repository URL matches without .git suffix",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				apps := []hubapi.Application{
					{
						Resource: hubapi.Resource{ID: 2},
						Name:     "Git suffix app",
						Repository: &hubapi.Repository{
							URL: "https://github.com/example/test",
						},
					},
				}
				jsonData, _ := json.Marshal(apps)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write(jsonData)
			},
			url:           "https://github.com/example/test.git",
			wantErr:       false,
			expectedAppID: 2,
		},
		{
			name: "application not found",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				apps := []hubapi.Application{
					{
						Resource: hubapi.Resource{ID: 1},
						Name:     "Different App",
					},
				}
				apps[0].Repository = &hubapi.Repository{URL: "https://github.com/example/different"}
				jsonData, _ := json.Marshal(apps)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write(jsonData)
			},
			url:           "https://github.com/example/notfound",
			wantErr:       true,
			errMsg:        "no applications found",
			expectedAppID: 0,
		},
		{
			name: "server error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Internal Server Error"))
			},
			url:     "https://github.com/example/test",
			wantErr: true,
			errMsg:  "500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			syncCmd := &syncCommand{
				url:       tt.url,
				log:       log,
				hubClient: mustTestHubClient(t, server.URL, testPAT),
			}

			app, err := syncCmd.getApplicationFromHub()

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
				if app.ID != tt.expectedAppID {
					t.Errorf("getApplicationFromHub() returned app ID %d, expected %d", app.ID, tt.expectedAppID)
				}
			}
		})
	}
}

func TestSyncCommand_getProfilesFromHubApplication(t *testing.T) {
	log := logr.Discard()
	setupTempAuth(t, &AuthConfig{Host: "http://test-host", Token: testPAT})

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
				profiles := []hubapi.AnalysisProfile{
					{
						Resource: hubapi.Resource{ID: 1},
						Name:     "Profile 1",
					},
					{
						Resource: hubapi.Resource{ID: 2},
						Name:     "Profile 2",
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(profiles)
			},
			appID:         1,
			wantErr:       false,
			expectedCount: 2,
		},
		{
			name: "no profiles found",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]hubapi.AnalysisProfile{})
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
			errMsg:  "404",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			syncCmd := &syncCommand{
				log:       log,
				hubClient: mustTestHubClient(t, server.URL, testPAT),
			}

			profiles, err := syncCmd.getProfilesFromHubApplication(tt.appID)

			if tt.wantErr {
				if err == nil {
					t.Errorf("getProfilesFromHubApplication() expected error but got none")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("getProfilesFromHubApplication() error = %v, expected to contain %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("getProfilesFromHubApplication() unexpected error = %v", err)
				}
				if len(profiles) != tt.expectedCount {
					t.Errorf("getProfilesFromHubApplication() returned %d profiles, expected %d", len(profiles), tt.expectedCount)
				}
			}
		})
	}
}

func TestSyncCommand_sync_noProfiles(t *testing.T) {
	log := logr.Discard()
	setupTempAuth(t, &AuthConfig{Host: "http://test-host", Token: testPAT})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case hubAPIPath(hubapi.ApplicationsRoute):
			apps := []hubapi.Application{
				{
					Resource: hubapi.Resource{ID: 1},
					Name:     "book-server",
					Repository: &hubapi.Repository{
						URL: "https://github.com/ibraginsky/book-server.git",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(apps)
		case "/hub/applications/1/analysis/profiles":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]hubapi.AnalysisProfile{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	syncCmd := &syncCommand{
		log:             log,
		url:             "https://github.com/ibraginsky/book-server.git",
		applicationPath: tmpDir,
		hubClient:       mustTestHubClient(t, server.URL, testPAT),
	}

	err := syncCmd.sync()
	if err == nil {
		t.Fatal("sync() expected error when Hub returns no profiles")
	}
	if !strings.Contains(err.Error(), "no analysis profiles found") {
		t.Errorf("sync() error = %v, want message about missing profiles", err)
	}
	if !strings.Contains(err.Error(), "book-server") {
		t.Errorf("sync() error = %v, want application name in message", err)
	}
}

func TestHubClient_downloadProfileBundle(t *testing.T) {
	log := logr.Discard()

	var tarBytes bytes.Buffer
	tarWriter := tar.NewWriter(&tarBytes)
	_ = tarWriter.WriteHeader(&tar.Header{
		Name: "profile.yaml",
		Mode: 0644,
		Size: int64(len("profile content")),
	})
	_, _ = tarWriter.Write([]byte("profile content"))
	_ = tarWriter.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/bundle") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tarBytes.Bytes())
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	tarPath := filepath.Join(tmpDir, "profile-1.tar")

	client := mustTestHubClient(t, server.URL, testPAT)
	if err := client.downloadProfileBundle(1, tarPath, log); err != nil {
		t.Fatalf("downloadProfileBundle() error = %v", err)
	}
	extractDir := trimTarExtension(tarPath)
	if _, err := os.Stat(extractDir); err != nil {
		t.Fatalf("expected extracted directory: %v", err)
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

func TestExtractTarFile_removesStaleFiles(t *testing.T) {
	log := logr.Discard()
	tmpDir, err := os.MkdirTemp("", "test-extract-stale-")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	destDir := filepath.Join(tmpDir, "profile-1")
	stalePath := filepath.Join(destDir, "rules", "removed-rule.yaml")
	if err := os.MkdirAll(filepath.Join(destDir, "rules"), 0755); err != nil {
		t.Fatalf("mkdir rules: %v", err)
	}
	if err := os.WriteFile(stalePath, []byte("stale"), 0644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}

	tarPath := filepath.Join(tmpDir, "bundle.tar")
	tarFile, err := os.Create(tarPath)
	if err != nil {
		t.Fatalf("create tar: %v", err)
	}
	tarWriter := tar.NewWriter(tarFile)
	header := &tar.Header{
		Name: "profile.yaml",
		Mode: 0644,
		Size: int64(len("name: test")),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		tarFile.Close()
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tarWriter.Write([]byte("name: test")); err != nil {
		tarFile.Close()
		t.Fatalf("tar body: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		tarFile.Close()
		t.Fatalf("tar close: %v", err)
	}
	if err := tarFile.Close(); err != nil {
		t.Fatalf("close tar file: %v", err)
	}

	if err := extractTarFile(tarPath, destDir, log); err != nil {
		t.Fatalf("extractTarFile: %v", err)
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Errorf("expected stale rule file removed, still at %s", stalePath)
	}
	profilePath := filepath.Join(destDir, "profile.yaml")
	if _, err := os.Stat(profilePath); err != nil {
		t.Errorf("expected profile.yaml from bundle: %v", err)
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
	setupTempAuth(t, &AuthConfig{Host: "http://test-host", Token: testPAT})

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
			errMsg:    "404",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			syncCmd := &syncCommand{
				applicationPath: tmpDir,
				log:             log,
				hubClient:       mustTestHubClient(t, server.URL, testPAT),
			}

			err := syncCmd.downloadProfileBundle(tt.profileID)

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
				profilesDir := filepath.Join(tmpDir, profile.Profiles)
				if _, err := os.Stat(profilesDir); os.IsNotExist(err) {
					t.Error("Expected profiles directory to be created")
				}
			}
		})
	}
}

func TestConfigCommandIntegration(t *testing.T) {
	log := logr.Discard()

	t.Run("config list subcommand with profile-dir flag", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-config-integration-")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		profilesDir := filepath.Join(tmpDir, profile.Profiles)
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
		cmd.SetArgs([]string{"list", "--profile-dir", tmpDir})

		err = cmd.Execute()
		if err != nil {
			t.Errorf("Command execution failed: %v", err)
		}
	})

	t.Run("config list subcommand without profile-dir flag should fail", func(t *testing.T) {
		cmd := NewConfigCmd(log)
		cmd.SetArgs([]string{"list"})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("Expected command execution to fail without --profile-dir")
		}
		if !strings.Contains(err.Error(), `required flag(s) "profile-dir" not set`) {
			t.Errorf("Expected required flag error, got: %v", err)
		}
	})
}

func TestSyncCommand_setDefaultApplicationPath(t *testing.T) {
	syncCmd := &syncCommand{}

	path, err := syncCmd.setDefaultApplicationPath()

	if err != nil {
		t.Errorf("setDefaultApplicationPath() unexpected error = %v", err)
	}

	if path == "" {
		t.Error("setDefaultApplicationPath() returned empty path")
	}

	// Verify it returns a valid directory path
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("setDefaultApplicationPath() returned non-existent path: %s", path)
	}
}

func TestSyncCommand_checkAuthentication(t *testing.T) {
	tests := []struct {
		name      string
		setupAuth func(t *testing.T)
		host      string
		wantErr   bool
		errMsg    string
	}{
		{
			name: "valid authentication",
			setupAuth: func(t *testing.T) {
				setupTempAuth(t, &AuthConfig{
					Host:  "http://test-host",
					Token: testPAT,
				})
			},
			wantErr: false,
		},
		{
			name: "missing token should fail",
			setupAuth: func(t *testing.T) {
				tempHome := t.TempDir()
				t.Setenv("HOME", tempHome)
				kantraDir := filepath.Join(tempHome, ".kantra")
				_ = os.MkdirAll(kantraDir, 0700)
				authFile := filepath.Join(kantraDir, "auth.json")
				legacyAuth := `{"host":"http://test-host","token":"","refresh":"old-refresh"}`
				_ = os.WriteFile(authFile, []byte(legacyAuth), 0600)
			},
			wantErr: true,
			errMsg:  "previous username/password login format",
		},
		{
			name: "no auth file should fail",
			setupAuth: func(t *testing.T) {
				setupTempAuth(t, nil)
			},
			wantErr: true,
			errMsg:  "no stored Hub authentication found",
		},
		{
			name: "host flag bypasses authentication",
			setupAuth: func(t *testing.T) {
				// Set up temp HOME but don't write any auth file
				setupTempAuth(t, nil)
			},
			host:    "http://test-hub.example.com",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupAuth(t)

			syncCmd := &syncCommand{
				host: tt.host,
			}
			err := syncCmd.checkAuthentication()

			if tt.wantErr {
				if err == nil {
					t.Errorf("checkAuthentication() expected error but got none")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("checkAuthentication() error = %v, expected to contain %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("checkAuthentication() unexpected error = %v", err)
				}
			}
		})
	}
}
