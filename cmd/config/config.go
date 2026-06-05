package config

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/profile"
	hubapi "github.com/konveyor/tackle2-hub/shared/api"
	hubfilter "github.com/konveyor/tackle2-hub/shared/binding/filter"
	"github.com/spf13/cobra"
)

type configCommand struct {
	logLevel  *uint32
	log       logr.Logger
	hubClient *hubClient
	insecure  bool
}

type syncCommand struct {
	url             string
	branch          string
	binary          string
	applicationPath string
	profilePath     string
	logLevel        *uint32
	log             logr.Logger
	hubClient       *hubClient
	insecure        bool
	host            string
}

type listCommand struct {
	profileDir string
	logLevel   *uint32
	log        logr.Logger
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
			if val, err := cmd.Flags().GetUint32("log-level"); err == nil {
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
	profilesDir := filepath.Join(l.profileDir, profile.Profiles)
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
			if val, err := cmd.Flags().GetUint32("log-level"); err == nil {
				syncCmd.logLevel = &val
			}
			if insecure, err := cmd.Parent().PersistentFlags().GetBool("insecure"); err == nil {
				syncCmd.insecure = insecure
			}
			application, err := syncCmd.getApplicationFromHub()
			if err != nil {
				log.Error(err, "failed to get application from Hub")
				return err
			}
			profiles, err := syncCmd.getProfilesFromHubApplication(int(application.ID))
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

	syncCommand.Flags().StringVar(&syncCmd.url, "url", "", "url of the remote application repository (use with repository-based applications). use url:branch to specify a branch")
	syncCommand.Flags().StringVar(&syncCmd.binary, "binary", "", "identifier of the application binary in the Hub")
	syncCommand.Flags().StringVar(&syncCmd.applicationPath, "application-path", "", "directory where Hub profiles are downloaded (required when using --url). Default is the current directory")
	syncCommand.Flags().StringVar(&syncCmd.profilePath, "profile-path", "", "directory where Hub profiles are downloaded (required when using --binary)")
	syncCommand.Flags().StringVar(&syncCmd.host, "host", "", "Hub base URL for unauthenticated instances (skips stored PAT login)")

	return syncCommand
}

func (s *syncCommand) Validate(ctx context.Context) error {
	if s.binary == "" && s.url == "" {
		return fmt.Errorf("either --url (repository-based application) or --binary (binary application) must be set")
	}
	if s.binary != "" && s.url != "" {
		return fmt.Errorf("cannot set both --url and --binary; use one for repository-based or binary application lookup")
	}

	if s.binary != "" {
		if s.profilePath == "" {
			return fmt.Errorf("--profile-path is required when syncing a binary application")
		}
	} else {
		s.url, s.branch = parseURLWithBranch(s.url)
		if s.applicationPath == "" {
			defaultPath, err := s.setDefaultApplicationPath()
			if err != nil {
				return err
			}
			s.applicationPath = defaultPath
		}
	}

	// Validate the directory used for profile download (application-path for --url, profile-path for --binary).
	downloadDir := s.applicationPath
	if s.binary != "" {
		downloadDir = s.profilePath
	}
	stat, err := os.Stat(downloadDir)
	if err != nil {
		if s.binary != "" && os.IsNotExist(err) {
			if err := os.MkdirAll(downloadDir, 0755); err != nil {
				return err
			}
			stat, err = os.Stat(downloadDir)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}
	if !stat.IsDir() {
		return fmt.Errorf("path %s is not a directory", downloadDir)
	}

	return nil
}

func (s *syncCommand) setDefaultApplicationPath() (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return currentDir, nil
}

func (s *syncCommand) getHubClient() (*hubClient, error) {
	if s.hubClient != nil {
		return s.hubClient, nil
	}

	if auth, err := loadStoredAuth(); err == nil {
		if s.host == "" || hubHostsEqual(auth.Host, s.host) {
			s.hubClient, err = newHubClient(auth.Host, auth.Token, s.insecure)
			return s.hubClient, err
		}
	}

	var err error
	if s.host != "" {
		s.hubClient, err = newHubClientNoAuth(s.host, s.insecure)
	} else {
		s.hubClient, err = newHubClientFromAuth(s.insecure)
	}
	if err != nil {
		return nil, err
	}
	return s.hubClient, nil
}

func (s *syncCommand) checkAuthentication() error {
	if auth, err := loadStoredAuth(); err == nil {
		if s.host == "" || hubHostsEqual(auth.Host, s.host) {
			return nil
		}
	}
	if s.host != "" {
		return nil
	}
	if _, err := loadStoredAuth(); err != nil {
		return fmt.Errorf("Hub authentication required for sync: %w", err)
	}
	return nil
}

func (s *syncCommand) getApplicationFromHub() (hubapi.Application, error) {
	if err := s.checkAuthentication(); err != nil {
		return hubapi.Application{}, err
	}
	hc, err := s.getHubClient()
	if err != nil {
		return hubapi.Application{}, err
	}

	var apps []hubapi.Application
	if s.binary != "" {
		apps, err = hc.listApplications(hubfilter.Filter{})
		if err != nil {
			return hubapi.Application{}, err
		}
		apps = filterApplicationsByBinary(apps, s.binary)
	} else {
		apps, err = hc.findApplicationsByRepositoryURL(s.url)
		if err != nil {
			return hubapi.Application{}, err
		}
	}

	lookup := s.url
	if s.binary != "" {
		lookup = s.binary
	}
	application, err := selectSingleApplication(apps, lookup)
	if err != nil {
		return hubapi.Application{}, err
	}

	if s.binary == "" {
		if application.Repository == nil || !repositoryURLsEquivalent(application.Repository.URL, s.url) {
			gotURL := ""
			if application.Repository != nil {
				gotURL = application.Repository.URL
			}
			return hubapi.Application{}, fmt.Errorf("URL mismatch: expected %s, got %s", s.url, gotURL)
		}
		if s.branch != "" && application.Repository.Branch != "" && application.Repository.Branch != s.branch {
			return hubapi.Application{}, fmt.Errorf("branch mismatch: expected %s, got %s", s.branch, application.Repository.Branch)
		}
	} else if application.Binary != s.binary {
		return hubapi.Application{}, fmt.Errorf("binary mismatch: expected %s, got %s", s.binary, application.Binary)
	}
	return application, nil
}

func parseURLWithBranch(input string) (url, branch string) {
	schemeEnd := strings.Index(input, "://")
	searchStart := 0
	if schemeEnd != -1 {
		searchStart = schemeEnd + 3
	}
	remainder := input[searchStart:]
	if idx := strings.LastIndex(remainder, ":"); idx != -1 {
		return input[:searchStart+idx], remainder[idx+1:]
	}
	return input, ""
}

func (s *syncCommand) getProfilesFromHubApplication(appID int) ([]hubapi.AnalysisProfile, error) {
	hc, err := s.getHubClient()
	if err != nil {
		return nil, err
	}
	return hc.listApplicationProfiles(uint(appID))
}

func (s *syncCommand) downloadProfileBundle(profileID int) error {
	hc, err := s.getHubClient()
	if err != nil {
		return err
	}
	downloadDir := s.applicationPath
	if s.binary != "" {
		downloadDir = s.profilePath
	}
	filename := fmt.Sprintf("profile-%d.tar", profileID)
	downloadPath := filepath.Join(downloadDir, profile.Profiles, filename)
	if err := os.MkdirAll(filepath.Join(downloadDir, profile.Profiles), 0755); err != nil {
		return err
	}
	return hc.downloadProfileBundle(uint(profileID), downloadPath, s.log)
}

func extractTarFile(tarPath, destDir string, log logr.Logger) error {
	tarFile, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer tarFile.Close()

	// Replace previous extract so files removed from the bundle on the Hub
	// are not left behind on disk
	if err := os.RemoveAll(destDir); err != nil {
		return fmt.Errorf("remove existing extract directory: %w", err)
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
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
