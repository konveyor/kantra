package cloud_foundry

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	cfProvider "github.com/konveyor/asset-generation/pkg/providers/discoverers/cloud_foundry"
	pTypes "github.com/konveyor/asset-generation/pkg/providers/types/provider"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

var _ = Describe("Discover Manifest", func() {
	var (
		outContent    bytes.Buffer
		contentWriter *bufio.Writer
		tempDir       string
		manifestPath  string
	)

	BeforeEach(func() {
		tempDir = createTempDir()
		manifestPath = filepath.Join(tempDir, "manifest.yaml")
		outContent.Reset()
		contentWriter = bufio.NewWriter(&outContent)
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	DescribeTable("File-based manifest discovery validation",
		func(manifestContent string, expectedErrorMessage ...string) {
			err := helperCreateTestManifest(manifestPath, manifestContent, 0644)
			Expect(err).ToNot(HaveOccurred(), "Unable to create manifest.yaml")

			// Create command instance and test through CLI
			_, cmd := NewDiscoverCloudFoundryCommand(logr.Discard())
			cmd.SetOut(contentWriter)
			cmd.SetErr(contentWriter)
			cmd.SetArgs([]string{"--input", manifestPath})

			err = cmd.Execute()
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
		Entry("with an empty manifest", "", "no applications found in"),
		Entry("with invalid YAML content", "invalid content", "failed to unmarshal YAML"),
		Entry("with a valid manifest", `name: test-app`, nil),
	)

	DescribeTable("Live discovery manifest validation",
		func(platformType string, orgsArg []string, spacesArg []string, cfConfigPathArg string, expectedErrorMessage ...string) {
			// Create command instance and test through CLI
			_, cmd := NewDiscoverCloudFoundryCommand(logr.Discard())
			cmd.SetOut(contentWriter)
			cmd.SetErr(contentWriter)

			args := []string{"--use-live-connection", "--platformType", platformType}
			if len(orgsArg) > 0 {
				args = append(args, "--orgs", strings.Join(orgsArg, ","))
			}
			if len(spacesArg) > 0 {
				args = append(args, "--spaces", strings.Join(spacesArg, ","))
			}
			if cfConfigPathArg != "" {
				args = append(args, "--cf-config", cfConfigPathArg)
			}
			cmd.SetArgs(args)

			err := cmd.Execute()
			contentWriter.Flush()

			if len(expectedErrorMessage) > 0 {
				for _, expected := range expectedErrorMessage {
					if expected == "" {
						Expect(err).ToNot(HaveOccurred(), "Expected no error, but got one")
						continue // Skip empty strings
					}

					Expect(err.Error()).To(ContainSubstring(expected),
						"Expected error message to contain: "+expected)
				}
			} else {
				Expect(err).ToNot(HaveOccurred(), "Expected no error, but got one")
			}
		},
		Entry("with unsupported platform type", "unsupported-platform", []string{"test-org"}, []string{"test-space"}, "../../../../test-data/asset_generation/discover", "unsupported platform type: unsupported-platform"),
		Entry("with invalid CF config path", "cloud-foundry", []string{"test-org"}, []string{"test-space"}, "/nonexistent/path", "no such file or directory"),
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
		// Reset global variables to prevent cross-test contamination
		outputDir = ""
		appName = ""
		orgs = []string{}
		spaces = []string{}
		input = ""
		useLive = false
		listApps = false
		concealSensitiveData = false
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
		// Clean up global variables to prevent cross-test contamination
		outputDir = ""
		appName = ""
		orgs = []string{}
		spaces = []string{}
		input = ""
		useLive = false
		listApps = false
		concealSensitiveData = false
	})

	Context("flags validation", func() {
		Context("when flag combinations are invalid", func() {
			It("should reject mutually exclusive flags: use-live-connection and input", func() {
				_, cmd = NewDiscoverCloudFoundryCommand(log)
				cmd.SetOut(&out)
				cmd.SetErr(&err)
				cmd.SetArgs([]string{"--use-live-connection", "--orgs", "test-org", "--spaces", "test-space",
					"--cf-config", "../../../../test-data/asset_generation/discover", "--input", manifestPath})

				executeErr := cmd.Execute()

				Expect(executeErr).To(HaveOccurred())
				Expect(executeErr.Error()).To(ContainSubstring("[input use-live-connection] were all set"))
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

			It("should return error when use-live-connection is true for discovery mode but orgs is not provided", func() {
				_, cmd = NewDiscoverCloudFoundryCommand(log)
				cmd.SetOut(&out)
				cmd.SetErr(&err)
				// Without --list-apps, orgs is required for discovery mode
				cmd.SetArgs([]string{"--use-live-connection", "--cf-config", "../../../../test-data/asset_generation/discover"})

				executeErr := cmd.Execute()

				Expect(executeErr).To(HaveOccurred())
				Expect(executeErr.Error()).To(ContainSubstring("--orgs flag is required when using --use-live-connection without --list-apps"))
			})

			It("should return error when --orgs is used without --use-live-connection", func() {
				_, cmd = NewDiscoverCloudFoundryCommand(log)
				cmd.SetOut(&out)
				cmd.SetErr(&err)
				// Using --orgs with --input should fail
				cmd.SetArgs([]string{"--input", manifestPath, "--orgs", "test-org"})

				executeErr := cmd.Execute()

				Expect(executeErr).To(HaveOccurred())
				Expect(executeErr.Error()).To(ContainSubstring("--orgs flag can only be used with --use-live-connection"))
			})
		})
		Context("when output-dir flag is provided", func() {
			It("should write output to file instead of stdout", func() {
				_, cmd = NewDiscoverCloudFoundryCommand(log)
				cmd.SetOut(&out)
				cmd.SetErr(&err)
				cmd.SetArgs([]string{"--input", manifestPath, "--output-dir", outputPath})

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
	})

	Context("when processing manifests with secrets", func() {

		Context("conceal sensitive data flag is enabled", func() {
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
				cmd.SetArgs([]string{"--input", manifestWithSecretsPath, "--conceal-sensitive-data"})

				executeErr := cmd.Execute()

				Expect(executeErr).To(Succeed())
				output := out.String()

				// Verify content section is present
				Expect(output).To(ContainSubstring("--- Content Section ---"))
				Expect(output).To(ContainSubstring("test-app-with-sensitive-data"))

				// Verify secrets section is present
				Expect(output).To(ContainSubstring("--- Secrets Section ---"))
				Expect(output).To(ContainSubstring("docker-registry-user"))
			})

			It("should write secrets to separate file when output-dir is specified", func() {
				_, cmd = NewDiscoverCloudFoundryCommand(log)
				cmd.SetOut(&out)
				cmd.SetErr(&err)
				cmd.SetArgs([]string{"--input", manifestWithSecretsPath, "--output-dir", outputPath, "--conceal-sensitive-data"})

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
				Expect(manifestContent).To(ContainSubstring("test-app-with-sensitive-data"))

				// Verify secrets content
				secretsContent := readOutputFile(outputPath, secretsFile)
				Expect(secretsContent).ToNot(BeEmpty())
				Expect(secretsContent).To(ContainSubstring("docker-registry-user"))
			})

			It("should create only manifest file when no secrets exist", func() {
				_, cmd = NewDiscoverCloudFoundryCommand(log)
				cmd.SetOut(&out)
				cmd.SetErr(&err)
				cmd.SetArgs([]string{"--input", manifestPath, "--output-dir", outputPath, "--conceal-sensitive-data"})

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
				cmd.SetArgs([]string{"--input", manifestPath, "--conceal-sensitive-data"})

				executeErr := cmd.Execute()

				Expect(executeErr).To(Succeed())
				output := out.String()

				// Verify content section is present
				Expect(output).NotTo(ContainSubstring("--- Content Section ---"))
				Expect(output).To(ContainSubstring("test-app"))

				// Verify secrets section is NOT present
				Expect(output).NotTo(ContainSubstring("--- Secrets Section ---"))
			})
		})

		Context("conceal sensitive data flag is disabled", func() {
			var manifestWithSecretsPath string

			BeforeEach(func() {
				manifestWithSecretsPath = createManifestWithSecrets(tempDir)
			})
			AfterEach(func() {
				os.RemoveAll(outputPath)
			})
			It("should not output secrets to stdout along with manifest content", func() {
				_, cmd = NewDiscoverCloudFoundryCommand(log)
				cmd.SetOut(&out)
				cmd.SetErr(&err)
				cmd.SetArgs([]string{"--input", manifestWithSecretsPath})

				executeErr := cmd.Execute()

				Expect(executeErr).To(Succeed())
				output := out.String()

				// Verify content section is present
				Expect(output).NotTo(ContainSubstring("--- Content Section ---"))
				Expect(output).To(ContainSubstring("test-app-with-sensitive-data"))

				// Verify secrets section is present
				Expect(output).NotTo(ContainSubstring("--- Secrets Section ---"))
			})

			It("should not write secrets to separate file when output-dir is specified", func() {
				_, cmd = NewDiscoverCloudFoundryCommand(log)
				cmd.SetOut(&out)
				cmd.SetErr(&err)
				cmd.SetArgs([]string{"--input", manifestWithSecretsPath, "--output-dir", outputPath})

				executeErr := cmd.Execute()

				Expect(executeErr).To(Succeed())
				Expect(out.String()).To(BeEmpty()) // No stdout output

				// Verify both files were created
				files := getOutputFiles(outputPath)
				Expect(files).To(HaveLen(1))

				// Find manifest and secrets files
				var manifestFile string
				for _, file := range files {
					Expect(file.Name()).NotTo(ContainSubstring("secrets"))
					if strings.Contains(file.Name(), "discover_manifest") {
						manifestFile = file.Name()
					}
				}

				Expect(manifestFile).NotTo(BeEmpty())

				// Verify manifest content
				manifestContent := readOutputFile(outputPath, manifestFile)
				Expect(manifestContent).ToNot(BeEmpty())
				Expect(manifestContent).To(ContainSubstring("test-app-with-sensitive-data"))

			})

			It("should not create secrets file when conceal-sensitive-data is disabled", func() {
				_, cmd = NewDiscoverCloudFoundryCommand(log)
				cmd.SetOut(&out)
				cmd.SetErr(&err)
				cmd.SetArgs([]string{"--input", manifestPath, "--output-dir", outputPath})

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
				Expect(output).NotTo(ContainSubstring("--- Content Section ---"))
				Expect(output).To(ContainSubstring("test-app"))

				// Verify secrets section is NOT present
				Expect(output).NotTo(ContainSubstring("--- Secrets Section ---"))
			})
		})
	})

	Context("when list-apps flag is provided", func() {
		Context("when using live discover", func() {
			It("should exclude orgs with no apps from output", func() {
				orgs = []string{"org1", "org2", "org3"}
				mockProv := &mockProvider{
					ListAppsFunc: func() (map[string][]any, error) {
						return map[string][]any{
							"org1": {
								cfProvider.AppReference{OrgName: "org1", SpaceName: "space1", AppName: "app1"},
							},
							// org2 has no apps (empty list), org3 doesn't exist in map
						}, nil
					},
				}

				var out bytes.Buffer
				contentWriter := bufio.NewWriter(&out)

				err := listApplicationsLive(mockProv, contentWriter)
				Expect(err).To(Succeed())

				contentWriter.Flush()
				// Verify only org1's app is printed, org2 and org3 are excluded
				output := out.String()
				Expect(output).To(ContainSubstring("org1"))
				Expect(output).ToNot(ContainSubstring("org2"))
				Expect(output).ToNot(ContainSubstring("org3"))
				Expect(output).To(ContainSubstring("app1"))
			})

			It("should list apps in hierarchical org->space->apps format", func() {
				mockProv := &mockProvider{
					DiscoverFunc: func(raw any) (*pTypes.DiscoverResult, error) {
						return &pTypes.DiscoverResult{}, nil
					},
					ListAppsFunc: func() (map[string][]any, error) {
						// Live discovery returns map keyed by org name
						return map[string][]any{
							"org1": {
								cfProvider.AppReference{OrgName: "org1", SpaceName: "space1", AppName: "app1"},
								cfProvider.AppReference{OrgName: "org1", SpaceName: "space1", AppName: "app2"},
								cfProvider.AppReference{OrgName: "org1", SpaceName: "space2", AppName: "app3"},
							},
							"org2": {
								cfProvider.AppReference{OrgName: "org2", SpaceName: "space1", AppName: "app4"},
							},
						}, nil
					},
				}

				var out bytes.Buffer
				contentWriter := bufio.NewWriter(&out)

				err := listApplicationsLive(mockProv, contentWriter)
				Expect(err).To(Succeed())

				contentWriter.Flush()

				output := out.String()
				// Verify hierarchical output format
				Expect(output).To(ContainSubstring("Organization: org1"))
				Expect(output).To(ContainSubstring("Organization: org2"))
				Expect(output).To(ContainSubstring("  Space: space1"))
				Expect(output).To(ContainSubstring("  Space: space2"))
				Expect(output).To(ContainSubstring("    - app1"))
				Expect(output).To(ContainSubstring("    - app2"))
				Expect(output).To(ContainSubstring("    - app3"))
				Expect(output).To(ContainSubstring("    - app4"))
			})
			It("should display empty organizations without spaces or apps", func() {
				mockProv := &mockProvider{
					ListAppsFunc: func() (map[string][]any, error) {
						// Return org-keyed map with empty app lists
						return map[string][]any{
							"org1": {},
							"org2": {},
						}, nil
					},
				}

				var out bytes.Buffer
				contentWriter := bufio.NewWriter(&out)

				err := listApplicationsLive(mockProv, contentWriter)
				Expect(err).To(Succeed())

				contentWriter.Flush()
				output := out.String()

				// Should print org names but no spaces/apps under them since lists are empty
				Expect(output).To(ContainSubstring("Organization: org1"))
				Expect(output).To(ContainSubstring("Organization: org2"))
				Expect(output).NotTo(ContainSubstring("Space:")) // No spaces since app lists are empty
				Expect(output).NotTo(ContainSubstring("    - ")) // No app entries
			})

			It("should return error if ListApps returns an error", func() {
				mockProv := &mockProvider{
					ListAppsFunc: func() (map[string][]any, error) {
						return nil, fmt.Errorf("some error")
					},
				}

				var out bytes.Buffer
				contentWriter := bufio.NewWriter(&out)

				err := listApplicationsLive(mockProv, contentWriter)
				Expect(err).To(MatchError("failed to list apps by space: some error"))
			})

			It("should handle no spaces returned (empty map)", func() {
				mockProv := &mockProvider{
					ListAppsFunc: func() (map[string][]any, error) {
						return map[string][]any{}, nil
					},
				}

				var out bytes.Buffer
				contentWriter := bufio.NewWriter(&out)

				err := listApplicationsLive(mockProv, contentWriter)
				Expect(err).To(Succeed())

				contentWriter.Flush()
				output := out.String()
				Expect(output).To(BeEmpty())
			})

			It("should return error when printApps fails", func() {
				mockProv := &mockProvider{
					ListAppsFunc: func() (map[string][]any, error) {
						// Return invalid type to trigger printApps error
						return map[string][]any{
							"org1": {
								"invalid-type", // Not an AppReference
							},
						}, nil
					},
				}

				var out bytes.Buffer
				contentWriter := bufio.NewWriter(&out)

				err := listApplicationsLive(mockProv, contentWriter)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unexpected type for app list"))
			})
		})

		Context("when using live discover with optional flags", func() {
			It("should allow --list-apps without --orgs (discovers all orgs)", func() {
				_, cmd := NewDiscoverCloudFoundryCommand(log)
				cmd.SetOut(&out)
				cmd.SetErr(&err)
				// list-apps without orgs should be valid
				cmd.SetArgs([]string{"--use-live-connection", "--list-apps", "--cf-config", "../../../../test-data/asset_generation/discover"})

				executeErr := cmd.Execute()

				// Will fail because no real CF environment, but flag validation should pass
				Expect(executeErr).To(HaveOccurred())
				Expect(executeErr.Error()).NotTo(ContainSubstring("--orgs flag is required"))
			})

			It("should handle --skip-ssl-validation flag", func() {
				_, cmd := NewDiscoverCloudFoundryCommand(log)
				cmd.SetOut(&out)
				cmd.SetErr(&err)
				// Test with skip-ssl-validation flag
				cmd.SetArgs([]string{"--use-live-connection", "--list-apps", "--skip-ssl-validation", "--cf-config", "../../../../test-data/asset_generation/discover"})

				executeErr := cmd.Execute()

				// Will fail because no real CF environment, but flag should be accepted
				Expect(executeErr).To(HaveOccurred())
				// The error won't mention skip-ssl-validation - it'll be about CF connection
			})

			It("should allow discovery mode without --spaces (discovers all spaces for orgs)", func() {
				_, cmd := NewDiscoverCloudFoundryCommand(log)
				cmd.SetOut(&out)
				cmd.SetErr(&err)
				// Discovery mode with orgs but without spaces should be valid
				cmd.SetArgs([]string{"--use-live-connection", "--orgs", "test-org", "--cf-config", "../../../../test-data/asset_generation/discover"})

				executeErr := cmd.Execute()

				// Will fail because no real CF environment, but flag validation should pass
				Expect(executeErr).To(HaveOccurred())
				Expect(executeErr.Error()).NotTo(ContainSubstring("spaces"))
			})
		})

		Context("when using local discover", func() {
			It("should return error when input is not properly set for local listing", func() {
				var out bytes.Buffer
				tempDir := createTempDir()
				defer os.RemoveAll(tempDir)

				createTestManifest(tempDir)

				// runListApps with useLive=false will try to use the global input variable
				// which is not set to tempDir, so it should fail
				err := runListApps(nil, false, &out)
				Expect(err).To(HaveOccurred())
			})

			It("lists a single app from a single manifest", func() {

				var out bytes.Buffer
				contentWriter := bufio.NewWriter(&out)
				tempDir := createTempDir()
				defer os.RemoveAll(tempDir)

				manifestPath = createTestManifest(tempDir)
				err := listApplicationsLocal(tempDir, contentWriter)
				Expect(err).ToNot(HaveOccurred())

				contentWriter.Flush()

				output := out.String()
				Expect(output).To(ContainSubstring("Space: local"))
				Expect(output).To(ContainSubstring("  - test-app"))
			})
			It("lists multiple apps from multiple manifests", func() {

				var out bytes.Buffer
				contentWriter := bufio.NewWriter(&out)
				tempDir := createTempDir()
				defer os.RemoveAll(tempDir)
				createMultipleManifests(tempDir, 3)
				err := listApplicationsLocal(tempDir, contentWriter)
				Expect(err).ToNot(HaveOccurred())

				contentWriter.Flush()

				output := out.String()
				Expect(output).To(ContainSubstring("Space: local"))
				Expect(output).To(ContainSubstring("  - test-app-0"))
				Expect(output).To(ContainSubstring("  - test-app-1"))
				Expect(output).To(ContainSubstring("  - test-app-2"))
			})
		})

		Context("printApps function tests", func() {
			It("should handle apps with empty space names", func() {
				var out bytes.Buffer

				appListByOrg := map[string][]any{
					"org1": {
						cfProvider.AppReference{OrgName: "org1", SpaceName: "", AppName: "app-no-space"},
					},
				}

				err := printApps(appListByOrg, &out)
				Expect(err).To(Succeed())
				Expect(out.String()).To(ContainSubstring("Organization: org1"))
				Expect(out.String()).To(ContainSubstring("Space: unknown"))
				Expect(out.String()).To(ContainSubstring("    - app-no-space"))
			})

			It("should handle multiple apps with some having empty space names", func() {
				var out bytes.Buffer

				appListByOrg := map[string][]any{
					"org1": {
						cfProvider.AppReference{OrgName: "org1", SpaceName: "", AppName: "app1"},
						cfProvider.AppReference{OrgName: "org1", SpaceName: "space1", AppName: "app2"},
					},
				}

				err := printApps(appListByOrg, &out)
				Expect(err).To(Succeed())
				output := out.String()
				Expect(output).To(ContainSubstring("Space: unknown"))
				Expect(output).To(ContainSubstring("Space: space1"))
			})

			It("should return error for invalid app type in printApps", func() {
				var out bytes.Buffer

				appListByOrg := map[string][]any{
					"org1": {
						"invalid-type", // Not an AppReference
					},
				}

				err := printApps(appListByOrg, &out)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unexpected type for app list"))
			})
		})

	})
	Context("when testing runListApps function", func() {
		var mockProv *mockProvider
		var out bytes.Buffer

		BeforeEach(func() {
			out.Reset()
		})

		It("should call listApplicationsLive when useLive is true", func() {
			listAppsCalled := false

			mockProv = &mockProvider{
				ListAppsFunc: func() (map[string][]any, error) {
					listAppsCalled = true
					return map[string][]any{
						"org1": {
							cfProvider.AppReference{OrgName: "org1", SpaceName: "space1", AppName: "app1"},
						},
					}, nil
				},
			}

			err := runListApps(mockProv, true, &out)
			Expect(err).To(Succeed())
			Expect(listAppsCalled).To(BeTrue())
		})

		It("should call listApplicationsLocal when useLive is false and output apps", func() {
			input = createTempDir()
			defer os.RemoveAll(input)
			createTestManifest(input)

			err := runListApps(nil, false, &out)
			Expect(err).To(Succeed())

			// Verify that the app was actually listed
			output := out.String()
			Expect(output).To(ContainSubstring("test-app"))
			Expect(output).To(ContainSubstring("Organization:"))
		})

		It("should return error when listApplicationsLocal fails", func() {
			input = "/nonexistent/path"

			err := runListApps(nil, false, &out)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when testing listApplicationsLocal function", func() {
		It("should return error when input path does not exist", func() {
			var out bytes.Buffer
			contentWriter := bufio.NewWriter(&out)

			err := listApplicationsLocal("/nonexistent/path", contentWriter)
			Expect(err).To(HaveOccurred())
		})

		It("should return error when createProviderForManifest fails due to unreadable file", func() {
			var out bytes.Buffer
			contentWriter := bufio.NewWriter(&out)
			tempDir := createTempDir()
			defer os.RemoveAll(tempDir)

			// Create a file with no read permissions to cause createProviderForManifest to fail
			unreadableFile := filepath.Join(tempDir, "unreadable.yaml")
			Expect(os.WriteFile(unreadableFile, []byte("name: test"), 0000)).To(Succeed())

			err := listApplicationsLocal(tempDir, contentWriter)
			Expect(err).To(HaveOccurred())
		})

		It("should return error for manifest files without app name", func() {
			var out bytes.Buffer
			contentWriter := bufio.NewWriter(&out)
			tempDir := createTempDir()
			defer os.RemoveAll(tempDir)

			// Create a text file that's not a valid manifest
			nonManifestFile := filepath.Join(tempDir, "test.txt")
			Expect(os.WriteFile(nonManifestFile, []byte("not a manifest"), 0644)).To(Succeed())

			err := listApplicationsLocal(tempDir, contentWriter)
			// Will error because the file doesn't have an app name
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no app name found"))
		})
	})

	Context("when using discoverLive function", func() {
		var mockProv *mockProvider
		var out bytes.Buffer

		BeforeEach(func() {
			out.Reset()
			// Reset global variables that discoverLive uses
			appName = ""
			orgs = []string{}
			outputDir = ""
		})

		It("should discover apps from multiple orgs", func() {
			orgs = []string{"org1", "org2"}
			discoveredCount := 0
			mockProv = &mockProvider{
				ListAppsFunc: func() (map[string][]any, error) {
					return map[string][]any{
						"org1": {
							cfProvider.AppReference{OrgName: "org1", SpaceName: "space1", AppName: "app1"},
						},
						"org2": {
							cfProvider.AppReference{OrgName: "org2", SpaceName: "space2", AppName: "app2"},
						},
					}, nil
				},
				DiscoverFunc: func(raw any) (*pTypes.DiscoverResult, error) {
					discoveredCount++
					return &pTypes.DiscoverResult{}, nil
				},
			}

			err := discoverLive(mockProv, &out)
			Expect(err).To(Succeed())
			Expect(discoveredCount).To(Equal(2)) // Should discover 2 apps
		})

		It("should skip orgs with no apps and process only orgs with apps", func() {
			orgs = []string{"org1", "org2", "org3"}
			discoveredCount := 0
			mockProv = &mockProvider{
				ListAppsFunc: func() (map[string][]any, error) {
					return map[string][]any{
						"org1": {
							cfProvider.AppReference{OrgName: "org1", SpaceName: "space1", AppName: "app1"},
						},
						"org2": {}, // Empty org - should be skipped in iteration
						// org3 is missing entirely - should be logged as skipped
					}, nil
				},
				DiscoverFunc: func(raw any) (*pTypes.DiscoverResult, error) {
					discoveredCount++
					return &pTypes.DiscoverResult{}, nil
				},
			}

			err := discoverLive(mockProv, &out)
			Expect(err).To(Succeed())
			Expect(discoveredCount).To(Equal(1)) // Only org1 has apps
		})

		It("should filter apps by appName when specified", func() {
			orgs = []string{"org1"}
			appName = "target-app"
			discoveredApps := []string{}

			mockProv = &mockProvider{
				ListAppsFunc: func() (map[string][]any, error) {
					return map[string][]any{
						"org1": {
							cfProvider.AppReference{OrgName: "org1", SpaceName: "space1", AppName: "target-app"},
							cfProvider.AppReference{OrgName: "org1", SpaceName: "space1", AppName: "other-app"},
						},
					}, nil
				},
				DiscoverFunc: func(raw any) (*pTypes.DiscoverResult, error) {
					appRef := raw.(cfProvider.AppReference)
					discoveredApps = append(discoveredApps, appRef.AppName)
					return &pTypes.DiscoverResult{}, nil
				},
			}

			err := discoverLive(mockProv, &out)
			Expect(err).To(Succeed())
			Expect(discoveredApps).To(Equal([]string{"target-app"}))
			Expect(discoveredApps).NotTo(ContainElement("other-app"))
		})

		It("should return error when ListApps fails", func() {
			mockProv = &mockProvider{
				ListAppsFunc: func() (map[string][]any, error) {
					return nil, fmt.Errorf("list apps error")
				},
			}

			err := discoverLive(mockProv, &out)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to list apps by org"))
		})

		It("should return error when Discover fails", func() {
			orgs = []string{"org1"}
			mockProv = &mockProvider{
				ListAppsFunc: func() (map[string][]any, error) {
					return map[string][]any{
						"org1": {
							cfProvider.AppReference{OrgName: "org1", SpaceName: "space1", AppName: "app1"},
						},
					}, nil
				},
				DiscoverFunc: func(raw any) (*pTypes.DiscoverResult, error) {
					return nil, fmt.Errorf("discover error")
				},
			}

			err := discoverLive(mockProv, &out)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("discover error"))
		})

		It("should return error for unexpected app type", func() {
			orgs = []string{"org1"}
			mockProv = &mockProvider{
				ListAppsFunc: func() (map[string][]any, error) {
					return map[string][]any{
						"org1": {
							"invalid-type", // Not an AppReference
						},
					}, nil
				},
			}

			err := discoverLive(mockProv, &out)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unexpected type for app list"))
		})

		It("should handle apps with empty org names", func() {
			orgs = []string{""} // Empty org name in the list
			discoveredCount := 0
			mockProv = &mockProvider{
				ListAppsFunc: func() (map[string][]any, error) {
					return map[string][]any{
						"": {
							cfProvider.AppReference{OrgName: "", SpaceName: "space1", AppName: "app1"},
						},
					}, nil
				},
				DiscoverFunc: func(raw any) (*pTypes.DiscoverResult, error) {
					discoveredCount++
					return &pTypes.DiscoverResult{}, nil
				},
			}

			err := discoverLive(mockProv, &out)
			Expect(err).To(Succeed())
			Expect(discoveredCount).To(Equal(1))
		})
	})

	Context("when performing local discover", func() {

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
			Expect(strings.TrimSpace(out.String())).To(Equal(strings.TrimSpace(expectedOutput)))
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

			It("should handle empty directories gracefully", func() {
				emptyDir := filepath.Join(tempDir, "empty")
				Expect(os.Mkdir(emptyDir, 0755)).To(Succeed())

				_, cmd = NewDiscoverCloudFoundryCommand(log)
				cmd.SetOut(&out)
				cmd.SetErr(&err)
				cmd.SetArgs([]string{"--input", emptyDir})

				executeErr := cmd.Execute()

				Expect(executeErr).To(Succeed())
				Expect(out.String()).To(BeEmpty())
			})

			It("should skip nested directories and process only files", func() {
				manifestDir := filepath.Join(tempDir, "manifests")
				nestedDir := filepath.Join(manifestDir, "nested")
				Expect(os.MkdirAll(nestedDir, 0755)).To(Succeed())

				// Create file in main directory
				mainFile := filepath.Join(manifestDir, "main-app.yaml")
				Expect(os.WriteFile(mainFile, []byte("name: main-app\nmemory: 256M"), 0644)).To(Succeed())

				// Create file in nested directory (should be skipped)
				nestedFile := filepath.Join(nestedDir, "nested-app.yaml")
				Expect(os.WriteFile(nestedFile, []byte("name: nested-app\nmemory: 256M"), 0644)).To(Succeed())

				_, cmd = NewDiscoverCloudFoundryCommand(log)
				cmd.SetOut(&out)
				cmd.SetErr(&err)
				cmd.SetArgs([]string{"--input", manifestDir})

				executeErr := cmd.Execute()

				Expect(executeErr).To(Succeed())
				Expect(out.String()).To(ContainSubstring("main-app"))
				Expect(out.String()).NotTo(ContainSubstring("nested-app"))
			})
		})

		Context("when using app-name filter with file-based discovery", func() {
			It("should filter apps by name when app-name flag is provided", func() {
				// Create directory with multiple apps
				manifestDir := filepath.Join(tempDir, "manifests")
				Expect(os.Mkdir(manifestDir, 0755)).To(Succeed())

				manifest1 := filepath.Join(manifestDir, "app1.yaml")
				manifest2 := filepath.Join(manifestDir, "app2.yaml")

				Expect(os.WriteFile(manifest1, []byte("name: target-app\nmemory: 256M"), 0644)).To(Succeed())
				Expect(os.WriteFile(manifest2, []byte("name: other-app\nmemory: 512M"), 0644)).To(Succeed())

				_, cmd = NewDiscoverCloudFoundryCommand(log)
				cmd.SetOut(&out)
				cmd.SetErr(&err)
				cmd.SetArgs([]string{"--input", manifestDir, "--app-name", "target-app"})

				executeErr := cmd.Execute()

				Expect(executeErr).To(Succeed())
				Expect(out.String()).To(ContainSubstring("target-app"))
				Expect(out.String()).NotTo(ContainSubstring("other-app"))
			})

			It("should return no results when app-name doesn't match any apps", func() {
				_, cmd = NewDiscoverCloudFoundryCommand(log)
				cmd.SetOut(&out)
				cmd.SetErr(&err)
				cmd.SetArgs([]string{"--input", manifestPath, "--app-name", "non-existent-app"})

				executeErr := cmd.Execute()

				Expect(executeErr).To(Succeed())
				Expect(out.String()).To(BeEmpty())
			})
		})

		Context("when output directory has issues", func() {
			It("should return error when output directory cannot be created", func() {
				// Try to create output directory in a non-existent parent path
				invalidOutputPath := "/nonexistent/path/output"
				_, cmd = NewDiscoverCloudFoundryCommand(log)
				cmd.SetOut(&out)
				cmd.SetErr(&err)
				cmd.SetArgs([]string{"--input", manifestPath, "--output-dir", invalidOutputPath})

				executeErr := cmd.Execute()

				Expect(executeErr).To(HaveOccurred())
				Expect(executeErr.Error()).To(ContainSubstring("failed to create output folder"))
			})
		})

		Context("when processing files with permission issues", func() {
			It("should return error when manifest file cannot be read", func() {
				// Create a file with no read permissions
				unreadableFile := filepath.Join(tempDir, "unreadable.yaml")
				Expect(os.WriteFile(unreadableFile, []byte("name: test-app"), 0000)).To(Succeed())

				_, cmd = NewDiscoverCloudFoundryCommand(log)
				cmd.SetOut(&out)
				cmd.SetErr(&err)
				cmd.SetArgs([]string{"--input", unreadableFile})

				executeErr := cmd.Execute()

				Expect(executeErr).To(HaveOccurred())
				Expect(executeErr.Error()).To(ContainSubstring("permission denied"))
			})
		})

		Context("when testing OutputAppManifestsYAML function", func() {
			BeforeEach(func() {
				// Reset global outputDir to ensure tests write to stdout, not file
				outputDir = ""
				out.Reset()
			})

			It("should output manifest with org, space, and app in the content", func() {
				discoverResult := &pTypes.DiscoverResult{
					Content: map[string]any{
						"name": "test-app",
					},
				}

				err := OutputAppManifestsYAML(&out, discoverResult, "test-org", "test-space", "test-app")
				Expect(err).To(Succeed())

				output := out.String()
				// Verify manifest structure is output
				Expect(output).To(ContainSubstring("manifest:"))
				Expect(output).To(ContainSubstring("name: test-app"))
				// Output should be YAML formatted
				Expect(output).To(MatchRegexp(`manifest:\s+name:`))
			})

			It("should output manifest without org but with space", func() {
				discoverResult := &pTypes.DiscoverResult{
					Content: map[string]any{
						"name": "space-app",
					},
				}

				err := OutputAppManifestsYAML(&out, discoverResult, "", "test-space", "space-app")
				Expect(err).To(Succeed())

				output := out.String()
				Expect(output).To(ContainSubstring("manifest:"))
				Expect(output).To(ContainSubstring("name: space-app"))
				// Verify YAML structure
				Expect(output).NotTo(BeEmpty())
			})

			It("should handle empty org and space names and output basic manifest", func() {
				discoverResult := &pTypes.DiscoverResult{
					Content: map[string]any{
						"name": "basic-app",
					},
				}

				err := OutputAppManifestsYAML(&out, discoverResult, "", "", "basic-app")
				Expect(err).To(Succeed())

				output := out.String()
				Expect(output).To(ContainSubstring("manifest:"))
				Expect(output).To(ContainSubstring("name: basic-app"))
			})

			It("should output secrets section when secrets are present", func() {
				discoverResult := &pTypes.DiscoverResult{
					Content: map[string]any{
						"name": "app-with-secrets",
					},
					Secret: map[string]any{
						"api_key":  "secret-value",
						"password": "secret-pass",
					},
				}

				err := OutputAppManifestsYAML(&out, discoverResult, "", "", "app-with-secrets")
				Expect(err).To(Succeed())

				output := out.String()
				// Verify content section header is present when secrets exist
				Expect(output).To(ContainSubstring("--- Content Section ---"))
				Expect(output).To(ContainSubstring("manifest:"))
				Expect(output).To(ContainSubstring("name: app-with-secrets"))

				// Verify secrets section
				Expect(output).To(ContainSubstring("--- Secrets Section ---"))
				Expect(output).To(ContainSubstring("api_key: secret-value"))
				Expect(output).To(ContainSubstring("password: secret-pass"))
			})

			It("should not output secrets section when secrets are empty", func() {
				discoverResult := &pTypes.DiscoverResult{
					Content: map[string]any{
						"name": "app-no-secrets",
					},
					Secret: map[string]any{},
				}

				err := OutputAppManifestsYAML(&out, discoverResult, "", "", "app-no-secrets")
				Expect(err).To(Succeed())

				output := out.String()
				// Should not have section headers when no secrets
				Expect(output).NotTo(ContainSubstring("--- Content Section ---"))
				Expect(output).NotTo(ContainSubstring("--- Secrets Section ---"))
				Expect(output).To(ContainSubstring("manifest:"))
			})
		})

		Context("when testing processAppList function", func() {
			var mockProv *mockProvider

			BeforeEach(func() {
				out.Reset()
				appName = "" // Reset global var
				outputDir = ""
			})

			It("should process all apps in the list", func() {
				processedApps := []string{}
				mockProv = &mockProvider{
					DiscoverFunc: func(raw any) (*pTypes.DiscoverResult, error) {
						appRef := raw.(cfProvider.AppReference)
						processedApps = append(processedApps, appRef.AppName)
						return &pTypes.DiscoverResult{}, nil
					},
				}

				appList := []any{
					cfProvider.AppReference{OrgName: "org1", SpaceName: "space1", AppName: "app1"},
					cfProvider.AppReference{OrgName: "org1", SpaceName: "space1", AppName: "app2"},
				}

				err := processAppList(mockProv, appList, &out)
				Expect(err).To(Succeed())
				Expect(processedApps).To(Equal([]string{"app1", "app2"}))
			})

			It("should filter by appName when specified", func() {
				appName = "app2"
				processedApps := []string{}
				mockProv = &mockProvider{
					DiscoverFunc: func(raw any) (*pTypes.DiscoverResult, error) {
						appRef := raw.(cfProvider.AppReference)
						processedApps = append(processedApps, appRef.AppName)
						return &pTypes.DiscoverResult{}, nil
					},
				}

				appList := []any{
					cfProvider.AppReference{OrgName: "org1", SpaceName: "space1", AppName: "app1"},
					cfProvider.AppReference{OrgName: "org1", SpaceName: "space1", AppName: "app2"},
					cfProvider.AppReference{OrgName: "org1", SpaceName: "space1", AppName: "app3"},
				}

				err := processAppList(mockProv, appList, &out)
				Expect(err).To(Succeed())
				Expect(processedApps).To(Equal([]string{"app2"}))
			})

			It("should return error for invalid app type", func() {
				mockProv = &mockProvider{}

				appList := []any{
					"invalid-type",
				}

				err := processAppList(mockProv, appList, &out)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unexpected type for app list"))
			})

			It("should return error when Discover fails", func() {
				mockProv = &mockProvider{
					DiscoverFunc: func(raw any) (*pTypes.DiscoverResult, error) {
						return nil, fmt.Errorf("discover failed")
					},
				}

				appList := []any{
					cfProvider.AppReference{OrgName: "org1", SpaceName: "space1", AppName: "app1"},
				}

				err := processAppList(mockProv, appList, &out)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("discover failed"))
			})
		})
	})
})

// Helper functions to reduce duplication and improve readability

func createTempDir() string {
	tempDir, err := os.MkdirTemp("", "cloud_foundry_test")
	Expect(err).NotTo(HaveOccurred())
	return tempDir
}

func createMultipleManifests(tempDir string, howmany int) {
	for i := 0; i < howmany; i++ {
		createTestManifest(tempDir, strconv.Itoa(i))
	}
}
func createTestManifest(tempDir string, suffix ...string) string {
	appName := "test-app"
	suffixVal := ""
	if len(suffix) > 0 && suffix[0] != "" {
		suffixVal = "-" + suffix[0]
	}
	manifestContent := []byte(fmt.Sprintf(`---
name: %s
memory: 256M
instances: 1
`, appName+suffixVal))

	manifestPath := filepath.Join(tempDir, "manifest"+suffixVal+".yaml")
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
name: test-app-with-sensitive-data
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
	return `manifest:
    name: test-app
    processes:
        - type: web
          memory: 256M
          healthCheck:
            invocationTimeout: 1
            interval: 30
            type: port
            timeout: 60
          readinessCheck:
            type: process
          instances: 1`
}

func helperCreateTestManifest(manifestPath string, content string, perm os.FileMode) error {
	err := os.WriteFile(manifestPath, []byte(content), perm) //0644
	if err != nil {
		return err
	}
	return nil
}
