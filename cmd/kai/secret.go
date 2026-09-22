package kai

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// buildSecretData assembles the Secret StringData from collected values,
// keyed by each field's SecretKey. Pure (no cluster) so it is unit-testable.
func buildSecretData(fields []credentialField, values map[string]string) map[string]string {
	data := make(map[string]string, len(fields))
	for _, f := range fields {
		data[f.SecretKey] = values[f.SecretKey]
	}
	return data
}

// collectSecretValues interactively prompts for each credential field, masking
// typed values and reading FromFile fields off disk (contents never echoed).
func collectSecretValues(fields []credentialField) (map[string]string, error) {
	values := make(map[string]string, len(fields))
	for _, f := range fields {
		if f.FromFile {
			var path string
			if err := runForm(inputField(f.Prompt, "", &path, readableFileValidator)); err != nil {
				return nil, err
			}
			content, err := os.ReadFile(strings.TrimSpace(path))
			if err != nil {
				return nil, fmt.Errorf("failed to read %s: %w", f.Prompt, err)
			}
			values[f.SecretKey] = string(content)
			continue
		}
		var val string
		if err := runForm(passwordField(f.Prompt, &val, requiredValidator(f.Prompt))); err != nil {
			return nil, err
		}
		values[f.SecretKey] = val
	}
	return values, nil
}

// resolveCredentials determines the credential Secret the Gateway will
// reference. By default it creates the Secret inline — prompting only for the
// credential values (masked), auto-naming the Secret, and using the provider's
// default key — so the common path is dead simple. The user can instead choose
// to reference an existing Secret by name. The returned bool reports whether a
// Secret was created (so the caller can skip the missing-Secret warning).
func resolveCredentials(ctx context.Context, cl client.Client, namespace, gatewayName string, p providerInfo, dryRun bool) (secretName, key string, created bool, err error) {
	if p.CredentialMode == credentialModeSingleKey {
		key = p.DefaultKeyName
	}

	// Dry-run never touches the cluster, so we can only reference a Secret.
	if dryRun {
		secretName, key, err = promptExistingSecret(p)
		return secretName, key, false, err
	}

	create, err := confirm("Create the credential Secret now? (choose No to reference an existing Secret)", true)
	if err != nil {
		return "", "", false, err
	}
	if !create {
		secretName, key, err = promptExistingSecret(p)
		return secretName, key, false, err
	}

	secretName = generateSecretName(gatewayName)
	fields := credentialFieldsFor(p, key)
	values, err := collectSecretValues(fields)
	if err != nil {
		return "", "", false, err
	}
	if err := createOrUpdateSecret(ctx, cl, namespace, secretName, buildSecretData(fields, values)); err != nil {
		return "", "", false, err
	}
	fmt.Fprintf(os.Stdout, "secret %q created\n", secretName)
	return secretName, key, true, nil
}

// promptExistingSecret asks for the name (and key, for single-key providers) of
// a Secret the user manages themselves.
func promptExistingSecret(p providerInfo) (secretName, key string, err error) {
	if err = runForm(inputField("Existing credential Secret name", "llm-credentials", &secretName, requiredValidator("secret name"))); err != nil {
		return "", "", err
	}
	if p.CredentialMode == credentialModeSingleKey {
		key = p.DefaultKeyName
		if err = runForm(inputField("Secret key holding the API credential", p.DefaultKeyName, &key, requiredValidator("secret key"))); err != nil {
			return "", "", err
		}
	}
	return secretName, key, nil
}

// generateSecretName derives a unique Secret name from the gateway name and a
// timestamp, so the user never has to invent one.
func generateSecretName(gateway string) string {
	return fmt.Sprintf("%s-creds-%s", gateway, time.Now().Format("20060102-150405"))
}

// createOrUpdateSecret creates the Opaque Secret, or updates its data when a
// Secret of the same name already exists.
func createOrUpdateSecret(ctx context.Context, cl client.Client, namespace, name string, data map[string]string) error {
	existing := &corev1.Secret{}
	err := cl.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, existing)
	switch {
	case apierrors.IsNotFound(err):
		secret := &corev1.Secret{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Type:       corev1.SecretTypeOpaque,
			StringData: data,
		}
		if err := cl.Create(ctx, secret); err != nil {
			return fmt.Errorf("failed to create Secret: %w", err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("failed to check for existing Secret: %w", err)
	default:
		existing.StringData = data
		if err := cl.Update(ctx, existing); err != nil {
			return fmt.Errorf("failed to update Secret: %w", err)
		}
		return nil
	}
}
