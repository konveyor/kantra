package cloud_foundry

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

var _ = Describe("Discover Manifest", func() {
	var (
		out          bytes.Buffer
		writer       *bufio.Writer = bufio.NewWriter(&out)
		tempDir      string
		manifestPath string
		// useLive bool
	)

	BeforeEach(func() {
		tempDir, err := os.MkdirTemp("", "cloud_foundry_test")
		Expect(err).NotTo(HaveOccurred())
		manifestPath = filepath.Join(tempDir, "manifest.yaml")
		input = manifestPath
		output = ""
		// Reset buffers before each test
		writer.Reset(&out)
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	DescribeTable("Manifest validation",
		func(manifestContent string, expectedErrorMessage ...string) {
			err := helperCreateTestManifest(manifestPath, manifestContent, 0644)
			Expect(err).ToNot(HaveOccurred(), "Unable to create manifest.yaml")
			input = manifestPath
			output = ""
			err = discoverManifest(writer)
			writer.Flush()

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
		Entry("with an empty manifest", "", "field validation for key 'Application.Metadata' field 'Metadata' failed on the 'required' tag"),
		Entry("with invalid YAML content", "invalid content", "cannot unmarshal !!str `invalid...` into cloud_foundry.AppManifest"),
		Entry("with a valid manifest", `name: test-app`, nil),
	)
})
var _ = Describe("Discover command", func() {

	var (
		log    logr.Logger
		out    bytes.Buffer
		err    bytes.Buffer
		writer *bufio.Writer

		tempDir         string
		cmd             *cobra.Command
		outputPath      string
		manifestContent []byte
		manifestPath    string
	)

	BeforeEach(func() {
		log = logr.Discard()
		out.Reset()
		err.Reset()
		writer = bufio.NewWriter(&out)

		// Create a temporary directory for test files
		tempDir, err := os.MkdirTemp("", "cloud_foundry_test")
		Expect(err).NotTo(HaveOccurred())

		manifestContent = []byte(`---
name: test-app
memory: 256M
instances: 1
`)

		manifestPath = filepath.Join(tempDir, "manifest.yaml")
		Expect(os.WriteFile(manifestPath, manifestContent, 0644)).To(Succeed())
		Expect(manifestPath).ToNot(BeEmpty())
		outputPath = filepath.Join(tempDir, "output.yaml")
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	type flagFile struct {
		setFlag  bool
		filePath string
	}
	type flagTest struct {
		Input              flagFile
		Output             flagFile
		ExpectSuccess      bool
		ExpectedOut        string
		ExpectErr          bool
		ExpectedErrMessage string
		OutputVerification bool
	}

	DescribeTable("flag behavior",
		func(flags flagTest) {
			_, cmd = NewDiscoverCloudFoundryCommand(log)

			cmd.SetOut(&out)
			cmd.SetErr(&err)
			args := []string{}

			if flags.Input.setFlag {
				args = append(args, "--input")
				if len(flags.Input.filePath) != 0 {
					args = append(args, flags.Input.filePath)
				} else {
					// Default
					args = append(args, manifestPath)
				}
			}
			if flags.Output.setFlag {
				args = append(args, "--output")
				if len(flags.Output.filePath) != 0 {
					args = append(args, flags.Output.filePath)
					defer os.Remove(flags.Output.filePath)
				} else {
					// Default
					args = append(args, outputPath)
				}
			}

			cmd.SetArgs(args)

			e := cmd.Execute()
			writer.Flush()

			if flags.ExpectSuccess {
				Expect(e).To(Succeed())
			} else {
				Expect(e).To(HaveOccurred())
			}

			if flags.ExpectErr {
				Expect(err.String()).To(ContainSubstring(flags.ExpectedErrMessage)) // Check STDERR for errors
			}

			if flags.OutputVerification {
				var outputContent string
				if flags.Output.setFlag {
					outputContentByte, err := os.ReadFile(outputPath)
					Expect(err).NotTo(HaveOccurred())
					outputContent = string(outputContentByte)
					Expect(out.String()).To(BeEmpty()) // Standard output should be empty
				} else {
					Expect(out.String()).ToNot(BeEmpty()) // Standard output should not be empty
					outputContent = out.String()
				}

				Expect(string(outputContent)).To(Equal(flags.ExpectedOut))
			} else {
				Expect(out.String()).To(ContainSubstring(flags.ExpectedOut)) // Check STDOUT for normal output
			}
		},

		Entry("discovers manifest and prints output to standard output when flags are valid",
			flagTest{
				Input:         flagFile{setFlag: true},
				Output:        flagFile{},
				ExpectSuccess: true,
				ExpectedOut:   "test-app",
			},
		),
		Entry("discovers manifest and prints output to standard output when flags are valid and verify output",
			flagTest{
				Input:         flagFile{setFlag: true},
				Output:        flagFile{},
				ExpectSuccess: true,
				ExpectedOut: `name: test-app
version: ""
timeout: 60
instances: 1
`,
				ExpectedErrMessage: "",
				OutputVerification: true},
		),
		Entry("writes to output file when --output flag is given",
			flagTest{
				Input:         flagFile{setFlag: true},
				Output:        flagFile{setFlag: true},
				ExpectSuccess: true,
			},
		),
		Entry("writes to output file when --output flag is given and verify output",
			flagTest{
				Input:         flagFile{setFlag: true},
				Output:        flagFile{setFlag: true},
				ExpectSuccess: true,
				ExpectedOut: `name: test-app
version: ""
timeout: 60
instances: 1
`,
				OutputVerification: true},
		),
		Entry("returns an error when input file is missing",
			flagTest{
				Input:              flagFile{},
				Output:             flagFile{},
				ExpectErr:          true,
				ExpectedErrMessage: "required flag",
			},
		),

		Entry("returns an error when input file does not exist",

			flagTest{
				Input:              flagFile{setFlag: true, filePath: "nonexistent.yaml"},
				Output:             flagFile{},
				ExpectErr:          true,
				ExpectedErrMessage: "no such file or directory",
			},
		),
	)
})

func helperCreateTestManifest(manifestPath string, content string, perm os.FileMode) error {
	err := os.WriteFile(manifestPath, []byte(content), perm) //0644
	if err != nil {
		return err
	}
	return nil
}
