package cloud_foundry

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	discover "github.com/konveyor/asset-generation/pkg/discover/cloud_foundry"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
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

func getKorifiHttpClient() (*http.Client, error) {
	config, err := getKubeConfig()
	if err != nil {
		return nil, err
	}
	certPEM, err := getClientCertificate(config)
	if err != nil {
		return nil, err
	}

	// Create a custom transport
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Use with caution
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

// Custom RoundTripper to add Authorization header
type authHeaderRoundTripper struct {
	certPEM string
	base    http.RoundTripper
}

func (t *authHeaderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	reqClone := req.Clone(req.Context())

	// Set the Authorization header
	reqClone.Header.Set("Authorization", "ClientCert "+t.certPEM)

	// Use the base transport to execute the request
	return t.base.RoundTrip(reqClone)
}

func listAllCfApps(httpClient *http.Client) (*ListResponse[AppResponse], error) {
	resp, err := httpClient.Get("https://localhost/v3/apps")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read and print response
	// body, err := io.ReadAll(resp.Body)
	// if err != nil {
	// 	return "", err
	// }
	// if resp.StatusCode != 200 {
	// 	return "", fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	// }

	var i ListResponse[AppResponse]
	// fmt.Printf("%+v\n", string(body))

	err = json.NewDecoder(resp.Body).Decode(&i)
	if err != nil {
		return nil, errors.Wrap(err, "Error unmarshalling info")
	}
	return &i, nil
	// return string(body), nil
}

func getInfo(httpClient *http.Client) (*InfoV3Response, error) {
	resp, err := httpClient.Get("https://localhost/v3/info")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read and print response
	// body, err := io.ReadAll(resp.Body)
	// if err != nil {
	// 	return nil, err
	// }
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

// // JsonifyMapValues converts the map values to json
// func JsonifyMapValues(inputMap map[string]interface{}) map[string]interface{} {
// 	for key, value := range inputMap {
// 		if value == nil {
// 			inputMap[key] = ""
// 			continue
// 		}
// 		if val, ok := value.(string); ok {
// 			inputMap[key] = val
// 			continue
// 		}
// 		var b bytes.Buffer
// 		encoder := json.NewEncoder(&b)
// 		if err := encoder.Encode(value); err != nil {
// 			fmt.Errorf("Unable to unmarshal data to json while converting map interfaces to string")
// 			continue
// 		}
// 		strValue := b.String()
// 		strValue = strings.TrimSpace(strValue)
// 		inputMap[key] = strValue
// 	}
// 	return inputMap
// }

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
	// fmt.Printf("%+v\n", string(body))

	err = json.NewDecoder(resp.Body).Decode(&i)
	if err != nil {
		return nil, errors.Wrap(err, "Error unmarshalling info")
	}
	return &i, nil
}

func VCAPAppToMap(data map[string]any) (map[string]string, error) {
	result := make(map[string]string)

	for key, value := range data {
		if arr, ok := value.([]any); ok {
			// If value is an array, convert it to a comma-separated string
			var strValues []string
			for _, item := range arr {
				strValues = append(strValues, fmt.Sprintf("%v", item))
			}
			result[key] = strings.Join(strValues, ",")
		} else {
			// Convert non-array values to string
			result[key] = fmt.Sprintf("%v", value)
		}
	}

	return result, nil
}

func extractVCAPApplication(env AppEnvResponse) (map[string]any, error) {
	vcapApp, ok := env.ApplicationEnvJSON["VCAP_APPLICATION"]
	if !ok {
		return nil, fmt.Errorf("VCAP_APPLICATION not found in ApplicationEnvJSON")
	}
	return vcapApp.(map[string]any), nil
}

func mapToVCAPEnv(data map[string]any) (map[string]string, error) {
	result := make(map[string]string)
	for k, v := range data {
		fmt.Printf("key %s \t value: %s\n", k, v)
		// TODO OTHER THAN VCAP_APPLICATION?
		if k == "VCAP_APPLICATION" {
			// Recursively process the inner map
			innerMap, err := mapToVCAPEnv(v.(map[string]any))
			if err != nil {
				return nil, fmt.Errorf("failed to process inner map for key %s: %v", k, err)
			}

			// Add the inner map's key-value pairs to the result
			for innerK, innerV := range innerMap {
				result[innerK] = innerV
			}
		} else {

			switch v := v.(type) {
			case string:
				result[k] = v
			case int, int64, int32:
				result[k] = fmt.Sprintf("%d", v)
			case float64, float32:
				result[k] = fmt.Sprintf("%f", v)
			case bool:
				result[k] = fmt.Sprintf("%t", v)
			case []any:
				// Preserve arrays by converting to a representation that can be parsed
				arrayStr := "["
				for i, elem := range v {
					if i > 0 {
						arrayStr += ", "
					}
					arrayStr += fmt.Sprintf("%q", fmt.Sprintf("%v", elem))
				}
				arrayStr += "]"
				result[k] = arrayStr

			default: // Fallback for other types
				result[k] = fmt.Sprintf("%v", v)
			}
		}
	}
	return result, nil
}

func VCAPEnvToMap(env VCAPApplicationEnv) map[string]string {
	return map[string]string{
		"application_id":    env.ApplicationId,
		"application_name":  env.ApplicationName,
		"name":              env.Name,
		"organization_id":   env.OrganizationId,
		"organization_name": env.OrganizationName,
		"space_id":          env.SpaceId,
		"space_name":        env.SpaceName,
		"uris":              env.URIs,
		"application_uris":  env.ApplicationURIs,
	}
}

func setVCAPEnv(appManifest *discover.AppManifest, appEnv AppEnvResponse) error {

	VCAPEnvMap, err := mapToVCAPEnv(appEnv.ApplicationEnvJSON)
	if err != nil {
		return err
	}
	appManifest.Env = VCAPEnvMap
	return nil
}

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

// Custom YAML marshaling function
func marshalYAMLWithArrays(data map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	for k, v := range data {
		switch val := v.(type) {
		case []any:
			// Keep the array as-is
			result[k] = val
		case string, int, int64, float64, bool:
			// Keep simple types as-is
			result[k] = val
		default:
			// Convert other types to string representation
			result[k] = fmt.Sprintf("%v", v)
		}
	}

	return result, nil
}
