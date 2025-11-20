package cmd

import (
	"bufio"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type loginCommand struct {
	logLevel  *uint32
	log       logr.Logger
	hubClient *hubClient
}

func NewLoginCmd(log logr.Logger) *cobra.Command {
	loginCmd := &loginCommand{}
	loginCmd.log = log

	loginCommand := &cobra.Command{
		Use:   "login",
		Short: "Login to the Hub",
		RunE: func(cmd *cobra.Command, args []string) error {
			if val, err := cmd.Flags().GetUint32(logLevelFlag); err == nil {
				loginCmd.logLevel = &val
			}

			loginCmd.Login(log)
			return nil
		},
	}

	return loginCommand
}

// *** TODO ***
func (l *loginCommand) Login(log logr.Logger) {
	log.Info("logging into Hub")

	fmt.Print("URL: ")
	reader := bufio.NewReader(os.Stdin)
	hubURL, err := reader.ReadString('\n')
	if err != nil {
		l.log.Error(err, "failed to read URL")
		return
	}
	hubURL = strings.TrimSpace(hubURL)

	if _, err := url.ParseRequestURI(hubURL); err != nil {
		l.log.Error(err, "invalid URL format")
		return
	}

	fmt.Print("username: ")
	username, err := reader.ReadString('\n')
	if err != nil {
		l.log.Error(err, "failed to read username")
		return
	}
	username = strings.TrimSpace(username)

	fmt.Print("password: ")
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		l.log.Error(err, "failed to read password")
		return
	}
	password := string(passwordBytes)

	err = l.performLogin(hubURL, username, password)
	if err != nil {
		l.log.Error(err, "login failed")
		fmt.Println("Login failed:", err)
		return
	}

	l.log.Info("Login successful")
}

func (l *loginCommand) performLogin(hubURL, username, password string) error {
	client := &http.Client{}

	// TODO for login
	loginURL := strings.TrimSuffix(hubURL, "/") + "/api/login"

	data := url.Values{}
	data.Set("username", username)
	data.Set("password", password)

	req, err := http.NewRequest("POST", loginURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "kantra-cli")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send login request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed with status: %s", resp.Status)
	}

	// *** TODO: Handle authentication token/session storage ***

	return nil
}
