package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	hubTokenEnvVar = "HUB_TOKEN"
	authFileName   = "auth.json"
	loginHint      = "run `kantra config login` (OIDC), set HUB_TOKEN to use an existing PAT, or create one in the Hub UI"
)

type AuthConfig struct {
	Host  string `json:"host"`
	Token string `json:"token"`
}

func (a *AuthConfig) Validate() error {
	if a == nil {
		return fmt.Errorf("authentication config is nil")
	}
	if strings.TrimSpace(a.Host) == "" {
		return fmt.Errorf("stored authentication is missing hub host; %s", loginHint)
	}
	if strings.TrimSpace(a.Token) == "" {
		return fmt.Errorf("stored authentication is missing a Hub personal access token; %s", loginHint)
	}
	return nil
}

func kantraConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".kantra"), nil
}

func ensureKantraConfigDir() (string, error) {
	dir, err := kantraConfigDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

func authFilePath() (string, error) {
	dir, err := kantraConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, authFileName), nil
}

func storeAuth(auth *AuthConfig) error {
	if err := auth.Validate(); err != nil {
		return err
	}
	normalizedHost, err := normalizeHubHost(auth.Host)
	if err != nil {
		return err
	}
	auth.Host = normalizedHost

	dir, err := ensureKantraConfigDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, authFileName)
	data, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func loadStoredAuth() (*AuthConfig, error) {
	path, err := authFilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no stored Hub authentication found; %s", loginHint)
		}
		return nil, err
	}
	var auth AuthConfig
	if err := json.Unmarshal(data, &auth); err != nil {
		return nil, fmt.Errorf("invalid auth file %s: %w", path, err)
	}
	if auth.Token == "" && legacyRefreshTokenPresent(data) {
		return nil, fmt.Errorf("stored credentials use the previous username/password login format; %s", loginHint)
	}
	if err := auth.Validate(); err != nil {
		return nil, err
	}
	auth.Host = strings.TrimSuffix(auth.Host, "/")
	return &auth, nil
}

func hubTokenFromEnv() string {
	return strings.TrimSpace(os.Getenv(hubTokenEnvVar))
}

func hubHostsEqual(a, b string) bool {
	na, err := normalizeHubHost(a)
	if err != nil {
		return false
	}
	nb, err := normalizeHubHost(b)
	if err != nil {
		return false
	}
	return na == nb
}

func normalizeHubHost(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", fmt.Errorf("host is required")
	}
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "http://" + host
	}
	host = strings.TrimSuffix(host, "/")
	if _, err := url.ParseRequestURI(host); err != nil {
		return "", fmt.Errorf("invalid hub URL: %w", err)
	}
	return host, nil
}

func legacyRefreshTokenPresent(data []byte) bool {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return false
	}
	refresh, ok := raw["refresh"]
	if !ok {
		return false
	}
	return string(refresh) != `""` && string(refresh) != "null"
}
