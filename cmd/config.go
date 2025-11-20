package cmd

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/tls"
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
	listProfiles string
	logLevel     *uint32
	log          logr.Logger
	hubClient    *hubClient
	insecure     bool
}

type syncCommand struct {
	url             string
	applicationPath string
	logLevel        *uint32
	log             logr.Logger
	hubClient       *hubClient
	insecure        bool
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
				err := configCmd.ListProfiles()
				if err != nil {
					return err
				}
				return nil
			}

			return nil
		},
	}

	configCommand.Flags().StringVar(&configCmd.listProfiles, "list-profiles", "", "list local Hub profiles in the application")
	configCommand.PersistentFlags().BoolVarP(&configCmd.insecure, "insecure", "k", false, "Skip TLS certificate verification")

	configCommand.AddCommand(NewSyncCmd(log))
	configCommand.AddCommand(NewLoginCmd(log))

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
		profilesDir := filepath.Join(c.listProfiles, Profiles)
		if _, err := os.Stat(profilesDir); os.IsNotExist(err) {
			return err
		}
		return nil
	}

	return nil
}

func (c *configCommand) ListProfiles() error {
	if c.listProfiles == "" {
		c.log.Info("application path not provided")
		return nil
	}
	profilesDir := filepath.Join(c.listProfiles, Profiles)
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
		c.log.Info("no profile directories found in: %s", profilesDir)
		return nil
	}
	fmt.Fprintln(os.Stdout, "profiles found in", c.listProfiles)
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
		return fmt.Errorf("%w failed to stat application path %s", err, s.applicationPath)
	}
	if !stat.IsDir() {
		return fmt.Errorf("application path %s is not a directory", s.applicationPath)
	}

	return nil
}

func (s *syncCommand) getHubClient() *hubClient {
	if s.hubClient == nil {
		s.hubClient = newHubClientWithOptions(s.insecure)
	}
	return s.hubClient
}

type hubClient struct {
	client   *http.Client
	host     string
	insecure bool
}

func newHubClientWithOptions(insecure bool) *hubClient {
	host := ""
	if foundHost := os.Getenv("HOST"); foundHost != "" {
		host = foundHost
	} else {
		if storedAuth, err := LoadStoredTokens(); err == nil && storedAuth.Host != "" {
			host = storedAuth.Host
		}
	}
	host = strings.TrimSuffix(host, "/")
	fmt.Printf("Hub client connecting to: %s\n", host)
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
	}
}

func (hc *hubClient) doRequest(path, acceptHeader string, log logr.Logger) (*http.Response, error) {
	return hc.doRequestWithRetry(path, acceptHeader, log, false)
}

func (hc *hubClient) doRequestWithRetry(path, acceptHeader string, log logr.Logger, isRetry bool) (*http.Response, error) {
	url := hc.host + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", acceptHeader)
	storedAuth, _ := LoadStoredTokens()

	token := os.Getenv("TOKEN")
	if token == "" && storedAuth != nil {
		token = storedAuth.Token
	}
	if token != "" {
		req.Header.Set("Authentication", "Bearer "+token)
	}
	resp, err := hc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to %s: %w", url, err)
	}
	if resp.StatusCode == http.StatusUnauthorized && !isRetry && storedAuth != nil && storedAuth.RefreshToken != "" {
		resp.Body.Close()

		log.V(7).Info("token expired, attempting automatic refresh")
		if err := hc.refreshStoredToken(storedAuth, log); err != nil {
			return nil, fmt.Errorf("authentication failed and token refresh failed: %w", err)
		}

		log.V(7).Info("token refreshed, retrying request")
		return hc.doRequestWithRetry(path, acceptHeader, log, true)
	}

	return resp, nil
}

func (hc *hubClient) readResponseBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (s *syncCommand) getApplicationFromHub(urlRepo string) (api.Application, error) {
	hubClient := s.getHubClient()
	path := fmt.Sprintf(`/applications?filter=repository.url='%s'`, urlRepo)
	resp, err := hubClient.doRequest(path, "application/x-yaml", s.log)
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
	var application api.Application
	for _, app := range apps {
		fmt.Println(app.Repository.URL)

		if app.Repository.URL == s.url {
			application = app
		}
	}

	return application, nil
}

func parseApplicationsFromHub(yamlData string) ([]api.Application, error) {
	var apps []api.Application
	err := yaml.Unmarshal([]byte(yamlData), &apps)
	if err != nil {
		return nil, err
	}
	return apps, nil
}

func (s *syncCommand) getProfilesFromHubApplicaton(appID int) ([]api.AnalysisProfile, error) {
	hubClient := s.getHubClient()
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
	hubClient := s.getHubClient()
	filename := fmt.Sprintf("profile-%d.tar", profileID)
	downloadPath := filepath.Join(s.applicationPath, Profiles, filename)
	err := os.MkdirAll(filepath.Join(s.applicationPath, Profiles), 0755)
	if err != nil {
		return fmt.Errorf("failed to create profiles directory %s: %w", filepath.Join(s.applicationPath, Profiles), err)
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
		return fmt.Errorf("failed to create output file %s: %w", outputPath, err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write to output file: %w", err)
	}
	log.V(7).Info("compressed bundle file downloaded successfully", "path", outputPath)

	extractDir := strings.TrimSuffix(outputPath, ".tar")
	err = extractTarFile(outputPath, extractDir, log)
	if err != nil {
		return fmt.Errorf("failed to extract tar file: %w", err)
	}
	err = deleteTarFile(outputPath)
	if err != nil {
		// don't return error here as extraction was successful
		log.Error(err, "failed to delete tar file after extraction", "path", outputPath)
	}

	return nil
}

func extractTarFile(tarPath, destDir string, log logr.Logger) error {
	tarFile, err := os.Open(tarPath)
	if err != nil {
		return fmt.Errorf("failed to open tar file %s: %w", tarPath, err)
	}
	defer tarFile.Close()

	err = os.MkdirAll(destDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create destination directory %s: %w", destDir, err)
	}

	var reader io.Reader = tarFile
	tarFile.Seek(0, 0)
	header := make([]byte, 3)
	n, err := tarFile.Read(header)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read file header: %w", err)
	}

	tarFile.Seek(0, 0)
	if n >= 2 && header[0] == 0x1f && header[1] == 0x8b {
		gzipReader, err := gzip.NewReader(tarFile)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
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
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		targetPath := filepath.Join(destDir, header.Name)
		if !strings.HasPrefix(targetPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path in tar: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(targetPath, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create directory %s: %w", targetPath, err)
			}

		case tar.TypeReg:
			parentDir := filepath.Dir(targetPath)
			err = os.MkdirAll(parentDir, 0755)
			if err != nil {
				return fmt.Errorf("failed to create parent directory %s: %w", parentDir, err)
			}

			outFile, err := os.Create(targetPath)
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", targetPath, err)
			}

			_, err = io.Copy(outFile, tarReader)
			outFile.Close()
			if err != nil {
				return fmt.Errorf("failed to write file %s: %w", targetPath, err)
			}

			err = os.Chmod(targetPath, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to set permissions for %s: %w", targetPath, err)
			}
		}
	}
	log.V(7).Info("bundle file extracted successfully", "path", destDir)
	return nil
}

func deleteTarFile(tarPath string) error {
	err := os.Remove(tarPath)
	if err != nil {
		return fmt.Errorf("failed to delete tar file %s: %w", tarPath, err)
	}
	return nil
}
