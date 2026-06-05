package config

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	hubapi "github.com/konveyor/tackle2-hub/shared/api"
	"github.com/konveyor/tackle2-hub/shared/binding/auth"
)

func TestHubIssuerURL(t *testing.T) {
	tests := []struct {
		host   string
		issuer string
	}{
		{
			host:   "https://tackle.example.com",
			issuer: "https://tackle.example.com/oidc",
		},
		{
			host:   "https://tackle.example.com/hub",
			issuer: "https://tackle.example.com/oidc",
		},
	}
	for _, tt := range tests {
		got, err := hubIssuerURL(tt.host)
		if err != nil {
			t.Fatalf("hubIssuerURL(%q) error = %v", tt.host, err)
		}
		if got != tt.issuer {
			t.Errorf("hubIssuerURL(%q) = %q, want %q", tt.host, got, tt.issuer)
		}
	}
}

func TestOnDemandOIDCHeader(t *testing.T) {
	oidc := auth.NewOIDC("http://issuer.example/oidc", hubOIDCClientID)
	wrapper := &onDemandOIDC{inner: oidc}

	if got := wrapper.Header(); got != "" {
		t.Errorf("Header() with no token = %q, want empty", got)
	}

	oidc.Use("access-token")
	if got := wrapper.Header(); got != "Bearer access-token" {
		t.Errorf("Header() with token = %q, want Bearer access-token", got)
	}
}

func TestNormalizeRepositoryURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://github.com/org/repo.git", "https://github.com/org/repo"},
		{"https://github.com/org/repo", "https://github.com/org/repo"},
		{"https://github.com/org/repo.git/", "https://github.com/org/repo"},
	}
	for _, tt := range tests {
		if got := normalizeRepositoryURL(tt.input); got != tt.want {
			t.Errorf("normalizeRepositoryURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRepositoryURLsEquivalent(t *testing.T) {
	if !repositoryURLsEquivalent("https://github.com/a/b.git", "https://github.com/a/b") {
		t.Error("expected URLs with and without .git to be equivalent")
	}
	if repositoryURLsEquivalent("https://github.com/a/b", "https://github.com/a/c") {
		t.Error("expected different repos to not be equivalent")
	}
}

func TestRepositoryURLFilter(t *testing.T) {
	f := repositoryURLFilter("https://github.com/org/app.git")
	s := f.String()
	if !strings.Contains(s, "repository.url=") {
		t.Fatalf("filter = %q, want repository.url predicate", s)
	}
	if !strings.Contains(s, "https://github.com/org/app.git") {
		t.Fatalf("filter = %q, want .git variant", s)
	}
	if !strings.Contains(s, "https://github.com/org/app") {
		t.Fatalf("filter = %q, want variant without .git", s)
	}
}

func TestRepositoryURLFilterValues_unique(t *testing.T) {
	values := repositoryURLFilterValues("https://github.com/org/app.git")
	if len(values) < 2 {
		t.Fatalf("expected multiple variants, got %v", values)
	}
}

func TestFilterApplicationsByRepositoryURL(t *testing.T) {
	apps := []hubapi.Application{
		{Repository: &hubapi.Repository{URL: "https://github.com/org/one"}},
		{Repository: &hubapi.Repository{URL: "https://github.com/org/two.git"}},
		{Binary: "only-binary"},
	}
	matched := filterApplicationsByRepositoryURL(apps, "https://github.com/org/one.git")
	if len(matched) != 1 {
		t.Fatalf("matched %d apps, want 1", len(matched))
	}
	if matched[0].Repository.URL != "https://github.com/org/one" {
		t.Errorf("matched URL = %q", matched[0].Repository.URL)
	}
}

func TestFindApplicationsByRepositoryURL_fallbackList(t *testing.T) {
	wantURL := "https://github.com/org/book-server"
	var filteredRequest bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != hubAPIPath(hubapi.ApplicationsRoute) {
			t.Errorf("path = %q, want %q", r.URL.Path, hubAPIPath(hubapi.ApplicationsRoute))
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if filter := r.URL.Query().Get("filter"); filter != "" {
			filteredRequest = true
			_ = json.NewEncoder(w).Encode([]hubapi.Application{})
			return
		}
		apps := []hubapi.Application{
			{
				Resource:   hubapi.Resource{ID: 42},
				Name:       "book-server",
				Repository: &hubapi.Repository{URL: wantURL},
			},
		}
		_ = json.NewEncoder(w).Encode(apps)
	}))
	defer server.Close()

	hc, err := newHubClient(server.URL, testPAT, false)
	if err != nil {
		t.Fatalf("newHubClient() error = %v", err)
	}

	matched, err := hc.findApplicationsByRepositoryURL(wantURL + ".git")
	if err != nil {
		t.Fatalf("findApplicationsByRepositoryURL() error = %v", err)
	}
	if !filteredRequest {
		t.Fatal("expected filtered applications request before fallback list")
	}
	if len(matched) != 1 || matched[0].ID != 42 {
		t.Fatalf("matched = %+v, want app ID 42", matched)
	}
}

func TestValidateHubToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != hubAPIPath(hubapi.UsersRoute) {
			t.Errorf("path = %q, want %q", r.URL.Path, hubAPIPath(hubapi.UsersRoute))
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer "+testPAT {
			t.Errorf("Authorization = %q", auth)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]hubapi.User{})
	}))
	defer server.Close()

	if err := validateHubToken(server.URL, testPAT, false); err != nil {
		t.Fatalf("validateHubToken() error = %v", err)
	}
}

func TestNewHubClient_normalizesHost(t *testing.T) {
	hc, err := newHubClient("https://hub.example.com/", testPAT, true)
	if err != nil {
		t.Fatalf("newHubClient() error = %v", err)
	}
	if hc.host != "https://hub.example.com" {
		t.Errorf("host = %q", hc.host)
	}
	tr := hc.binding.Client.Transport()
	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("expected insecure TLS when insecure=true")
	}
}

func TestNewHubBindingClientWithOIDC(t *testing.T) {
	client, oidc, err := newHubBindingClientWithOIDC("https://tackle.example.com", true)
	if err != nil {
		t.Fatalf("newHubBindingClientWithOIDC() error = %v", err)
	}
	if client == nil || oidc == nil {
		t.Fatal("expected client and oidc authenticator")
	}
	if got := (&onDemandOIDC{inner: oidc}).Header(); got != "" {
		t.Errorf("Header() before login = %q, want empty", got)
	}
}

func TestSelectSingleApplication(t *testing.T) {
	one := hubapi.Application{Resource: hubapi.Resource{ID: 1}}
	_, err := selectSingleApplication(nil, "lookup")
	if err == nil || !strings.Contains(err.Error(), "no applications found") {
		t.Fatalf("selectSingleApplication(nil) error = %v", err)
	}
	_, err = selectSingleApplication([]hubapi.Application{one, one}, "lookup")
	if err == nil || !strings.Contains(err.Error(), "multiple applications") {
		t.Fatalf("selectSingleApplication(2) error = %v", err)
	}
	got, err := selectSingleApplication([]hubapi.Application{one}, "lookup")
	if err != nil || got.ID != 1 {
		t.Fatalf("selectSingleApplication(1) = %v, %v", got, err)
	}
}

func TestFilterApplicationsByBinary(t *testing.T) {
	apps := []hubapi.Application{
		{Binary: "app-a"},
		{Binary: "app-b"},
	}
	matched := filterApplicationsByBinary(apps, "app-b")
	if len(matched) != 1 || matched[0].Binary != "app-b" {
		t.Fatalf("matched = %+v", matched)
	}
}

func TestTrimTarExtension(t *testing.T) {
	if got := trimTarExtension("/tmp/bundle.tar"); got != "/tmp/bundle" {
		t.Errorf("trimTarExtension() = %q", got)
	}
	if got := trimTarExtension("/tmp/bundle"); got != "/tmp/bundle" {
		t.Errorf("trimTarExtension() = %q", got)
	}
}
