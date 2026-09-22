package kai

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	agenticv1alpha1 "github.com/konveyor/agentic-controller/api/v1alpha1"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func newGatewayCommand(cfg *kaiConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "gateway",
		Aliases: []string{"gw", "gateways"},
		Short:   "Manage LLM Gateways",
	}
	cmd.AddCommand(newGatewayCreateCommand(cfg))
	cmd.AddCommand(newGatewayEditCommand(cfg))
	cmd.AddCommand(newGatewayDeleteCommand(cfg))
	cmd.AddCommand(newGatewayListCommand(cfg))
	cmd.AddCommand(newGatewayGetCommand(cfg))
	cmd.AddCommand(newGatewayDescribeCommand(cfg))
	return cmd
}

func newGatewayCreateCommand(cfg *kaiConfig) *cobra.Command {
	var dryRun bool
	var contextWindow int64
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a Gateway via an interactive, provider-validated wizard",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return runGatewayCreate(cmd.Context(), cfg, name, dryRun, contextWindow)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the Gateway YAML without creating it")
	cmd.Flags().Int64Var(&contextWindow, "context-window", 0, "model context window in tokens (0 = use the model's default)")
	return cmd
}

func runGatewayCreate(ctx context.Context, cfg *kaiConfig, name string, dryRun bool, contextWindow int64) error {
	if err := requireTerminal(); err != nil {
		return err
	}
	cl, err := cfg.newClient()
	if err != nil {
		return err
	}

	// Stage 1: name (if not supplied) and provider selection.
	provider := supportedProviders[0].ID
	nameVal := name
	if name == "" {
		if err := runForm(
			inputField("Gateway name", "my-anthropic-gw", &nameVal, requiredValidator("name")),
			selectField("Provider", providerIDs(), &provider),
		); err != nil {
			return err
		}
	} else {
		if err := runForm(
			selectField("Provider", providerIDs(), &provider),
		); err != nil {
			return err
		}
	}
	name = strings.TrimSpace(nameVal)

	p, ok := lookupProvider(provider)
	if !ok {
		return fmt.Errorf("unsupported provider %q", provider)
	}

	// Stage 2: endpoint and model only. Context window is not prompted (derived
	// from the model), and credentials come afterward so the common path can be
	// as simple as "type your API key".
	endpoint := p.DefaultEndpoint
	modelName := ""
	tier := ""

	stage2 := []huh.Field{
		inputField("Endpoint URL", p.DefaultEndpoint, &endpoint, validateEndpoint),
		inputField("Model name", "claude-sonnet-4-5-20250929", &modelName, requiredValidator("model name")),
		inputField("Model tier (optional)", "premium", &tier, nil),
	}
	if err := runForm(stage2...); err != nil {
		return err
	}

	// Resolve the context window: explicit flag wins, otherwise the model's
	// known default, otherwise a plausible fallback (with a note).
	cw := contextWindow
	if cw <= 0 {
		if d, ok := defaultContextWindow(modelName); ok {
			cw = d
		} else {
			cw = fallbackContextWindow
			fmt.Fprintf(os.Stderr, "note: unknown model %q; defaulting context window to %d (override with --context-window)\n", strings.TrimSpace(modelName), cw)
		}
	}

	if err := validateProvider(p.ID); err != nil {
		return err
	}
	if err := validateEndpoint(endpoint); err != nil {
		return err
	}

	// Credentials: by default create a Secret inline (just prompt for the
	// values, auto-name the Secret, use the provider's default key). The user
	// can instead reference a Secret they created themselves.
	secretName, key, secretCreated, err := resolveCredentials(ctx, cl, cfg.namespace, name, p, dryRun)
	if err != nil {
		return err
	}
	if err := validateCredentialRef(p, secretName, key); err != nil {
		return err
	}
	if !secretCreated {
		warnAboutSecret(ctx, cl, cfg.namespace, p, secretName)
	}

	gw := &agenticv1alpha1.Gateway{
		TypeMeta: metav1.TypeMeta{APIVersion: agenticv1alpha1.GroupVersion.String(), Kind: "Gateway"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cfg.namespace,
		},
		Spec: agenticv1alpha1.GatewaySpec{
			Provider: p.ID,
			Endpoint: strings.TrimSpace(endpoint),
			CredentialRef: agenticv1alpha1.GatewayCredentialRef{
				SecretName: strings.TrimSpace(secretName),
				Key:        strings.TrimSpace(key),
			},
			Model: agenticv1alpha1.GatewayModel{
				Name:          strings.TrimSpace(modelName),
				ContextWindow: cw,
				Tier:          strings.TrimSpace(tier),
			},
		},
	}

	data, err := yaml.Marshal(gw)
	if err != nil {
		return err
	}
	if dryRun {
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}

	fmt.Fprintln(os.Stdout, "\n"+string(data))
	proceed, err := confirm(fmt.Sprintf("Create Gateway %q in namespace %q?", name, cfg.namespace), true)
	if err != nil {
		return err
	}
	if !proceed {
		fmt.Fprintln(os.Stdout, "aborted")
		return nil
	}

	if err := cl.Create(ctx, gw); err != nil {
		return fmt.Errorf("failed to create Gateway: %w", err)
	}
	fmt.Fprintf(os.Stdout, "gateway %q created\n", name)
	return nil
}

// warnAboutSecret checks whether the credential Secret exists and contains the
// keys the provider expects, printing warnings but never failing.
func warnAboutSecret(ctx context.Context, cl client.Client, namespace string, p providerInfo, secretName string) {
	if secretName == "" {
		return
	}
	secret := &corev1.Secret{}
	err := cl.Get(ctx, client.ObjectKey{Namespace: namespace, Name: secretName}, secret)
	if apierrors.IsNotFound(err) {
		fmt.Fprintf(os.Stderr, "warning: Secret %q not found in namespace %q; create it before running agents that use this gateway\n", secretName, namespace)
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not verify Secret %q: %v\n", secretName, err)
		return
	}
	if missing := missingSecretKeys(p, secret.Data); len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "warning: Secret %q is missing expected keys for provider %q: %s\n",
			secretName, p.ID, strings.Join(missing, ", "))
	}
}

func newGatewayEditCommand(cfg *kaiConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit a Gateway in your $EDITOR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := cfg.newClient()
			if err != nil {
				return err
			}
			gw := &agenticv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: args[0], Namespace: cfg.namespace},
			}
			return editResource(cmd.Context(), cl, gw)
		},
	}
}

func newGatewayDeleteCommand(cfg *kaiConfig) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a Gateway",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := cfg.newClient()
			if err != nil {
				return err
			}
			gw := &agenticv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: args[0], Namespace: cfg.namespace},
			}
			return deleteResource(cmd.Context(), cl, gw, args[0], "gateway", yes)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

func newGatewayListCommand(cfg *kaiConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List Gateways",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := cfg.newClient()
			if err != nil {
				return err
			}
			var list agenticv1alpha1.GatewayList
			if err := cl.List(cmd.Context(), &list, client.InNamespace(cfg.namespace)); err != nil {
				return fmt.Errorf("failed to list gateways: %w", err)
			}
			if len(list.Items) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "no gateways found in namespace %q\n", cfg.namespace)
				return nil
			}
			rows := make([][]string, 0, len(list.Items))
			for i := range list.Items {
				gw := &list.Items[i]
				rows = append(rows, []string{
					gw.Name,
					gw.Spec.Provider,
					gw.Spec.Model.Name,
					strconv.FormatBool(gw.Status.ConnectionVerified),
					conditionStatus(gw.Status.Conditions, "Ready"),
					age(gw.CreationTimestamp),
				})
			}
			table(cmd.OutOrStdout(), []string{"NAME", "PROVIDER", "MODEL", "VERIFIED", "READY", "AGE"}, rows)
			return nil
		},
	}
}
