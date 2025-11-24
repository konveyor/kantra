package cmd

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type LoginRequest struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh"`
	User         string `json:"user,omitempty"`
	ExpiresAt    int    `json:"expiry,omitempty"`
	Host         string `json:"host,omitempty"`
}

type loginCommand struct {
	logLevel  *uint32
	log       logr.Logger
	hubClient *hubClient
	insecure  bool
	host      string
	user      string
	password  string
	refresh   bool
}

func NewLoginCmd(log logr.Logger) *cobra.Command {
	loginCmd := &loginCommand{}
	loginCmd.log = log

	loginCommand := &cobra.Command{
		Use:   "login [host] [user] [password]",
		Short: "Login to the Hub and store authentication tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			if val, err := cmd.Flags().GetUint32(logLevelFlag); err == nil {
				loginCmd.logLevel = &val
			}
			if len(args) > 0 {
				loginCmd.host = args[0]
			}
			if len(args) > 1 {
				loginCmd.user = args[1]
			}
			if len(args) > 2 {
				loginCmd.password = args[2]
			}
			if loginCmd.refresh {
				return loginCmd.Refresh(log)
			}

			return loginCmd.Login(log)
		},
	}

	loginCommand.Flags().BoolVarP(&loginCmd.insecure, "insecure", "k", false, "skip TLS certificate verification")
	loginCommand.Flags().BoolVarP(&loginCmd.refresh, "refresh", "r", false, "refresh existing authentication tokens")

	return loginCommand
}

func (l *loginCommand) Login(log logr.Logger) error {
	log.Info("logging into Hub")
	reader := bufio.NewReader(os.Stdin)
	if l.host == "" {
		fmt.Print("Host: ")
		if input, err := reader.ReadString('\n'); err == nil {
			l.host = strings.TrimSpace(input)
		}
		if l.host == "" {
			return fmt.Errorf("host is required")
		}
	}
	if l.user == "" {
		fmt.Print("Username: ")
		if input, err := reader.ReadString('\n'); err == nil {
			l.user = strings.TrimSpace(input)
		}
		if l.user == "" {
			return fmt.Errorf("username is required")
		}
	}
	if l.password == "" {
		fmt.Print("Password: ")
		passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		l.password = string(passwordBytes)
		fmt.Println()
	}

	if !strings.HasPrefix(l.host, "http://") && !strings.HasPrefix(l.host, "https://") {
		l.host = "http://" + l.host
	}

	if _, err := url.ParseRequestURI(l.host); err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	loginResp, err := l.performLogin(l.host, l.user, l.password)
	if err != nil {
		l.log.Error(err, "login failed")
		return fmt.Errorf("login failed: %w", err)
	}

	if err := l.storeTokens(loginResp); err != nil {
		l.log.Error(err, "failed to store tokens")
		return fmt.Errorf("failed to store tokens: %w", err)
	}

	l.log.Info("Login successful", "user", loginResp.User)
	return nil
}

func (l *loginCommand) performLogin(hubURL, username, password string) (*LoginResponse, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	if l.insecure {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client.Transport = tr
	}
	loginURL := strings.TrimSuffix(hubURL, "/") + "/auth/login"
	loginReq := LoginRequest{
		User:     username,
		Password: password,
	}

	jsonData, err := json.Marshal(loginReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal login request: %w", err)
	}
	req, err := http.NewRequest("POST", loginURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "kantra-cli")

	l.log.Info("Attempting login", "url", loginURL, "user", username)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send login request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("login failed with status %d: %s", resp.StatusCode, string(body))
	}
	var loginResp LoginResponse
	if err := json.Unmarshal(body, &loginResp); err != nil {
		return nil, fmt.Errorf("failed to parse login response: %w", err)
	}
	if loginResp.Token == "" {
		return nil, fmt.Errorf("login response missing token")
	}

	return &loginResp, nil
}

func (l *loginCommand) RefreshToken(hubURL string, loginResp *LoginResponse) (*LoginResponse, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	if l.insecure {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client.Transport = tr
	}

	refreshURL := strings.TrimSuffix(hubURL, "/") + "/auth/refresh"
	jsonData, err := json.Marshal(loginResp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal refresh request: %w", err)
	}

	req, err := http.NewRequest("POST", refreshURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "kantra-cli")

	l.log.Info("Attempting token refresh", "url", refreshURL)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send refresh request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read refresh response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var newLoginResp LoginResponse
	if err := json.Unmarshal(body, &newLoginResp); err != nil {
		return nil, fmt.Errorf("failed to parse refresh response: %w", err)
	}

	return &newLoginResp, nil
}

func (l *loginCommand) storeTokens(loginResp *LoginResponse) error {
	if loginResp.Host == "" && l.host != "" {
		loginResp.Host = l.host
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}
	kantraDir := filepath.Join(homeDir, ".kantra")
	if err := os.MkdirAll(kantraDir, 0700); err != nil {
		return fmt.Errorf("failed to create kantra directory: %w", err)
	}

	tokenFile := filepath.Join(kantraDir, "auth.json")
	jsonData, err := json.MarshalIndent(loginResp, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal tokens: %w", err)
	}

	if err := os.WriteFile(tokenFile, jsonData, 0600); err != nil {
		return fmt.Errorf("failed to write token file: %w", err)
	}
	l.log.V(7).Info("tokens stored", "file", tokenFile)
	return nil
}

func LoadStoredTokens() (*LoginResponse, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	tokenFile := filepath.Join(homeDir, ".kantra", "auth.json")
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no stored authentication found, please run 'kantra login' first")
		}
		return nil, fmt.Errorf("failed to read token file: %w", err)
	}

	var loginResp LoginResponse
	if err := json.Unmarshal(data, &loginResp); err != nil {
		return nil, fmt.Errorf("failed to parse stored tokens: %w", err)
	}

	return &loginResp, nil
}

func (l *loginCommand) Refresh(log logr.Logger) error {
	log.V(7).Info("refreshing authentication tokens")
	storedTokens, err := LoadStoredTokens()
	if err != nil {
		return fmt.Errorf("failed to load stored tokens: %w", err)
	}
	if l.host == "" {
		return fmt.Errorf("host is required for refresh (provide as argument)")
	}
	if !strings.HasPrefix(l.host, "http://") && !strings.HasPrefix(l.host, "https://") {
		l.host = "http://" + l.host
	}

	newTokens, err := l.RefreshToken(l.host, storedTokens)
	if err != nil {
		return fmt.Errorf("failed to refresh tokens: %w", err)
	}
	if err := l.storeTokens(newTokens); err != nil {
		return fmt.Errorf("failed to store refreshed tokens: %w", err)
	}

	l.log.V(7).Info("tokens refreshed successfully", "user", newTokens.User)
	return nil
}

func (hc *hubClient) refreshStoredToken(storedAuth *LoginResponse, log logr.Logger) error {
	loginCmd := &loginCommand{
		log:      log,
		insecure: hc.insecure,
		host:     hc.host,
	}
	newTokens, err := loginCmd.RefreshToken(hc.host, storedAuth)
	if err != nil {
		return fmt.Errorf("failed to refresh token: %w", err)
	}

	if err := loginCmd.storeTokens(newTokens); err != nil {
		return fmt.Errorf("failed to store refreshed tokens: %w", err)
	}
	log.V(7).Info("token automatically refreshed and stored")
	return nil
}
