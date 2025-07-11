package cloud_foundry

import (
	pTypes "github.com/konveyor/asset-generation/pkg/providers/types/provider"
)

type mockProvider struct {
	DiscoverFunc func(RawData any) (*pTypes.DiscoverResult, error)
	ListAppsFunc func() (map[string][]any, error)
}

func (m *mockProvider) Discover(raw any) (*pTypes.DiscoverResult, error) {
	if m.DiscoverFunc != nil {
		return m.DiscoverFunc(raw)
	}
	return nil, nil
}

func (m *mockProvider) ListApps() (map[string][]any, error) {
	if m.ListAppsFunc != nil {
		return m.ListAppsFunc()
	}
	return nil, nil
}
