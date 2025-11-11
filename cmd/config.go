package cmd

import (
	"bufio"
	"context"
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

type configCommand struct {
	listProfiles string
	sync         string
	login        bool
	logLevel     *uint32
	log          logr.Logger
}

func NewConfigCmd(log logr.Logger) *cobra.Command {
	configCmd := &configCommand{}
	configCmd.log = log

	configCommand := &cobra.Command{
		Use:   "config",
		Short: "Configure kantra",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			err := configCmd.Validate(cmd.Context())
			if err != nil {
				log.Error(err, "failed to validate flags")
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if val, err := cmd.Flags().GetUint32(logLevelFlag); err == nil {
				configCmd.logLevel = &val
			}

			if configCmd.listProfiles != "" {
				configCmd.ListProfiles()
				return nil
			}

			if configCmd.login {
				configCmd.Login()
				return nil
			}

			return nil
		},
	}

	configCommand.Flags().StringVar(&configCmd.listProfiles, "list-profiles", "", "list local Hub profiles in the application")
	configCommand.Flags().StringVar(&configCmd.sync, "sync", "", "get the applicaiton profiles from the Hub")
	configCommand.Flags().BoolVar(&configCmd.login, "login", false, "login to the Hub")
	return configCommand
}

func (c *configCommand) Validate(ctx context.Context) error {
	if c.listProfiles != "" {
		stat, err := os.Stat(c.listProfiles)
		if err != nil {
			return fmt.Errorf("%w failed to stat application path for profile %s", err, c.listProfiles)
		}
		if !stat.IsDir() {
			return fmt.Errorf("application path for profile %s is not a directory", c.listProfiles)
		}
		return nil
	}

	return nil
}

func (c *configCommand) ListProfiles() {
	fmt.Println("list profiles")
}

func (c *configCommand) Login() {
	fmt.Println("logging into Hub")

	fmt.Print("URL: ")
	reader := bufio.NewReader(os.Stdin)
	hubURL, err := reader.ReadString('\n')
	if err != nil {
		c.log.Error(err, "failed to read URL")
		return
	}
	hubURL = strings.TrimSpace(hubURL)

	if _, err := url.ParseRequestURI(hubURL); err != nil {
		c.log.Error(err, "invalid URL format")
		return
	}

	fmt.Print("username: ")
	username, err := reader.ReadString('\n')
	if err != nil {
		c.log.Error(err, "failed to read username")
		return
	}
	username = strings.TrimSpace(username)

	fmt.Print("password: ")
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		c.log.Error(err, "failed to read password")
		return
	}
	password := string(passwordBytes)
	fmt.Println() // Add newline after password input

	err = c.performLogin(hubURL, username, password)
	if err != nil {
		c.log.Error(err, "login failed")
		fmt.Println("Login failed:", err)
		return
	}

	fmt.Println("Login successful")
}

func (c *configCommand) performLogin(hubURL, username, password string) error {
	client := &http.Client{}

	// TODO
	loginURL := strings.TrimSuffix(hubURL, "/") + "/api/login"

	data := url.Values{}
	data.Set("username", username)
	data.Set("password", password)

	// POST request
	req, err := http.NewRequest("POST", loginURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "kantra-cli")

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send login request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed with status: %s", resp.Status)
	}

	// TODO: Handle authentication token/session storage here
	// You might want to save the authentication token or session cookie
	// for future API calls

	return nil
}
