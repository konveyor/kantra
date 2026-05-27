package config

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	hubapi "github.com/konveyor/tackle2-hub/shared/api"
)

func TestNewLoginCmd(t *testing.T) {
	cmd := NewLoginCmd(logr.Discard())

	if cmd.Use != "login [host]" {
		t.Errorf("Use = %q", cmd.Use)
	}
	if cmd.Flags().Lookup("insecure") == nil {
		t.Error("expected --insecure flag")
	}
	if cmd.Flags().Lookup("token") != nil {
		t.Error("did not expect --token flag")
	}
	if cmd.Flags().Lookup("pat") != nil {
		t.Error("did not expect --pat flag")
	}
}

func TestLoginCommand_resolveToken_prefersHUB_TOKEN(t *testing.T) {
	t.Setenv(hubTokenEnvVar, "env-pat-token")
	t.Cleanup(func() { t.Setenv(hubTokenEnvVar, "") })

	l := &loginCommand{log: logr.Discard()}
	token, err := l.resolveToken("https://hub.example.com")
	if err != nil {
		t.Fatalf("resolveToken() error = %v", err)
	}
	if token != "env-pat-token" {
		t.Errorf("token = %q, want env-pat-token", token)
	}
}

func TestLoginCommand_login_storesHUB_TOKEN(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != hubapi.UsersRoute {
			t.Errorf("path = %q, want %q", r.URL.Path, hubapi.UsersRoute)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer env-pat" {
			t.Errorf("Authorization = %q", auth)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]hubapi.User{})
	}))
	defer server.Close()

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv(hubTokenEnvVar, "env-pat")
	t.Cleanup(func() { t.Setenv(hubTokenEnvVar, "") })

	l := &loginCommand{
		log:  logr.Discard(),
		host: server.URL,
	}
	if err := l.login(); err != nil {
		t.Fatalf("login() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tempHome, ".kantra", authFileName))
	if err != nil {
		t.Fatalf("read auth file: %v", err)
	}
	var stored AuthConfig
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("unmarshal auth: %v", err)
	}
	if stored.Host != server.URL {
		t.Errorf("stored host = %q", stored.Host)
	}
	if stored.Token != "env-pat" {
		t.Errorf("stored token = %q", stored.Token)
	}
}

func TestDeviceLogin_oidcAuthorizationFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oidc/device_authorization" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := deviceLogin(server.URL, false)
	if err == nil {
		t.Fatal("expected error from device authorization failure")
	}
	if !strings.Contains(err.Error(), "OIDC login failed") {
		t.Errorf("error = %v, want OIDC login failure", err)
	}
}

func TestDeviceLogin_tokenCreateFailure(t *testing.T) {
	var baseURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oidc/device_authorization":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code":      "device-code",
				"user_code":        "ABCD-EFGH",
				"verification_uri": baseURL + "/device",
				"interval":         0,
			})
		case "/oidc/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "oidc-access",
				"refresh_token": "oidc-refresh",
			})
		case hubapi.AuthTokensRoute:
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"forbidden"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	baseURL = server.URL

	_, err := deviceLogin(server.URL, false)
	if err == nil {
		t.Fatal("expected error when PAT creation fails")
	}
	if !strings.Contains(err.Error(), "create Hub personal access token") {
		t.Errorf("error = %v", err)
	}
}
