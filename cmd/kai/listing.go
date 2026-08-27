package kai

import (
	"context"

	agenticv1alpha1 "github.com/konveyor/agentic-controller/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// resourceNames returns the names of all objects in list within the namespace.
// list must be a pointer to a typed *List whose Items expose ObjectMeta.
func gatewayNames(ctx context.Context, cl client.Client, namespace string) ([]string, error) {
	var l agenticv1alpha1.GatewayList
	if err := cl.List(ctx, &l, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(l.Items))
	for i := range l.Items {
		names = append(names, l.Items[i].Name)
	}
	return names, nil
}

func agentNames(ctx context.Context, cl client.Client, namespace string) ([]string, error) {
	var l agenticv1alpha1.AgentList
	if err := cl.List(ctx, &l, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(l.Items))
	for i := range l.Items {
		names = append(names, l.Items[i].Name)
	}
	return names, nil
}

func skillCardNames(ctx context.Context, cl client.Client, namespace string) ([]string, error) {
	var l agenticv1alpha1.SkillCardList
	if err := cl.List(ctx, &l, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(l.Items))
	for i := range l.Items {
		names = append(names, l.Items[i].Name)
	}
	return names, nil
}

func skillCollectionNames(ctx context.Context, cl client.Client, namespace string) ([]string, error) {
	var l agenticv1alpha1.SkillCollectionList
	if err := cl.List(ctx, &l, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(l.Items))
	for i := range l.Items {
		names = append(names, l.Items[i].Name)
	}
	return names, nil
}
