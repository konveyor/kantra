package cmd

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/konveyor/tackle2-hub/api"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

type configCommand struct {
	logLevel  *uint32
	log       logr.Logger
	hubClient *hubClient
	insecure  bool
}

type syncCommand struct {
	url             string
	applicationPath string
	logLevel        *uint32
	log             logr.Logger
	hubClient       *hubClient
	insecure        bool
}

type listCommand struct {
	profileDir string
	logLevel   *uint32
	log        logr.Logger
}

type hubClient struct {
	client   *http.Client
	host     string
	insecure bool
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

			return nil
		},
	}

	configCommand.PersistentFlags().BoolVarP(&configCmd.insecure, "insecure", "k", false, "Skip TLS certificate verification")

	configCommand.AddCommand(NewSyncCmd(log))
	configCommand.AddCommand(NewLoginCmd(log))
	configCommand.AddCommand(NewListCmd(log))

	return configCommand
}

func (c *configCommand) Validate(ctx context.Context) error {
	return nil
}

func NewListCmd(log logr.Logger) *cobra.Command {
	listCmd := &listCommand{}
	listCmd.log = log

	listCommand := &cobra.Command{
		Use:   "list",
		Short: "List local Hub profiles in the application",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			err := listCmd.Validate(cmd.Context())
			if err != nil {
				log.Error(err, "failed to validate flags")
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if val, err := cmd.Flags().GetUint32("log-level"); err == nil {
				listCmd.logLevel = &val
			}

			err := listCmd.ListProfiles()
			if err != nil {
				return err
			}
			return nil
		},
	}

	listCommand.Flags().StringVar(&listCmd.profileDir, "profile-dir", "", "application directory path to look for profiles. Default is the current directory")

	return listCommand
}

func (l *listCommand) Validate(ctx context.Context) error {
	if l.profileDir == "" {
		currentDir, err := os.Getwd()
		if err != nil {
			return err
		}
		l.profileDir = currentDir
	}

	stat, err := os.Stat(l.profileDir)
	if err != nil {
		return err
	}
	if !stat.IsDir() {
		return fmt.Errorf("application path for profile %s is not a directory", l.profileDir)
	}
	profilesDir := filepath.Join(l.profileDir, profile.Profiles)
	if _, err := os.Stat(profilesDir); os.IsNotExist(err) {
		return err
	}
	return nil
}

func (l *listCommand) ListProfiles() error {
	profilesDir := filepath.Join(l.profileDir, Profiles)
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		return err
	}
	var profileDirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			profileDirs = append(profileDirs, entry.Name())
		}
	}

	if len(profileDirs) == 0 {
		l.log.Info("no profile directories found in: %s", profilesDir)
		return nil
	}
	fmt.Fprintln(os.Stdout, "profiles found in", l.profileDir)
	for _, dir := range profileDirs {
		fmt.Fprintln(os.Stdout, dir)
	}

	return nil
}

func NewSyncCmd(log logr.Logger) *cobra.Command {
	syncCmd := &syncCommand{}
	syncCmd.log = log

	syncCommand := &cobra.Command{
		Use:   "sync [URL]",
		Short: "Sync Hub application profiles",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			err := syncCmd.Validate(cmd.Context())
			if err != nil {
				log.Error(err, "failed to validate flags")
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if val, err := cmd.Flags().GetUint32(logLevelFlag); err == nil {
				syncCmd.logLevel = &val
			}
			if insecure, err := cmd.Parent().PersistentFlags().GetBool("insecure"); err == nil {
				syncCmd.insecure = insecure
			}
			application, err := syncCmd.getApplicationFromHub(syncCmd.url)
			if err != nil {
				log.Error(err, "failed to get applications from Hub")
				return err
			}
			profiles, err := syncCmd.getProfilesFromHubApplicaton(int(application.Resource.ID))
			if err != nil {
				return err
			}
			for _, profile := range profiles {
				log.Info("downloading profile", "name", profile.Name, "ID", profile.ID)
				err = syncCmd.downloadProfileBundle(int(profile.ID))
				if err != nil {
					log.Error(err, "failed to download profile bundle", "profileID", profile.ID, "profileName", profile.Name)
					return err
				}
			}
			return nil
		},
	}

	syncCommand.Flags().StringVar(&syncCmd.url, "url", "", "url of the remote application repository")
	syncCommand.MarkFlagRequired("url")
	syncCommand.Flags().StringVar(&syncCmd.applicationPath, "application-path", "", "path to the local application to download Hub profiles")
	syncCommand.MarkFlagRequired("application-path")

	return syncCommand
}

func (s *syncCommand) Validate(ctx context.Context) error {
	if s.applicationPath == "" {
		return fmt.Errorf("application path is required")
	}
	stat, err := os.Stat(s.applicationPath)
	if err != nil {
		return err
	}
	if !stat.IsDir() {
		return fmt.Errorf("application path %s is not a directory", s.applicationPath)
	}

	return nil
}

func (s *syncCommand) getHubClient() (*hubClient, error) {
	var err error
	if s.hubClient == nil {
		s.hubClient, err = newHubClientWithOptions(s.insecure)
		if err != nil {
			return nil, err
		}
	}
	return s.hubClient, nil
}

func newHubClientWithOptions(insecure bool) (*hubClient, error) {
	storedAuth, err := loadStoredTokens()
	if err != nil {
		return nil, err
	}
	if storedAuth.Host == "" {
		return nil, fmt.Errorf("stored authentication is invalid. Please login")
	}
	host := strings.TrimSuffix(storedAuth.Host, "/")
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	if insecure {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client.Transport = tr
	}
	return &hubClient{
		client:   client,
		host:     host,
		insecure: insecure,
	}, nil
}

func (hc *hubClient) doRequest(path, acceptHeader string, log logr.Logger) (*http.Response, error) {
	return hc.doRequestWithRetry(path, acceptHeader, log, false)
}

func (hc *hubClient) doRequestWithRetry(path, acceptHeader string, log logr.Logger, isRetry bool) (*http.Response, error) {
	url := hc.host + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", acceptHeader)
	storedAuth, err := loadStoredTokens()
	if err != nil {
		return nil, err
	}

	var token string
	if storedAuth != nil {
		expired, err := isTokenExpired(storedAuth.Token)
		if err != nil {
			return nil, err
		}
		if expired && !isRetry {
			// refresh if token if expired
			if err := hc.refreshStoredToken(storedAuth, log); err != nil {
				return nil, err
			}
			if refreshedAuth, err := loadStoredTokens(); err == nil {
				token = refreshedAuth.Token
			}
		} else if !expired {
			token = storedAuth.Token
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := hc.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized && !isRetry {
		resp.Body.Close()
		// refresh token if unauthorized
		if err := hc.refreshStoredToken(storedAuth, log); err != nil {
			return nil, err
		}
		return hc.doRequestWithRetry(path, acceptHeader, log, true)
	}

	return resp, nil
}

func (hc *hubClient) readResponseBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (s *syncCommand) checkAuthentication() error {
	storedAuth, err := loadStoredTokens()
	if err != nil {
		return err
	}
	if storedAuth.Token == "" {
		return fmt.Errorf("stored authentication is invalid. Please login")
	}

	return nil
}

func (s *syncCommand) getApplicationFromHub(urlRepo string) (api.Application, error) {
	if err := s.checkAuthentication(); err != nil {
		return api.Application{}, err
	}
	hubClient, err := s.getHubClient()
	if err != nil {
		return api.Application{}, err
	}
	path := fmt.Sprintf("/applications?filter=repository.url='%s'", urlRepo)

	resp, err := hubClient.doRequest(path, "application/json", s.log)
	if err != nil {
		return api.Application{}, err
	}
	body, err := hubClient.readResponseBody(resp)
	if err != nil {
		return api.Application{}, err
	}

	apps, err := parseApplicationsFromHub(string(body))
	if err != nil {
		return api.Application{}, err
	}
	if len(apps) == 0 {
		return api.Application{}, fmt.Errorf("no applications found in Hub for URL: %s", urlRepo)

		// TODO handle multiple applications later
	} else if len(apps) > 1 {
		return api.Application{}, fmt.Errorf("multiple applications found in Hub for URL: %s", urlRepo)
	}
	var application api.Application
	if apps[0].Repository.URL == s.url {
		application = apps[0]
	} else {
		return api.Application{}, fmt.Errorf("URL mismatch: expected %s, got %s", s.url, apps[0].Repository.URL)
	}

	return application, nil
}

func parseApplicationsFromHub(jsonData string) ([]api.Application, error) {
	var apps []api.Application

	err := json.Unmarshal([]byte(jsonData), &apps)
	if err != nil {
		return nil, err
	}

	return apps, nil
}

func (s *syncCommand) getProfilesFromHubApplicaton(appID int) ([]api.AnalysisProfile, error) {
	hubClient, err := s.getHubClient()
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/applications/%d/analysis/profiles", appID)
	resp, err := hubClient.doRequest(path, "application/x-yaml", s.log)
	if err != nil {
		return nil, err
	}
	body, err := hubClient.readResponseBody(resp)
	if err != nil {
		return nil, err
	}
	profiles := []api.AnalysisProfile{}
	if err := yaml.Unmarshal(body, &profiles); err != nil {
		return nil, err
	}

	return profiles, nil
}

func (s *syncCommand) downloadProfileBundle(profileID int) error {
	hubClient, err := s.getHubClient()
	if err != nil {
		return err
	}
	filename := fmt.Sprintf("profile-%d.tar", profileID)
	downloadPath := filepath.Join(s.applicationPath, Profiles, filename)
	err = os.MkdirAll(filepath.Join(s.applicationPath, Profiles), 0755)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/analysis/profiles/%d/bundle", profileID)
	resp, err := hubClient.doRequest(path, "application/octet-stream", s.log)
	if err != nil {
		return err
	}

	return hubClient.downloadToFile(resp, downloadPath, s.log)
}

func (hc *hubClient) downloadToFile(resp *http.Response, outputPath string, log logr.Logger) error {
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return err
	}
	log.V(7).Info("compressed bundle file downloaded successfully", "path", outputPath)

	extractDir := strings.TrimSuffix(outputPath, ".tar")
	err = extractTarFile(outputPath, extractDir, log)
	if err != nil {
		return err
	}
	err = deleteTarFile(outputPath)
	if err != nil {
		// don't return error here as extraction was successful
		log.Error(err, "failed to delete tar file after extraction", "path", outputPath)
	}
	log.Info("profile bundle downloaded successfully", "path", outputPath)

	return nil
}

func extractTarFile(tarPath, destDir string, log logr.Logger) error {
	tarFile, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer tarFile.Close()

	err = os.MkdirAll(destDir, 0755)
	if err != nil {
		return err
	}
	var reader io.Reader = tarFile
	tarFile.Seek(0, 0)
	header := make([]byte, 3)
	n, err := tarFile.Read(header)
	if err != nil && err != io.EOF {
		return err
	}

	tarFile.Seek(0, 0)
	if n >= 2 && header[0] == 0x1f && header[1] == 0x8b {
		gzipReader, err := gzip.NewReader(tarFile)
		if err != nil {
			return err
		}
		defer gzipReader.Close()
		reader = gzipReader
	} else {
		log.V(7).Info("detected uncompressed tar file")
	}

	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		targetPath := filepath.Join(destDir, header.Name)
		if !strings.HasPrefix(targetPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path in tar: %s", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(targetPath, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
		case tar.TypeReg:
			parentDir := filepath.Dir(targetPath)
			err = os.MkdirAll(parentDir, 0755)
			if err != nil {
				return err
			}
			outFile, err := os.Create(targetPath)
			if err != nil {
				return err
			}
			_, err = io.Copy(outFile, tarReader)
			outFile.Close()
			if err != nil {
				return err
			}
			err = os.Chmod(targetPath, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
		}
	}
	log.V(7).Info("bundle file extracted successfully", "path", destDir)
	return nil
}

func deleteTarFile(tarPath string) error {
	err := os.Remove(tarPath)
	if err != nil {
		return err
	}
	return nil
}
