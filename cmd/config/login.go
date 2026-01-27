package config

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
	"github.com/golang-jwt/jwt/v5"
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

type RefreshRequest struct {
	RefreshToken string `json:"refresh"`
}

type loginCommand struct {
	logLevel  *uint32
	log       logr.Logger
	hubClient *hubClient
	insecure  bool
	host      string
	user      string
	password  string
}

func NewLoginCmd(log logr.Logger) *cobra.Command {
	loginCmd := &loginCommand{}
	loginCmd.log = log

	loginCommand := &cobra.Command{
		Use:   "login [host] [user] [password]",
		Short: "Login to the Hub and store authentication tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			if val, err := cmd.Flags().GetUint32("log-level"); err == nil {
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

			return loginCmd.Login(log)
		},
	}

	loginCommand.Flags().BoolVarP(&loginCmd.insecure, "insecure", "k", false, "skip TLS certificate verification")

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
		return err
	}
	loginResp, err := l.performLogin(l.host, l.user, l.password)
	if err != nil {
		return err
	}
	if err := l.storeTokens(loginResp); err != nil {
		return err
	}

	l.log.Info("login successful", "user", loginResp.User)
	return nil
}

func (l *loginCommand) performLogin(hubURL, username, password string) (*LoginResponse, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	if l.insecure {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client.Transport = tr
	}
	baseURL, err := url.Parse(strings.TrimSuffix(hubURL, "/"))
	if err != nil {
		return nil, err
	}
	loginURL := baseURL.JoinPath("auth", "login").String()
	loginReq := LoginRequest{
		User:     username,
		Password: password,
	}

	jsonData, err := json.Marshal(loginReq)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", loginURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	l.log.Info("Attempting login", "url", loginURL, "user", username)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("login failed with status %d: %s", resp.StatusCode, string(body))
	}
	var loginResp LoginResponse
	if err := json.Unmarshal(body, &loginResp); err != nil {
		return nil, err
	}
	// Empty token is valid when authentication is disabled on Hub
	// It returns 201 with token="" when auth is disabled
	loginResp.Host = hubURL

	return &loginResp, nil
}

func (l *loginCommand) RefreshToken(hubURL string, loginResp *LoginResponse) (*LoginResponse, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	if l.insecure {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client.Transport = tr
	}
	baseURL, err := url.Parse(strings.TrimSuffix(hubURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("invalid hub URL: %w", err)
	}
	refreshURL := baseURL.JoinPath("auth", "refresh").String()

	if loginResp.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}
	refreshReq := RefreshRequest{
		RefreshToken: loginResp.RefreshToken,
	}
	jsonData, err := json.Marshal(refreshReq)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", refreshURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var newLoginResp LoginResponse
	if err := json.Unmarshal(body, &newLoginResp); err != nil {
		return nil, err
	}
	if newLoginResp.Host == "" {
		newLoginResp.Host = loginResp.Host
	}

	return &newLoginResp, nil
}

func (l *loginCommand) storeTokens(loginResp *LoginResponse) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	kantraDir := filepath.Join(homeDir, ".kantra")
	if err := os.MkdirAll(kantraDir, 0700); err != nil {
		return err
	}

	tokenFile := filepath.Join(kantraDir, "auth.json")
	jsonData, err := json.MarshalIndent(loginResp, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tokenFile, jsonData, 0600); err != nil {
		return err
	}
	return nil
}

func loadStoredTokens() (*LoginResponse, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	tokenFile := filepath.Join(homeDir, ".kantra", "auth.json")
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no stored authentication found, please login")
		}
		return nil, err
	}
	var loginResp LoginResponse
	if err := json.Unmarshal(data, &loginResp); err != nil {
		return nil, err
	}

	return &loginResp, nil
}

func (hc *hubClient) refreshStoredToken(storedAuth *LoginResponse, log logr.Logger) error {
	loginCmd := &loginCommand{
		log:      log,
		insecure: hc.insecure,
		host:     hc.host,
	}
	newTokens, err := loginCmd.RefreshToken(hc.host, storedAuth)
	if err != nil {
		return err
	}
	if err := loginCmd.storeTokens(newTokens); err != nil {
		return err
	}

	return nil
}

func isTokenExpired(token string) (bool, error) {
	if token == "" {
		return true, nil
	}
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	claims := jwt.MapClaims{}
	_, _, err := parser.ParseUnverified(token, claims)
	if err != nil {
		return true, err
	}
	if exp, ok := claims["exp"]; ok {
		if expFloat, ok := exp.(float64); ok {
			expTime := time.Unix(int64(expFloat), 0)
			return time.Now().After(expTime), nil
		}
		return false, fmt.Errorf("invalid expiration claim format in token")
	}

	return false, nil
}
