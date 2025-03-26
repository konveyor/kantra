package cloud_foundry

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/go-logr/logr"
	discover "github.com/konveyor/asset-generation/pkg/discover/cloud_foundry"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var (
	useLive        bool
	input          string
	output         string
	cfURL, cfToken string
)

func NewDiscoverCloudFoundryCommand(log logr.Logger) (string, *cobra.Command) {

	cmd := &cobra.Command{
		Aliases: []string{"cf"},
		Use:     "cloud-foundry",
		Short:   "Discover Cloud Foundry applications",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.ParseFlags(args); err != nil {
				return err
			}
			// Validate flags dynamically based on --use-live-connection
			// if useLive {
			// 	if cfURL == "" {
			// 		return fmt.Errorf("--cf-url is required when --use-live-connection is enabled")
			// 	}
			// 	if cfToken == "" {
			// 		return fmt.Errorf("--cf-token is required when --use-live-connection is enabled")
			// 	}
			// }
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return discoverManifest(cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&input, "input", "", "specify the location of the manifest.yaml to analyze.")
	cmd.Flags().StringVar(&output, "output", "", "output file (default: standard output).")
	cmd.Flags().BoolVar(&useLive, "use-live-connection", false, "uses live platform connections for real-time discovery")
	cmd.Flags().StringVar(&cfURL, "cf-url", "", "cf API URL (e.g., http://172.18.0.2:30050).")
	cmd.Flags().StringVar(&cfToken, "cf-token", "", "Authentication token for cf (use 'cf oauth-token' to get).")
	cmd.MarkFlagFilename("input", "yaml", "yml")
	cmd.MarkFlagFilename("output")
	cmd.MarkFlagsOneRequired("input", "use-live-connection")
	cmd.MarkFlagsMutuallyExclusive("input", "use-live-connection")

	return "Cloud Foundry V3 (local manifest)", cmd
}

func discoverManifest(writer io.Writer) error {
	var err error
	var b []byte
	if !useLive {
		b, err = os.ReadFile(input)
		if err != nil {
			return err
		}
	} else {
		// Load kubeconfig
		config, err := getKubeConfig()
		if err != nil {
			fmt.Printf("Error loading kubeconfig: %v\n", err)
			return err
		}

		clientCertificateData, err := getClientCertificate(config)
		// // Combine certificate data (base64 encoded) like in the curl command
		// combinedCertData := base64.StdEncoding.EncodeToString(append(dataCert, keyCert...))
		httpClient := getKorifiHttpClient()

		// Create request
		req, err := http.NewRequest("GET", "https://localhost/v3/apps", nil)
		if err != nil {
			fmt.Printf("Error creating request: %v\n", err)
			return err
		}

		// Set Authorization header exactly like the curl command
		req.Header.Set("Authorization", "ClientCert "+clientCertificateData)

		// Send request
		resp, err := httpClient.Do(req)
		if err != nil {
			fmt.Printf("Error making request: %v\n", err)
			return err
		}
		defer resp.Body.Close()

		// Read and print response
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Printf("Error reading response: %v\n", err)
			return err
		}

		fmt.Printf("Response Status: %s\n", resp.Status)
		fmt.Println(string(body))
		return nil
	}
	ma := discover.AppManifest{}
	err = yaml.Unmarshal(b, &ma)
	if err != nil {
		return err
	}
	a, err := discover.Discover(ma)
	if err != nil {
		return err

	}

	b, err = yaml.Marshal(a)
	if err != nil {
		return err

	}
	if output == "" {
		fmt.Fprintf(writer, "%s", b)
		return nil
	}
	return os.WriteFile(output, b, 0644)
}
