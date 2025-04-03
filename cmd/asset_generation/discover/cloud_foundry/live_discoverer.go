package cloud_foundry

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/homedir"

	"github.com/go-logr/logr"
	discover "github.com/konveyor/asset-generation/pkg/discover/cloud_foundry"
)

type LiveDiscoverer struct {
	client *http.Client
	// kubeContext string
	logger *logr.Logger
}

func getKubeConfig() (*api.Config, error) {
	home := homedir.HomeDir()
	kubeconfig := filepath.Join(home, ".kube", "config")

	config, err := clientcmd.LoadFromFile(kubeconfig)
	if err != nil {
		fmt.Printf("Error loading kubeconfig: %v\n", err)
		return nil, err
	}
	return config, nil
}

func getClientCertificate(config *api.Config) (string, error) {
	// Find the desired user context (in this case, "kind-korifi")
	var dataCert, keyCert []byte
	for username, authInfo := range config.AuthInfos {
		if username == "kind-korifi" {
			dataCert = authInfo.ClientCertificateData
			keyCert = authInfo.ClientKeyData
			break
		}
	}

	if len(dataCert) == 0 || len(keyCert) == 0 {
		return "", fmt.Errorf("could not find certificate data for kind-korifi")
	}

	return base64.StdEncoding.EncodeToString(append(dataCert, keyCert...)), nil
}

func getKorifiHttpClient() (*http.Client, error) {
	config, err := getKubeConfig()
	if err != nil {
		return nil, err
	}
	certPEM, err := getClientCertificate(config)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	// Create a custom RoundTripper that adds the Authorization header
	roundTripper := &authHeaderRoundTripper{
		certPEM: certPEM,
		base:    transport,
	}

	// Create an HTTP client with the custom RoundTripper
	return &http.Client{
		Transport: roundTripper,
	}, nil
}

func NewLiveDiscoverer(log logr.Logger) (*LiveDiscoverer, error) {
	client, err := getKorifiHttpClient()
	if err != nil {
		return nil, fmt.Errorf("error creating Korifi client: %v", err)
	}
	return &LiveDiscoverer{client: client, logger: &log}, nil
}

func (ld *LiveDiscoverer) Discover() (*discover.CloudFoundryManifest, error) {
	apps, err := listAllCfApps(ld.client)
	if err != nil {
		return nil, fmt.Errorf("error listing CF apps: %v", err)
	}

	log.Println("Apps discovered:", apps)

	var cfManifest discover.CloudFoundryManifest
	for _, app := range apps.Resources {
		log.Println("Processing app:", app.GUID)

		appEnv, err := getEnv(ld.client, app.GUID)
		if err != nil {
			return nil, fmt.Errorf("error getting environment for app %s: %v", app.GUID, err)
		}

		appName, err := getAppName(*appEnv)
		if err != nil {
			return nil, fmt.Errorf("error getting app name: %v", err)
		}

		normalizedAppName, err := NormalizeForMetadataName(strings.TrimSpace(appName))
		if err != nil {
			return nil, fmt.Errorf("error normalizing app name: %v", err)
		}

		process, err := getProcesses(ld.client, app.GUID)
		if err != nil {
			return nil, fmt.Errorf("error getting processes: %v", err)
		}

		appProcesses := discover.AppManifestProcesses{}
		for _, proc := range process.Resources {
			procInstances := uint(proc.Instances)

			appProcesses = append(appProcesses, discover.AppManifestProcess{
				Type:                         discover.AppProcessType(proc.Type),
				Command:                      proc.Command,
				DiskQuota:                    string(proc.DiskQuotaMB),
				HealthCheckType:              discover.AppHealthCheckType(proc.HealthCheck.Type),
				HealthCheckHTTPEndpoint:      proc.HealthCheck.Data.HTTPEndpoint,
				HealthCheckInvocationTimeout: uint(proc.HealthCheck.Data.InvocationTimeout),
				Instances:                    &procInstances,
				// LogRateLimitPerSecond
				Memory: string(proc.MemoryMB), //TODO: add suffix 'MB'?
				// Timeout
				// ReadinessHealthCheckType
				// ReadinessHealthCheckHttpEndpoint
				// ReadinessHealthInvocationTimeout
				// ReadinessHealthCheckInterval
				// Lifecycle

			})
		}

		routes, err := getRoutes(ld.client, app.GUID)
		if err != nil {
			return nil, fmt.Errorf("error getting processes: %v", err)
		}
		appRoutes := discover.AppManifestRoutes{}
		for _, r := range routes.Resources {
			appRoutes = append(appRoutes, discover.AppManifestRoute{
				Route:    r.URL,
				Protocol: discover.AppRouteProtocol(r.Protocol),
				// Options: loadbalancing?
			})
		}

		appManifest := discover.AppManifest{
			Name: normalizedAppName,
			Env:  appEnv.EnvironmentVariables,
			//TODO how to get this info?
			Metadata: &discover.AppMetadata{
				Labels:      app.Metadata.Labels,
				Annotations: app.Metadata.Annotations,
			},
			Processes:          &appProcesses,
			Routes:             &appRoutes,
			AppManifestProcess: discover.AppManifestProcess{
				// app.
			},
			// Buildpacks

			// RandomRoute
			// NoRoute
			// Services
			// Sidecars
			// Processes
			// Stack
		}
		fmt.Printf("append: %v\n\n", appManifest)
		cfManifest.Applications = append(cfManifest.Applications, &appManifest)

	}

	err = writeToYAMLFile(cfManifest, "manifest.yaml")
	if err != nil {
		return nil, fmt.Errorf("error writing manifest to file: %v", err)
	}

	return &cfManifest, nil
}
