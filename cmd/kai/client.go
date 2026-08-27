package kai

import (
	"context"
	"fmt"
	"os"

	agenticv1alpha1 "github.com/konveyor/agentic-controller/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// scheme registers the core Kubernetes types (so Secret reads work) alongside
// the agentic-controller CRD types.
func scheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(agenticv1alpha1.AddToScheme(s))
	return s
}

// restConfig loads the REST configuration honoring the --kubeconfig flag, then
// $KUBECONFIG, then ~/.kube/config (standard client-go loading rules).
func (c *kaiConfig) restConfig() (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if c.kubeconfig != "" {
		loadingRules.ExplicitPath = c.kubeconfig
	}
	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	cfg, err := cc.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig (is KUBECONFIG set to a valid cluster?): %w", err)
	}
	return cfg, nil
}

// newClient constructs a typed controller-runtime client for the target cluster.
func (c *kaiConfig) newClient() (client.Client, error) {
	cfg, err := c.restConfig()
	if err != nil {
		return nil, err
	}
	cl, err := client.New(cfg, client.Options{Scheme: scheme()})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}
	return cl, nil
}

// newClientset constructs a client-go Clientset, used for subresources the
// typed controller-runtime client cannot serve (notably pods/log).
func (c *kaiConfig) newClientset() (*kubernetes.Clientset, error) {
	cfg, err := c.restConfig()
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}
	return cs, nil
}

// deleteResource deletes obj after an optional confirmation prompt. kind and
// name are used only for user-facing messages.
func deleteResource(ctx context.Context, cl client.Client, obj client.Object, name, kind string, yes bool) error {
	if !yes {
		ok, err := confirm(fmt.Sprintf("Delete %s %q in namespace %q?", kind, name, obj.GetNamespace()), false)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(os.Stdout, "aborted")
			return nil
		}
	}
	if err := cl.Delete(ctx, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%s %q not found in namespace %q", kind, name, obj.GetNamespace())
		}
		return fmt.Errorf("failed to delete %s: %w", kind, err)
	}
	fmt.Fprintf(os.Stdout, "%s %q deleted\n", kind, name)
	return nil
}
