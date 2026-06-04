package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/go-logr/logr"
	hubapi "github.com/konveyor/tackle2-hub/shared/api"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const defaultPATLifespanHours = 168

type loginCommand struct {
	logLevel *uint32
	log      logr.Logger
	insecure bool
	host     string
}

func NewLoginCmd(log logr.Logger) *cobra.Command {
	loginCmd := &loginCommand{}
	loginCmd.log = log

	cmd := &cobra.Command{
		Use:   "login [host]",
		Short: "Login to the Hub and store authentication tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			if val, err := cmd.Flags().GetUint32("log-level"); err == nil {
				loginCmd.logLevel = &val
			}
			if len(args) > 0 {
				loginCmd.host = args[0]
			}
			return loginCmd.login()
		},
	}

	cmd.Flags().BoolVarP(&loginCmd.insecure, "insecure", "k", false, "skip TLS certificate verification")

	return cmd
}

func (l *loginCommand) login() error {
	l.log.Info("logging into Hub")

	reader := bufio.NewReader(os.Stdin)
	if l.host == "" {
		fmt.Print("Host: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read host: %w", err)
		}
		l.host = strings.TrimSpace(input)
	}

	host, err := normalizeTackleBaseURL(l.host)
	if err != nil {
		return err
	}

	token, err := l.resolveToken(host)
	if err != nil {
		return err
	}

	l.log.Info("validating Hub credentials", "host", host)
	if err := validateHubToken(host, token, l.insecure); err != nil {
		return fmt.Errorf("hub authentication failed: %w", err)
	}

	if err := storeAuth(&AuthConfig{Host: host, Token: token}); err != nil {
		return err
	}

	l.log.Info("login successful", "host", host)
	return nil
}

func (l *loginCommand) resolveToken(host string) (string, error) {
	if token := hubTokenFromEnv(); token != "" {
		return token, nil
	}

	if isTerminal(os.Stdin) {
		l.log.Info("starting OIDC login")
		return deviceLogin(host, l.insecure)
	}

	return l.readTokenFromPrompt()
}

func (l *loginCommand) readTokenFromPrompt() (string, error) {
	fmt.Print("Token: ")
	tokenBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", fmt.Errorf("failed to read token: %w", err)
	}
	fmt.Println()

	token := strings.TrimSpace(string(tokenBytes))
	if token == "" {
		return "", fmt.Errorf("Hub personal access token is required (%s)", loginHint)
	}
	return token, nil
}

func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// authenticates via OIDC (device flow or refresh), then upgrades to a Hub API key (PAT).
func deviceLogin(host string, insecure bool) (token string, err error) {
	client, oidc, err := newHubBindingClientWithOIDC(host, insecure)
	if err != nil {
		return "", err
	}

	if err := oidc.Login(); err != nil {
		return "", fmt.Errorf("OIDC login failed: %w", err)
	}

	apiPAT := &hubapi.PAT{Lifespan: defaultPATLifespanHours}
	if err := client.Token.Create(apiPAT); err != nil {
		return "", fmt.Errorf("create Hub personal access token: %w", err)
	}
	token = apiPAT.String()
	if token == "" {
		return "", fmt.Errorf("Hub did not return a personal access token")
	}
	return token, nil
}
