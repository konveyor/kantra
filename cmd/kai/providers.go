package kai

import (
	"fmt"
	"net/url"
	"strings"
)

// credentialMode describes how a provider's credentials are supplied to the
// sandbox, which changes how the Gateway's credentialRef must be filled in.
type credentialMode string

const (
	// credentialModeSingleKey means the credential is a single value stored
	// under one key in the Secret (injected as KONVEYOR_LLM_API_KEY).
	credentialModeSingleKey credentialMode = "single-key"
	// credentialModeMultiKey means the whole Secret is exposed via envFrom
	// (e.g. AWS SigV4 or GCP ADC). The credentialRef.key must be left empty.
	credentialModeMultiKey credentialMode = "multi-key"
)

// credentialField describes one value that must be stored in a provider's
// credential Secret so the wizard knows what to prompt for and under which key.
type credentialField struct {
	// SecretKey is the key the value is stored under in the Secret. For
	// multi-key providers this is also the env-var name exposed via envFrom.
	SecretKey string
	// Prompt is the wizard label shown when asking for the value.
	Prompt string
	// FromFile reads the value from a file path (e.g. a large JSON blob)
	// instead of a masked paste; the file contents are never echoed.
	FromFile bool
}

// providerInfo describes an LLM provider kantra knows how to configure a
// Gateway for. The set mirrors the providers the agentic harness supports
// (see harness/internal/goose/lifecycle.go in agentic-controller).
type providerInfo struct {
	ID              string
	DisplayName     string
	DefaultEndpoint string
	CredentialMode  credentialMode
	// DefaultKeyName is the suggested Secret key for single-key providers.
	DefaultKeyName string
	// CredentialFields are the values a multi-key provider's Secret must
	// contain. Single-key providers leave this empty; their single field is
	// synthesized from the user-chosen key (see credentialFieldsFor).
	CredentialFields []credentialField
}

// supportedProviders is the authoritative registry of providers kantra can
// build a validated Gateway for.
var supportedProviders = []providerInfo{
	{
		ID:              "anthropic",
		DisplayName:     "Anthropic",
		DefaultEndpoint: "https://api.anthropic.com",
		CredentialMode:  credentialModeSingleKey,
		DefaultKeyName:  "api-key",
	},
	{
		ID:              "openai",
		DisplayName:     "OpenAI",
		DefaultEndpoint: "https://api.openai.com/v1",
		CredentialMode:  credentialModeSingleKey,
		DefaultKeyName:  "api-key",
	},
	{
		ID:              "google",
		DisplayName:     "Google AI (Gemini)",
		DefaultEndpoint: "https://generativelanguage.googleapis.com",
		CredentialMode:  credentialModeSingleKey,
		DefaultKeyName:  "api-key",
	},
	{
		ID:              "xai",
		DisplayName:     "xAI (Grok)",
		DefaultEndpoint: "https://api.x.ai",
		CredentialMode:  credentialModeSingleKey,
		DefaultKeyName:  "api-key",
	},
	{
		ID:              "gcp-vertex-ai",
		DisplayName:     "Google Cloud Vertex AI",
		DefaultEndpoint: "https://us-central1-aiplatform.googleapis.com",
		CredentialMode:  credentialModeMultiKey,
		CredentialFields: []credentialField{
			{SecretKey: "GOOGLE_APPLICATION_CREDENTIALS_JSON", Prompt: "Path to service-account JSON file", FromFile: true},
		},
	},
	{
		ID:              "aws-bedrock",
		DisplayName:     "AWS Bedrock",
		DefaultEndpoint: "https://bedrock-runtime.us-east-1.amazonaws.com",
		CredentialMode:  credentialModeMultiKey,
		CredentialFields: []credentialField{
			{SecretKey: "AWS_ACCESS_KEY_ID", Prompt: "AWS Access Key ID"},
			{SecretKey: "AWS_SECRET_ACCESS_KEY", Prompt: "AWS Secret Access Key"},
			{SecretKey: "AWS_REGION", Prompt: "AWS Region"},
		},
	},
}

// lookupProvider returns the registry entry for the given provider id.
func lookupProvider(id string) (providerInfo, bool) {
	for _, p := range supportedProviders {
		if p.ID == id {
			return p, true
		}
	}
	return providerInfo{}, false
}

// providerIDs returns the list of supported provider identifiers.
func providerIDs() []string {
	ids := make([]string, 0, len(supportedProviders))
	for _, p := range supportedProviders {
		ids = append(ids, p.ID)
	}
	return ids
}

// validateProvider confirms the id is one kantra supports.
func validateProvider(id string) error {
	if _, ok := lookupProvider(id); !ok {
		return fmt.Errorf("unsupported provider %q (supported: %s)",
			id, strings.Join(providerIDs(), ", "))
	}
	return nil
}

// validateEndpoint confirms the endpoint is a well-formed absolute URL.
func validateEndpoint(endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("endpoint must be an http(s) URL")
	}
	if u.Host == "" {
		return fmt.Errorf("endpoint must include a host")
	}
	return nil
}

// validateCredentialRef enforces the per-provider credential rules:
//   - single-key providers require a non-empty key
//   - multi-key providers require an empty key (whole Secret via envFrom)
//
// secretName is always required.
func validateCredentialRef(p providerInfo, secretName, key string) error {
	if strings.TrimSpace(secretName) == "" {
		return fmt.Errorf("credential secret name is required")
	}
	switch p.CredentialMode {
	case credentialModeSingleKey:
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("provider %q requires a Secret key holding the API credential", p.ID)
		}
	case credentialModeMultiKey:
		if strings.TrimSpace(key) != "" {
			return fmt.Errorf("provider %q uses the whole Secret via envFrom; leave the key empty", p.ID)
		}
	}
	return nil
}

// credentialFieldsFor returns the values that must be stored in the credential
// Secret for the given provider. Single-key providers have one field stored
// under the user-chosen key; multi-key providers use their declared fields.
func credentialFieldsFor(p providerInfo, chosenKey string) []credentialField {
	if p.CredentialMode == credentialModeSingleKey {
		return []credentialField{{SecretKey: chosenKey, Prompt: p.DisplayName + " API key"}}
	}
	return p.CredentialFields
}

// requiredSecretKeys returns the Secret keys a multi-key provider must contain.
func requiredSecretKeys(p providerInfo) []string {
	keys := make([]string, 0, len(p.CredentialFields))
	for _, f := range p.CredentialFields {
		keys = append(keys, f.SecretKey)
	}
	return keys
}

// missingSecretKeys reports which of the provider's required keys are absent
// from the given Secret data. Used to warn (not fail) during gateway creation.
func missingSecretKeys(p providerInfo, present map[string][]byte) []string {
	var missing []string
	if p.CredentialMode == credentialModeMultiKey {
		for _, k := range requiredSecretKeys(p) {
			if _, ok := present[k]; !ok {
				missing = append(missing, k)
			}
		}
	}
	return missing
}
