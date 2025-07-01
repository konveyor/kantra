package cloud_foundry

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

var _ = Describe("Discover Manifest", func() {
	var (
		outContent bytes.Buffer

		contentWriter *bufio.Writer = bufio.NewWriter(&outContent)
		tempDir       string
		manifestPath  string
		// useLive bool
	)

	BeforeEach(func() {
		tempDir, err := os.MkdirTemp("", "cloud_foundry_test")
		Expect(err).NotTo(HaveOccurred())
		manifestPath = filepath.Join(tempDir, "manifest.yaml")
		// Reset buffers before each test
		contentWriter.Reset(&outContent)
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	DescribeTable("Manifest validation",
		func(manifestContent string, expectedErrorMessage ...string) {
			err := helperCreateTestManifest(manifestPath, manifestContent, 0644)
			input = manifestPath
			Expect(err).ToNot(HaveOccurred(), "Unable to create manifest.yaml")
			err = discoverManifest(contentWriter)
			contentWriter.Flush()
			if len(expectedErrorMessage) > 0 {
				for _, expected := range expectedErrorMessage {
					if expected == "" {
						Expect(err).ToNot(HaveOccurred(), "Expected no error for invalid manifest, but got one")
						continue // Skip empty strings
					}

					Expect(err.Error()).To(ContainSubstring(expected),
						"Expected error message to contain: "+expected)
				}
			} else {
				Expect(err).ToNot(HaveOccurred(), "Expected no error for invalid manifest, but got one")
			}
		},
		Entry("with an empty manifest", "", "no app name found in manifest file"),
		Entry("with invalid YAML content", "invalid content", "failed to unmarshal YAML from"),
		Entry("with a valid manifest", `name: test-app`, nil),
	)
})

var _ = Describe("Discover command", func() {
	var (
		log          logr.Logger
		out          bytes.Buffer
		err          bytes.Buffer
		tempDir      string
		cmd          *cobra.Command
		outputPath   string
		manifestPath string
	)

	BeforeEach(func() {
		log = logr.Discard()
		out.Reset()
		err.Reset()

		tempDir = createTempDir()
		manifestPath = createTestManifest(tempDir)
		outputPath = createOutputDir(tempDir)
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	Context("when input flag is provided", func() {
		It("should discover manifest and print to stdout by default", func() {
			_, cmd = NewDiscoverCloudFoundryCommand(log)
			cmd.SetOut(&out)
			cmd.SetErr(&err)
			cmd.SetArgs([]string{"--input", manifestPath})

			executeErr := cmd.Execute()

			Expect(executeErr).To(Succeed())
			Expect(out.String()).To(ContainSubstring("test-app"))
			Expect(err.String()).To(BeEmpty())
		})

		It("should produce correct YAML output format", func() {
			_, cmd = NewDiscoverCloudFoundryCommand(log)
			cmd.SetOut(&out)
			cmd.SetErr(&err)
			cmd.SetArgs([]string{"--input", manifestPath})

			executeErr := cmd.Execute()

			Expect(executeErr).To(Succeed())
			expectedOutput := getExpectedYAMLOutput()
			Expect(strings.TrimSpace(out.String())).To(Equal(strings.TrimSpace("--- Content Section ---\n" + expectedOutput)))
		})
	})

	Context("when output-folder flag is provided", func() {
		It("should write output to file instead of stdout", func() {
			_, cmd = NewDiscoverCloudFoundryCommand(log)
			cmd.SetOut(&out)
			cmd.SetErr(&err)
			cmd.SetArgs([]string{"--input", manifestPath, "--output-folder", outputPath})

			executeErr := cmd.Execute()

			Expect(executeErr).To(Succeed())
			Expect(out.String()).To(BeEmpty()) // No stdout output

			// Verify file was created
			files := getOutputFiles(outputPath)
			Expect(files).To(HaveLen(1))

			// Verify file content
			content := readOutputFile(outputPath, files[0].Name())
			expectedOutput := getExpectedYAMLOutput()
			Expect(strings.TrimSpace(content)).To(Equal(strings.TrimSpace(expectedOutput)))
		})
	})

	Context("when processing manifests with secrets", func() {
		var manifestWithSecretsPath string

		BeforeEach(func() {
			manifestWithSecretsPath = createManifestWithSecrets(tempDir)
		})
		AfterEach(func() {
			os.RemoveAll(outputPath)
		})
		It("should output secrets to stdout along with manifest content", func() {
			_, cmd = NewDiscoverCloudFoundryCommand(log)
			cmd.SetOut(&out)
			cmd.SetErr(&err)
			cmd.SetArgs([]string{"--input", manifestWithSecretsPath})

			executeErr := cmd.Execute()

			Expect(executeErr).To(Succeed())
			output := out.String()

			// Verify content section is present
			Expect(output).To(ContainSubstring("--- Content Section ---"))
			Expect(output).To(ContainSubstring("test-app-with-secrets"))

			// Verify secrets section is present
			Expect(output).To(ContainSubstring("--- Secrets Section ---"))
			Expect(output).To(ContainSubstring("docker-registry-user"))
		})

		It("should write secrets to separate file when output-folder is specified", func() {
			_, cmd = NewDiscoverCloudFoundryCommand(log)
			cmd.SetOut(&out)
			cmd.SetErr(&err)
			cmd.SetArgs([]string{"--input", manifestWithSecretsPath, "--output-folder", outputPath})

			executeErr := cmd.Execute()

			Expect(executeErr).To(Succeed())
			Expect(out.String()).To(BeEmpty()) // No stdout output

			// Verify both files were created
			files := getOutputFiles(outputPath)
			Expect(files).To(HaveLen(2))

			// Find manifest and secrets files
			var manifestFile, secretsFile string
			for _, file := range files {
				if strings.Contains(file.Name(), "discover_manifest") {
					manifestFile = file.Name()
				} else if strings.Contains(file.Name(), "secrets") {
					secretsFile = file.Name()
				}
			}

			Expect(manifestFile).NotTo(BeEmpty())
			Expect(secretsFile).NotTo(BeEmpty())

			// Verify manifest content
			manifestContent := readOutputFile(outputPath, manifestFile)
			Expect(manifestContent).ToNot(BeEmpty())
			Expect(manifestContent).To(ContainSubstring("test-app-with-secrets"))

			// Verify secrets content
			secretsContent := readOutputFile(outputPath, secretsFile)
			Expect(secretsContent).ToNot(BeEmpty())
			Expect(secretsContent).To(ContainSubstring("docker-registry-user"))
		})

		It("should handle manifests with no secrets gracefully", func() {
			_, cmd = NewDiscoverCloudFoundryCommand(log)
			cmd.SetOut(&out)
			cmd.SetErr(&err)
			cmd.SetArgs([]string{"--input", manifestPath, "--output-folder", outputPath})

			executeErr := cmd.Execute()

			Expect(executeErr).To(Succeed())

			// Verify only manifest file was created (no secrets file)
			files := getOutputFiles(outputPath)
			Expect(files).To(HaveLen(1))

			// Verify it's the manifest file
			fileName := files[0].Name()
			Expect(fileName).To(ContainSubstring("discover_manifest"))
			Expect(fileName).NotTo(ContainSubstring("secrets"))
		})

		It("should not output secrets section to stdout when manifest has no secrets", func() {
			_, cmd = NewDiscoverCloudFoundryCommand(log)
			cmd.SetOut(&out)
			cmd.SetErr(&err)
			cmd.SetArgs([]string{"--input", manifestPath})

			executeErr := cmd.Execute()

			Expect(executeErr).To(Succeed())
			output := out.String()

			// Verify content section is present
			Expect(output).To(ContainSubstring("--- Content Section ---"))
			Expect(output).To(ContainSubstring("test-app"))

			// Verify secrets section is NOT present
			Expect(output).NotTo(ContainSubstring("--- Secrets Section ---"))
		})
	})

	Context("when required flags are missing", func() {
		It("should return error when input flag is not provided", func() {
			_, cmd = NewDiscoverCloudFoundryCommand(log)
			cmd.SetOut(&out)
			cmd.SetErr(&err)
			cmd.SetArgs([]string{})

			executeErr := cmd.Execute()

			Expect(executeErr).To(HaveOccurred())
			Expect(err.String()).To(ContainSubstring("Error: input flag is required"))
		})

		It("should return error when use-live-connection is true but no spaces provided", func() {
			_, cmd = NewDiscoverCloudFoundryCommand(log)
			cmd.SetOut(&out)
			cmd.SetErr(&err)
			cmd.SetArgs([]string{"--use-live-connection"})

			executeErr := cmd.Execute()

			Expect(executeErr).To(HaveOccurred())
			Expect(executeErr.Error()).To(ContainSubstring("at least one space is required"))
		})
	})

	Context("when using live discovery", func() {
		It("should validate that spaces are provided", func() {
			_, cmd = NewDiscoverCloudFoundryCommand(log)
			cmd.SetOut(&out)
			cmd.SetErr(&err)
			cmd.SetArgs([]string{"--use-live-connection", "--spaces", "test-space"})

			executeErr := cmd.Execute()

			// This will fail because we don't have a real CF environment, but validation should pass
			Expect(executeErr).To(HaveOccurred())
			// The error should NOT be about missing spaces, but about CF connection
			Expect(executeErr.Error()).NotTo(ContainSubstring("at least one space is required"))
		})

		It("should validate cf-config path when provided", func() {
			nonExistentPath := "/nonexistent/cf/config"
			_, cmd = NewDiscoverCloudFoundryCommand(log)
			cmd.SetOut(&out)
			cmd.SetErr(&err)
			cmd.SetArgs([]string{"--use-live-connection", "--spaces", "test-space", "--cf-config", nonExistentPath})

			executeErr := cmd.Execute()

			Expect(executeErr).To(HaveOccurred())
			Expect(executeErr.Error()).To(ContainSubstring("failed to retrieve Cloud Foundry configuration file"))
		})

		It("should accept valid cf-config path", func() {
			validConfigPath := createValidCfConfig(tempDir)
			_, cmd = NewDiscoverCloudFoundryCommand(log)
			cmd.SetOut(&out)
			cmd.SetErr(&err)
			cmd.SetArgs([]string{"--use-live-connection", "--spaces", "test-space", "--cf-config", validConfigPath})

			executeErr := cmd.Execute()

			// This will fail because we don't have a real CF environment, but config path validation should pass
			Expect(executeErr).To(HaveOccurred())
			Expect(executeErr.Error()).NotTo(ContainSubstring("failed to retrieve Cloud Foundry configuration file"))
		})

		It("should support multiple spaces", func() {
			_, cmd = NewDiscoverCloudFoundryCommand(log)
			cmd.SetOut(&out)
			cmd.SetErr(&err)
			cmd.SetArgs([]string{"--use-live-connection", "--spaces", "space1,space2,space3"})

			executeErr := cmd.Execute()

			// This will fail because we don't have a real CF environment, but validation should pass
			Expect(executeErr).To(HaveOccurred())
			Expect(executeErr.Error()).NotTo(ContainSubstring("at least one space is required"))
		})

		It("should support app-name filter", func() {
			_, cmd = NewDiscoverCloudFoundryCommand(log)
			cmd.SetOut(&out)
			cmd.SetErr(&err)
			cmd.SetArgs([]string{"--use-live-connection", "--spaces", "test-space", "--app-name", "my-app"})

			executeErr := cmd.Execute()

			// This will fail because we don't have a real CF environment, but validation should pass
			Expect(executeErr).To(HaveOccurred())
			Expect(executeErr.Error()).NotTo(ContainSubstring("at least one space is required"))
		})

		It("should support skip-ssl-validation flag", func() {
			_, cmd = NewDiscoverCloudFoundryCommand(log)
			cmd.SetOut(&out)
			cmd.SetErr(&err)
			cmd.SetArgs([]string{"--use-live-connection", "--spaces", "test-space", "--skip-ssl-validation"})

			executeErr := cmd.Execute()

			// This will fail because we don't have a real CF environment, but validation should pass
			Expect(executeErr).To(HaveOccurred())
			Expect(executeErr.Error()).NotTo(ContainSubstring("at least one space is required"))
		})

		It("should support custom platform type", func() {
			_, cmd = NewDiscoverCloudFoundryCommand(log)
			cmd.SetOut(&out)
			cmd.SetErr(&err)
			cmd.SetArgs([]string{"--use-live-connection", "--spaces", "test-space", "--platformType", "cloud-foundry"})

			executeErr := cmd.Execute()

			// This will fail because we don't have a real CF environment, but validation should pass
			Expect(executeErr).To(HaveOccurred())
			Expect(executeErr.Error()).NotTo(ContainSubstring("at least one space is required"))
		})
	})

	Context("when flag combinations are invalid", func() {
		It("should reject mutually exclusive flags: use-live-connection and input", func() {
			_, cmd = NewDiscoverCloudFoundryCommand(log)
			cmd.SetOut(&out)
			cmd.SetErr(&err)
			cmd.SetArgs([]string{"--use-live-connection", "--spaces", "test-space",
				"--cf-config", "../../../../test-data/asset_generation/discover", "--input", manifestPath})

			executeErr := cmd.Execute()

			Expect(executeErr).To(HaveOccurred())
			Expect(executeErr.Error()).To(ContainSubstring("[input use-live-connection] were all set"))
		})
	})

	Context("when input file does not exist", func() {
		It("should return error for nonexistent file", func() {
			_, cmd = NewDiscoverCloudFoundryCommand(log)
			cmd.SetOut(&out)
			cmd.SetErr(&err)
			cmd.SetArgs([]string{"--input", "nonexistent.yaml"})

			executeErr := cmd.Execute()

			Expect(executeErr).To(HaveOccurred())
			Expect(executeErr.Error()).To(ContainSubstring("no such file or directory"))
		})
	})

	Context("when processing directories", func() {
		It("should process all manifest files in a directory", func() {
			// Create multiple manifest files in a directory
			manifestDir := filepath.Join(tempDir, "manifests")
			Expect(os.Mkdir(manifestDir, 0755)).To(Succeed())

			manifest1 := filepath.Join(manifestDir, "app1.yaml")
			manifest2 := filepath.Join(manifestDir, "app2.yaml")

			Expect(os.WriteFile(manifest1, []byte("name: app1\nmemory: 256M"), 0644)).To(Succeed())
			Expect(os.WriteFile(manifest2, []byte("name: app2\nmemory: 512M"), 0644)).To(Succeed())

			_, cmd = NewDiscoverCloudFoundryCommand(log)
			cmd.SetOut(&out)
			cmd.SetErr(&err)
			cmd.SetArgs([]string{"--input", manifestDir})

			executeErr := cmd.Execute()

			Expect(executeErr).To(Succeed())
			Expect(out.String()).To(ContainSubstring("app1"))
			Expect(out.String()).To(ContainSubstring("app2"))
		})
	})
})

// Helper functions to reduce duplication and improve readability

func createTempDir() string {
	tempDir, err := os.MkdirTemp("", "cloud_foundry_test")
	Expect(err).NotTo(HaveOccurred())
	return tempDir
}

func createTestManifest(tempDir string) string {
	manifestContent := []byte(`---
name: test-app
memory: 256M
instances: 1
`)
	manifestPath := filepath.Join(tempDir, "manifest.yaml")
	Expect(os.WriteFile(manifestPath, manifestContent, 0644)).To(Succeed())
	return manifestPath
}

func createOutputDir(tempDir string) string {
	outputPath := tempDir + "/output"
	Expect(os.Mkdir(outputPath, os.ModePerm)).NotTo(HaveOccurred())
	return outputPath
}

func createValidCfConfig(tempDir string) string {
	configContent := []byte(`{
  "AccessToken": "bearer token",
  "APIVersion": "3.0.0",
  "AuthorizationEndpoint": "https://example.com/oauth/authorize",
  "DopplerEndpoint": "ws://example.com:443",
  "LogCacheEndpoint": "https://example.com",
  "NetworkPolicyV1Endpoint": "https://example.com/networking",
  "RefreshToken": "refresh-token",
  "RoutingEndpoint": "https://example.com/routing",
  "SkipSSLValidation": false,
  "Target": "https://api.example.com",
  "TokenEndpoint": "https://example.com/oauth/token",
  "UAAEndpoint": "https://example.com"
}`)
	configPath := filepath.Join(tempDir, "cf-config.json")
	Expect(os.WriteFile(configPath, configContent, 0644)).To(Succeed())
	return configPath
}

func createManifestWithSecrets(tempDir string) string {
	manifestContent := []byte(`---
name: test-app-with-secrets
memory: 512M
instances: 2
env:
  DATABASE_URL: postgresql://user:password@localhost/db
  API_KEY: secret-api-key-value
services:
  - database-service
docker:
    image: myregistry/myapp:latest
    username: docker-registry-user
`)
	manifestPath := filepath.Join(tempDir, "manifest-with-secrets.yml")
	Expect(os.WriteFile(manifestPath, manifestContent, 0644)).To(Succeed())
	return manifestPath
}

func getOutputFiles(outputPath string) []os.DirEntry {
	files, err := os.ReadDir(outputPath)
	Expect(err).NotTo(HaveOccurred())
	return files
}

func readOutputFile(outputPath, filename string) string {
	filePath := filepath.Join(outputPath, filename)
	content, err := os.ReadFile(filePath)
	Expect(err).NotTo(HaveOccurred())
	return string(content)
}

func getExpectedYAMLOutput() string {
	return `BuildPacks: null
Command: ""
DiskQuota: ""
Docker:
  Image: ""
  Username: ""
Env: null
HealthCheck:
  Endpoint: /
  Interval: 30
  Timeout: 1
  Type: port
Instances: 1
Lifecycle: ""
LogRateLimit: ""
Memory: 256M
Metadata:
  Annotations: null
  Labels: null
  Name: test-app
  Space: ""
  Version: ""
Processes: null
ReadinessCheck:
  Endpoint: /
  Interval: 30
  Timeout: 1
  Type: process
Routes:
  NoRoute: false
  RandomRoute: false
  Routes: null
Services: null
Sidecars: null
Stack: ""
Timeout: 60`
}

func helperCreateTestManifest(manifestPath string, content string, perm os.FileMode) error {
	err := os.WriteFile(manifestPath, []byte(content), perm) //0644
	if err != nil {
		return err
	}
	return nil
}
