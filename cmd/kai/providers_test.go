package kai

import "testing"

func TestValidateProvider(t *testing.T) {
	for _, id := range providerIDs() {
		if err := validateProvider(id); err != nil {
			t.Errorf("expected provider %q to be valid, got %v", id, err)
		}
	}
	if err := validateProvider("does-not-exist"); err == nil {
		t.Error("expected error for unsupported provider")
	}
}

func TestValidateEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"https", "https://api.anthropic.com", false},
		{"http", "http://localhost:8080", false},
		{"empty", "", true},
		{"no scheme", "api.anthropic.com", true},
		{"ftp scheme", "ftp://example.com", true},
		{"no host", "https://", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEndpoint(tt.in)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateEndpoint(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
		})
	}
}

func TestValidateCredentialRef(t *testing.T) {
	single, _ := lookupProvider("anthropic")
	multi, _ := lookupProvider("aws-bedrock")

	tests := []struct {
		name       string
		provider   providerInfo
		secretName string
		key        string
		wantErr    bool
	}{
		{"single with key", single, "creds", "api-key", false},
		{"single missing key", single, "creds", "", true},
		{"single missing secret", single, "", "api-key", true},
		{"multi without key", multi, "creds", "", false},
		{"multi with key rejected", multi, "creds", "api-key", true},
		{"multi missing secret", multi, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCredentialRef(tt.provider, tt.secretName, tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCredentialRef() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCredentialFieldsFor(t *testing.T) {
	single, _ := lookupProvider("anthropic")
	got := credentialFieldsFor(single, "my-key")
	if len(got) != 1 || got[0].SecretKey != "my-key" {
		t.Fatalf("single-key: expected one field stored under the chosen key, got %+v", got)
	}
	if got[0].FromFile {
		t.Errorf("single-key API key should not be a file field")
	}

	gcp, _ := lookupProvider("gcp-vertex-ai")
	got = credentialFieldsFor(gcp, "")
	if len(got) != 1 || got[0].SecretKey != "GOOGLE_APPLICATION_CREDENTIALS_JSON" || !got[0].FromFile {
		t.Fatalf("gcp-vertex-ai: expected a single FromFile ADC field, got %+v", got)
	}

	aws, _ := lookupProvider("aws-bedrock")
	got = credentialFieldsFor(aws, "")
	want := []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_REGION"}
	if len(got) != len(want) {
		t.Fatalf("aws-bedrock: expected %d fields, got %d", len(want), len(got))
	}
	for i, k := range want {
		if got[i].SecretKey != k {
			t.Errorf("aws-bedrock field %d = %q, want %q", i, got[i].SecretKey, k)
		}
	}
}

func TestXAIProvider(t *testing.T) {
	p, ok := lookupProvider("xai")
	if !ok {
		t.Fatal("expected xai provider in the registry")
	}
	if p.CredentialMode != credentialModeSingleKey {
		t.Errorf("xai should be single-key, got %q", p.CredentialMode)
	}
	if err := validateProvider("xai"); err != nil {
		t.Errorf("validateProvider(xai) = %v", err)
	}
	if err := validateCredentialRef(p, "creds", "api-key"); err != nil {
		t.Errorf("validateCredentialRef(xai) = %v", err)
	}
}

func TestMissingSecretKeys(t *testing.T) {
	multi, _ := lookupProvider("aws-bedrock")
	present := map[string][]byte{"AWS_ACCESS_KEY_ID": []byte("x")}
	missing := missingSecretKeys(multi, present)
	if len(missing) != 2 {
		t.Fatalf("expected 2 missing keys, got %v", missing)
	}

	single, _ := lookupProvider("anthropic")
	if got := missingSecretKeys(single, nil); got != nil {
		t.Errorf("single-key provider should report no required keys, got %v", got)
	}
}
