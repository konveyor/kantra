package cloud_foundry

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"path/filepath"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/homedir"
)

func getKubeConfig() (*api.Config, error) {
	home := homedir.HomeDir()
	kubeconfig := filepath.Join(home, ".kube", "config")

	// Load kubeconfig
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

func getKorifiHttpClient() *http.Client {
	tlsConfig := &tls.Config{
		// Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true, // Use with caution in production
	}

	// Create custom HTTP client
	tr := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	return &http.Client{Transport: tr}

}

func listAllCfApps(httpClient *http.Client, clientCertificate string) (string, error) {
	// Create request
	req, err := http.NewRequest("GET", "https://localhost/v3/apps", nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return "", err
	}

	req.Header.Set("Authorization", "ClientCert "+clientCertificate)

	// Send request
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Read and print response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	fmt.Printf("Response Status: %s\n", resp.Status)
	fmt.Println(string(body))
	return string(body), nil
}
