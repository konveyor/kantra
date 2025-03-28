package cloud_foundry

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

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
		korifiClient, err := getKorifiHttpClient()
		if err != nil {
			fmt.Printf("Error creating client: %v\n", err)
			return err
		}

		cfInfo, err := getInfo(korifiClient)
		if err != nil {
			fmt.Printf("Can't get info: %v\n", err)
			return err
		}

		log.Println(cfInfo)
		// var cfAppsManifest []*discover.AppManifest
		name, err := NormalizeForMetadataName(strings.TrimSpace(cfInfo.Name))
		if err != nil {
			fmt.Printf("Can't normalize name: %v\n", err)
			return err
		}

		log.Println("normalized name: ", name)
		log.Println("\n--------------------------")

		apps, err := listAllCfApps(korifiClient)
		if err != nil {
			fmt.Printf("Error creating request: %v\n", err)
			return err
		}

		log.Println("Apps: ", apps)
		var cfManifest discover.CloudFoundryManifest
		for _, app := range apps.Resources {
			fmt.Println(app)
			fmt.Println(app.GUID)
			appEnv, err := getEnv(korifiClient, app.GUID)
			if err != nil {
				return err
			}
			log.Println("*************************************")
			log.Println(appEnv)

			appManifest := discover.AppManifest{}
			appEnv.ApplicationEnvJSON["custom-gloria"] = "custom-value"
			appEnv.ApplicationEnvJSON["custom-gloria-array"] = []any{"custom-value1", "custom-value2"}
			// TODO: other env var?
			err = setVCAPEnv(&appManifest, *appEnv)
			if err != nil {
				return err
			}

			for key, value := range appManifest.Env {
				fmt.Printf("%s: %s\n", key, value)
			}
			log.Println("++++++++++++++++++++++++++++++++++++++")
			log.Println(appManifest)
			log.Println("######################################")
			cfManifest.Applications = append(cfManifest.Applications, &appManifest)
		}

		fmt.Println(cfManifest)
		writeToYAMLFile(cfManifest, "manifest.yaml")
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
