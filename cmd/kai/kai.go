package kai

import (
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
)

// defaultNamespace is the conventional Konveyor install namespace where the
// agentic-controller CRDs are created and looked up.
const defaultNamespace = "konveyor-tackle"

// kaiConfig holds the shared configuration for every kai subcommand. It is
// created once by NewKaiCommand and threaded down to each command group so that
// the persistent --namespace/--kubeconfig flags resolve to a single place.
type kaiConfig struct {
	log        logr.Logger
	namespace  string
	kubeconfig string
}

// NewKaiCommand builds the "kai" command group used to manage the Konveyor
// agentic CRDs (Gateways, Agents, Workflows and Skills) on a cluster the user
// is already authenticated to via KUBECONFIG.
func NewKaiCommand(log logr.Logger) *cobra.Command {
	cfg := &kaiConfig{log: log}

	cmd := &cobra.Command{
		Use:   "kai",
		Short: "Manage Konveyor agentic resources (agents, gateways, workflows, skills)",
		Long: `Manage the Konveyor agentic CRDs installed by the agentic-controller.

Commands operate against the cluster referenced by your KUBECONFIG and default
to the "` + defaultNamespace + `" namespace (override with --namespace).`,
	}

	cmd.PersistentFlags().StringVarP(&cfg.namespace, "namespace", "n", defaultNamespace,
		"namespace to operate in")
	cmd.PersistentFlags().StringVar(&cfg.kubeconfig, "kubeconfig", "",
		"path to the kubeconfig file (defaults to $KUBECONFIG then ~/.kube/config)")

	cmd.AddCommand(newGatewayCommand(cfg))
	cmd.AddCommand(newAgentCommand(cfg))
	cmd.AddCommand(newWorkflowCommand(cfg))
	cmd.AddCommand(newSkillCommand(cfg))

	return cmd
}
