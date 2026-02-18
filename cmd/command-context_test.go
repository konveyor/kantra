package cmd

import (
	"testing"

	"github.com/devfile/alizer/pkg/apis/model"
	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
)

func Test_AnalyzeCommandContext_setProviders_MultipleProviders(t *testing.T) {
	ctx := &AnalyzeCommandContext{
		log:          logr.Discard(),
		providersMap: make(map[string]ProviderInit),
	}

	providers := []string{util.JavaProvider, util.GoProvider, util.PythonProvider}
	languages := []model.Language{} // Empty, should use providers list
	foundProviders := []string{}

	result, err := ctx.setProviders(providers, languages, foundProviders)
	if err != nil {
		t.Fatalf("setProviders() error = %v", err)
	}

	// Should return all 3 providers
	if len(result) != 3 {
		t.Errorf("setProviders() returned %d providers, want 3", len(result))
	}

	// Verify all providers are present
	providerMap := make(map[string]bool)
	for _, p := range result {
		providerMap[p] = true
	}

	for _, expected := range providers {
		if !providerMap[expected] {
			t.Errorf("setProviders() missing provider %s", expected)
		}
	}
}

func Test_AnalyzeCommandContext_setProviders_SingleProvider(t *testing.T) {
	ctx := &AnalyzeCommandContext{
		log:          logr.Discard(),
		providersMap: make(map[string]ProviderInit),
	}

	providers := []string{util.JavaProvider}
	languages := []model.Language{}
	foundProviders := []string{}

	result, err := ctx.setProviders(providers, languages, foundProviders)
	if err != nil {
		t.Fatalf("setProviders() error = %v", err)
	}

	if len(result) != 1 {
		t.Errorf("setProviders() returned %d providers, want 1", len(result))
	}

	if result[0] != util.JavaProvider {
		t.Errorf("setProviders() returned provider %s, want %s", result[0], util.JavaProvider)
	}
}

func Test_AnalyzeCommandContext_setProviders_EmptyProvidersUsesLanguages(t *testing.T) {
	ctx := &AnalyzeCommandContext{
		log:          logr.Discard(),
		providersMap: make(map[string]ProviderInit),
	}

	providers := []string{} // Empty, should detect from languages
	languages := []model.Language{
		{
			Name:           "Java",
			CanBeComponent: true,
			Weight:         0.2,
		},
		{
			Name:           "Go",
			CanBeComponent: true,
			Weight:         0.01,
		},
	}
	foundProviders := []string{}

	result, err := ctx.setProviders(providers, languages, foundProviders)
	if err != nil {
		t.Fatalf("setProviders() error = %v", err)
	}

	// Should auto-detect java and go from languages
	if len(result) != 2 {
		t.Errorf("setProviders() returned %d providers, want 2", len(result))
	}

	providerMap := make(map[string]bool)
	for _, p := range result {
		providerMap[p] = true
	}

	if !providerMap["java"] {
		t.Error("setProviders() missing java provider from language detection")
	}
	if !providerMap["go"] {
		t.Error("setProviders() missing go provider from language detection")
	}
}

func Test_AnalyzeCommandContext_setProviders_NonDefaultLanguage(t *testing.T) {
	ctx := &AnalyzeCommandContext{
		log:          logr.Discard(),
		providersMap: make(map[string]ProviderInit),
	}

	providers := []string{}
	languages := []model.Language{
		{
			Name:           "Rust",
			CanBeComponent: true,
			Weight:         .3,
		},
		{
			Name:           "PHP",
			CanBeComponent: true,
			Weight:         .03,
		},
	}
	foundProviders := []string{}

	result, err := ctx.setProviders(providers, languages, foundProviders)
	if err != nil {
		t.Fatalf("setProviders() error = %v", err)
	}

	// Rust should be the only provider
	if len(result) != 1 {
		t.Errorf("setProviders() returned %d providers, want 1", len(result))
	}

	if result[0] != "rust" {
		t.Errorf("setProviders() returned provider %s for non default Rust, want %s",
			result[0], "Rust")
	}
}

func Test_AnalyzeCommandContext_setProviders_JavaScriptDetection(t *testing.T) {
	ctx := &AnalyzeCommandContext{
		log:          logr.Discard(),
		providersMap: make(map[string]ProviderInit),
	}

	providers := []string{}
	languages := []model.Language{
		{
			Name:           "JavaScript",
			CanBeComponent: true,
		},
	}
	foundProviders := []string{}

	result, err := ctx.setProviders(providers, languages, foundProviders)
	if err != nil {
		t.Fatalf("setProviders() error = %v", err)
	}

	// JavaScript should map to nodejs provider
	if len(result) != 1 {
		t.Errorf("setProviders() returned %d providers, want 1", len(result))
	}

	if result[0] != util.NodeJSProvider {
		t.Errorf("setProviders() returned provider %s for JavaScript, want %s",
			result[0], util.NodeJSProvider)
	}
}

func Test_AnalyzeCommandContext_setProviders_TypeScriptDetection(t *testing.T) {
	ctx := &AnalyzeCommandContext{
		log:          logr.Discard(),
		providersMap: make(map[string]ProviderInit),
	}

	providers := []string{}
	languages := []model.Language{
		{
			Name:           "TypeScript",
			CanBeComponent: true,
		},
	}
	foundProviders := []string{}

	result, err := ctx.setProviders(providers, languages, foundProviders)
	if err != nil {
		t.Fatalf("setProviders() error = %v", err)
	}

	// TypeScript should map to nodejs provider
	if len(result) != 1 {
		t.Errorf("setProviders() returned %d providers, want 1", len(result))
	}

	if result[0] != util.NodeJSProvider {
		t.Errorf("setProviders() returned provider %s for TypeScript, want %s",
			result[0], util.NodeJSProvider)
	}
}

func Test_AnalyzeCommandContext_setProviders_CSharpDetection(t *testing.T) {
	ctx := &AnalyzeCommandContext{
		log:          logr.Discard(),
		providersMap: make(map[string]ProviderInit),
	}

	providers := []string{}
	languages := []model.Language{
		{
			Name:           "C#",
			CanBeComponent: true,
		},
	}
	foundProviders := []string{}

	result, err := ctx.setProviders(providers, languages, foundProviders)
	if err != nil {
		t.Fatalf("setProviders() error = %v", err)
	}

	// C# should map to dotnet provider
	if len(result) != 1 {
		t.Errorf("setProviders() returned %d providers, want 1", len(result))
	}

	if result[0] != util.CsharpProvider {
		t.Errorf("setProviders() returned provider %s for C#, want %s",
			result[0], util.CsharpProvider)
	}
}

func Test_AnalyzeCommandContext_setProviders_PreserveFoundProviders(t *testing.T) {
	ctx := &AnalyzeCommandContext{
		log:          logr.Discard(),
		providersMap: make(map[string]ProviderInit),
	}

	providers := []string{util.GoProvider}
	languages := []model.Language{}
	foundProviders := []string{util.JavaProvider} // Already has java

	result, err := ctx.setProviders(providers, languages, foundProviders)
	if err != nil {
		t.Fatalf("setProviders() error = %v", err)
	}

	// Should have both java (from foundProviders) and go (from providers)
	if len(result) != 2 {
		t.Errorf("setProviders() returned %d providers, want 2", len(result))
	}

	providerMap := make(map[string]bool)
	for _, p := range result {
		providerMap[p] = true
	}

	if !providerMap[util.JavaProvider] {
		t.Error("setProviders() lost existing java provider from foundProviders")
	}
	if !providerMap[util.GoProvider] {
		t.Error("setProviders() missing go provider from providers list")
	}
}
