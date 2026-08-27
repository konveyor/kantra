package kai

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// editResource fetches obj, opens its YAML representation in the user's $EDITOR
// (kubectl-edit style), and applies the edited result. obj must be a pointer to
// a typed resource; it is populated in place. A no-op edit is reported and no
// update is issued.
func editResource(ctx context.Context, cl client.Client, obj client.Object) error {
	key := client.ObjectKeyFromObject(obj)
	if err := cl.Get(ctx, key, obj); err != nil {
		return fmt.Errorf("failed to get %s/%s: %w", key.Namespace, key.Name, err)
	}

	// Strip server-managed noise so the editable document is clean.
	obj.SetManagedFields(nil)

	original, err := yaml.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to marshal resource to YAML: %w", err)
	}

	edited, err := openInEditor(original)
	if err != nil {
		return err
	}

	if bytes.Equal(bytes.TrimSpace(original), bytes.TrimSpace(edited)) {
		fmt.Fprintf(os.Stdout, "no changes made to %s\n", key.Name)
		return nil
	}

	if err := yaml.Unmarshal(edited, obj); err != nil {
		return fmt.Errorf("edited YAML is invalid: %w", err)
	}

	if err := cl.Update(ctx, obj); err != nil {
		return fmt.Errorf("failed to apply changes: %w", err)
	}
	fmt.Fprintf(os.Stdout, "%s updated\n", key.Name)
	return nil
}

// openInEditor writes content to a temporary file, opens it in $EDITOR (falling
// back to vi), and returns the edited content.
func openInEditor(content []byte) ([]byte, error) {
	f, err := os.CreateTemp("", "kantra-kai-*.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	name := f.Name()
	defer os.Remove(name)

	if _, err := f.Write(content); err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, err
	}

	editor := editorCommand()
	parts := strings.Fields(editor)
	parts = append(parts, name)
	cmd := exec.Command(parts[0], parts[1:]...) //nolint:gosec // user's own $EDITOR
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("editor exited with error: %w", err)
	}

	return os.ReadFile(name)
}

// editorCommand resolves the editor to use, honoring $KUBE_EDITOR then $EDITOR
// then falling back to vi.
func editorCommand() string {
	for _, env := range []string{"KUBE_EDITOR", "EDITOR"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return v
		}
	}
	return "vi"
}
