package cloud_foundry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// Custom RoundTripper to add Authorization header
type authHeaderRoundTripper struct {
	certPEM string
	base    http.RoundTripper
}

func (t *authHeaderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	reqClone := req.Clone(req.Context())

	// Set the Authorization header
	reqClone.Header.Set("Authorization", "ClientCert "+t.certPEM)
	reqClone.Header.Set("X-Username", "kubernetes-admin")
	// Use the base transport to execute the request
	return t.base.RoundTrip(reqClone)
}

// func listAllCfApps2(clientset *http.Client) (*ListResponse[AppResponse], error) {
// 	// Create HTTP client for Korifi API
// 	client, err := getKorifiHttpClient()
// 	if err != nil {
// 		log.Fatalf("Failed to create Korifi HTTP client: %v", err)
// 	}

// 	// Construct the URL for the Korifi API endpoint
// 	url := "https://localhost/v3/apps" // Modify this URL if necessary

// 	// Create a GET request to the endpoint
// 	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
// 	if err != nil {
// 		log.Fatalf("Failed to create request: %v", err)
// 	}

// 	// Make the request
// 	resp, err := client.Do(req)
// 	if err != nil {
// 		log.Fatalf("Request failed: %v", err)
// 	}
// 	defer resp.Body.Close()

// 	// Read and output the response body
// 	body, err := ioutil.ReadAll(resp.Body)
// 	if err != nil {
// 		log.Fatalf("Failed to read response body: %v", err)
// 	}

// 	// Output the response status and body
// 	fmt.Printf("Response Status: %s\n", resp.Status)
// 	fmt.Printf("Response Body: %s\n", string(body))

// 	return nil, nil
// }

func listAllCfApps(httpClient *http.Client) (*ListResponse[AppResponse], error) {
	fmt.Printf(":: \\\\\\\\\\\\\\ listApp func ")

	resp, err := httpClient.Get("https://localhost/v3/apps")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, resp.Status)
	}

	var i ListResponse[AppResponse]
	err = json.NewDecoder(resp.Body).Decode(&i)
	if err != nil {
		return nil, errors.Wrap(err, "Error unmarshalling info")
	}
	return &i, nil
}

func getInfo(httpClient *http.Client) (*InfoV3Response, error) {
	resp, err := httpClient.Get("https://localhost/v3/info")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	fmt.Println("status code", resp.Status)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, resp.Status)
	}

	var i InfoV3Response
	// fmt.Printf("%+v\n", string(body))

	err = json.NewDecoder(resp.Body).Decode(&i)
	if err != nil {
		return nil, errors.Wrap(err, "Error unmarshalling info")
	}
	return &i, nil
}

// disallowedDNSCharactersRegex provides pattern for characters not allowed in a DNS Name
var disallowedDNSCharactersRegex = regexp.MustCompile(`[^a-z0-9\-]`)

// ReplaceStartingTerminatingHyphens replaces the first and last characters of a string if they are hyphens
func ReplaceStartingTerminatingHyphens(str, startReplaceStr, endReplaceStr string) string {
	first := str[0]
	last := str[len(str)-1]
	if first == '-' {
		fmt.Printf("Warning: The first character of the name %q are not alphanumeric.\n", str)
		str = startReplaceStr + str[1:]
	}
	if last == '-' {
		fmt.Printf("Warning: The last character of the name %q are not alphanumeric.", str)
		str = str[:len(str)-1] + endReplaceStr
	}
	return str
}

// NormalizeForMetadataName converts the string to be compatible for service name
func NormalizeForMetadataName(metadataName string) (string, error) {
	if metadataName == "" {
		return "", fmt.Errorf("failed to normalize for service/metadata name because it is an empty string")
	}
	newName := disallowedDNSCharactersRegex.ReplaceAllLiteralString(strings.ToLower(metadataName), "-")
	maxLength := 63
	if len(newName) > maxLength {
		newName = newName[0:maxLength]
	}
	newName = ReplaceStartingTerminatingHyphens(newName, "a", "z")
	if newName != metadataName {
		fmt.Printf("Changing metadata name from %s to %s\n", metadataName, newName)
	}
	return newName, nil
}

// ============== Env section ===============

func getEnv(httpClient *http.Client, guid string) (*AppEnvResponse, error) {
	resp, err := httpClient.Get(fmt.Sprintf("https://localhost/v3/apps/%s/env", guid))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, resp.Status)
	}

	var i AppEnvResponse
	err = json.NewDecoder(resp.Body).Decode(&i)
	if err != nil {
		return nil, errors.Wrap(err, "Error unmarshalling info")
	}
	return &i, nil
}

func getAppName(appEnv AppEnvResponse) (string, error) {
	vcap, valid := appEnv.ApplicationEnvJSON["VCAP_APPLICATION"]
	if !valid {
		return "", fmt.Errorf("can't find ")
	}
	vcapMap, valid := vcap.(map[string]any) // Ensure it's a map
	if !valid {
		return "", fmt.Errorf("VCAP_APPLICATION is not a valid map: %v", vcap)
	}

	appName, valid := vcapMap["application_name"]
	if !valid {
		return "", fmt.Errorf("can't find application name in VCAP_APPLICATION")
	}

	appNameStr, isString := appName.(string)
	if !isString {
		return "", fmt.Errorf("application_name is not a string: %v", appName)
	}

	return appNameStr, nil
}

// ==========================================================

func writeToYAMLFile(data interface{}, filename string) error {
	// Marshal the data to YAML
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("error marshaling to YAML: %w", err)
	}

	// Write to file with 0644 permissions
	err = os.WriteFile(filename, yamlData, 0644)
	if err != nil {
		return fmt.Errorf("error writing YAML to file: %w", err)
	}

	return nil
}

// =========================================================

func getProcesses(httpClient *http.Client, guid string) (*ListResponse[ProcessResponse], error) {
	resp, err := httpClient.Get(fmt.Sprintf("https://localhost/v3/apps/%s/processes", guid))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, resp.Status)
	}

	var i ListResponse[ProcessResponse]
	err = json.NewDecoder(resp.Body).Decode(&i)
	if err != nil {
		return nil, errors.Wrap(err, "Error unmarshalling info")
	}
	return &i, nil
}

// ========================================================================

func getRoutes(httpClient *http.Client, guid string) (*ListResponse[RouteResponse], error) {
	resp, err := httpClient.Get(fmt.Sprintf("https://localhost/v3/apps/%s/routes", guid))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, resp.Status)
	}

	var i ListResponse[RouteResponse]
	err = json.NewDecoder(resp.Body).Decode(&i)
	if err != nil {
		return nil, errors.Wrap(err, "Error unmarshalling info")
	}
	return &i, nil
}
