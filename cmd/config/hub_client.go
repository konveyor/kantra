package config

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-logr/logr"
	hubapi "github.com/konveyor/tackle2-hub/shared/api"
	"github.com/konveyor/tackle2-hub/shared/binding"
	"github.com/konveyor/tackle2-hub/shared/binding/auth"
	"github.com/konveyor/tackle2-hub/shared/binding/client"
	hubfilter "github.com/konveyor/tackle2-hub/shared/binding/filter"
)

const hubOIDCClientID = "kantra"

type hubClient struct {
	binding *binding.RichClient
	host    string
}

// defers sending Authorization until after Login()
type onDemandOIDC struct {
	inner *auth.OIDC
}

func (o *onDemandOIDC) Login() error {
	return o.inner.Login()
}

func (o *onDemandOIDC) Header() string {
	if o.inner.Token() == "" {
		return ""
	}
	return o.inner.Header()
}

func (o *onDemandOIDC) SetTransport(tr *http.Transport) {
	o.inner.SetTransport(tr)
}

func newHubBindingClient(tackleBase, token string, insecure bool) (*binding.RichClient, error) {
	apiURL, err := hubAPIURL(tackleBase)
	if err != nil {
		return nil, err
	}
	client := binding.New(apiURL)
	if token != "" {
		client.Client.Use(auth.NewBearer(token))
	}
	applyInsecureTransport(client, insecure)
	return client, nil
}

func newHubBindingClientWithOIDC(tackleBase string, insecure bool) (*binding.RichClient, *auth.OIDC, error) {
	oidc, err := newHubOIDCAuthenticator(tackleBase, insecure)
	if err != nil {
		return nil, nil, err
	}
	client, err := newHubBindingClient(tackleBase, "", insecure)
	if err != nil {
		return nil, nil, err
	}
	client.Client.Use(&onDemandOIDC{inner: oidc})
	return client, oidc, nil
}

func newHubOIDCAuthenticator(tackleBase string, insecure bool) (*auth.OIDC, error) {
	issuer, err := hubIssuerURL(tackleBase)
	if err != nil {
		return nil, err
	}
	oidc := auth.NewOIDC(issuer, hubOIDCClientID)
	applyInsecureOIDCTransport(oidc, insecure)
	return oidc, nil
}

func hubIssuerURL(host string) (string, error) {
	base, err := normalizeTackleBaseURL(host)
	if err != nil {
		return "", err
	}
	return url.JoinPath(base, "oidc")
}

func applyInsecureTransport(client *binding.RichClient, insecure bool) {
	if !insecure {
		return
	}
	client.Client.SetTransport(insecureTransport())
}

func applyInsecureOIDCTransport(oidc *auth.OIDC, insecure bool) {
	if !insecure {
		return
	}
	oidc.SetTransport(insecureTransport())
}

func insecureTransport() *http.Transport {
	return &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
}

func validateHubToken(host, token string, insecure bool) error {
	client, err := newHubBindingClient(host, token, insecure)
	if err != nil {
		return err
	}
	_, err = client.User.List()
	return err
}

func newHubClient(host, token string, insecure bool) (*hubClient, error) {
	base, err := normalizeTackleBaseURL(host)
	if err != nil {
		return nil, err
	}
	client, err := newHubBindingClient(base, token, insecure)
	if err != nil {
		return nil, err
	}
	return &hubClient{
		binding: client,
		host:    base,
	}, nil
}

func newHubClientFromAuth(insecure bool) (*hubClient, error) {
	auth, err := loadStoredAuth()
	if err != nil {
		return nil, err
	}
	return newHubClient(auth.Host, auth.Token, insecure)
}

func newHubClientNoAuth(host string, insecure bool) (*hubClient, error) {
	base, err := normalizeTackleBaseURL(host)
	if err != nil {
		return nil, err
	}
	client, _, err := newHubBindingClientWithOIDC(base, insecure)
	if err != nil {
		return nil, err
	}
	return &hubClient{
		binding: client,
		host:    base,
	}, nil
}

func (hc *hubClient) listApplications(filter hubfilter.Filter) ([]hubapi.Application, error) {
	apps := []hubapi.Application{}
	var params []client.Param
	if filter.String() != "" {
		f := client.Filter{Filter: filter}
		params = append(params, f.Param())
	}
	err := hc.binding.Client.Get(hubapi.ApplicationsRoute, &apps, params...)
	if err != nil {
		return nil, err
	}
	return apps, nil
}

func (hc *hubClient) listApplicationProfiles(appID uint) ([]hubapi.AnalysisProfile, error) {
	path := client.Path(hubapi.AppAnalysisProfilesRoute).Inject(client.Params{hubapi.ID: appID})
	profiles := []hubapi.AnalysisProfile{}
	if err := hc.binding.Client.Get(path, &profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}

func (hc *hubClient) downloadProfileBundle(profileID uint, tarPath string, log logr.Logger) error {
	if err := hc.binding.AnalysisProfile.GetBundle(profileID, tarPath); err != nil {
		return err
	}
	log.V(7).Info("profile bundle downloaded", "path", tarPath)

	extractDir := trimTarExtension(tarPath)
	if err := extractTarFile(tarPath, extractDir, log); err != nil {
		return err
	}
	if err := deleteTarFile(tarPath); err != nil {
		log.Error(err, "failed to delete tar file after extraction", "path", tarPath)
	}
	log.Info("profile bundle extracted successfully", "path", extractDir)
	return nil
}

func trimTarExtension(path string) string {
	if len(path) > 4 && path[len(path)-4:] == ".tar" {
		return path[:len(path)-4]
	}
	return path
}

func normalizeRepositoryURL(u string) string {
	u = strings.TrimSpace(u)
	u = strings.TrimSuffix(u, "/")
	if strings.HasSuffix(strings.ToLower(u), ".git") {
		u = u[:len(u)-4]
	}
	return u
}

func repositoryURLsEquivalent(a, b string) bool {
	return normalizeRepositoryURL(a) == normalizeRepositoryURL(b)
}

func repositoryURLFilter(url string) hubfilter.Filter {
	variants := repositoryURLFilterValues(url)
	var f hubfilter.Filter
	if len(variants) == 1 {
		f.And("repository.url").Eq(variants[0])
		return f
	}
	values := make([]any, len(variants))
	for i, v := range variants {
		values[i] = v
	}
	f.And("repository.url").Eq(hubfilter.Any(values))
	return f
}

func repositoryURLFilterValues(url string) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(u string) {
		u = strings.TrimSpace(u)
		u = strings.TrimSuffix(u, "/")
		if u == "" {
			return
		}
		if _, ok := seen[u]; ok {
			return
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	add(url)
	norm := normalizeRepositoryURL(url)
	add(norm)
	if norm != "" {
		add(norm + ".git")
	}
	return out
}

func filterApplicationsByRepositoryURL(apps []hubapi.Application, repoURL string) []hubapi.Application {
	var matched []hubapi.Application
	for _, app := range apps {
		if app.Repository == nil {
			continue
		}
		if repositoryURLsEquivalent(app.Repository.URL, repoURL) {
			matched = append(matched, app)
		}
	}
	return matched
}

func (hc *hubClient) findApplicationsByRepositoryURL(repoURL string) ([]hubapi.Application, error) {
	apps, err := hc.listApplications(repositoryURLFilter(repoURL))
	if err != nil {
		return nil, err
	}
	if matched := filterApplicationsByRepositoryURL(apps, repoURL); len(matched) > 0 {
		return matched, nil
	}
	all, err := hc.listApplications(hubfilter.Filter{})
	if err != nil {
		return nil, err
	}
	return filterApplicationsByRepositoryURL(all, repoURL), nil
}

func filterApplicationsByBinary(apps []hubapi.Application, binary string) []hubapi.Application {
	var matched []hubapi.Application
	for _, app := range apps {
		if app.Binary == binary {
			matched = append(matched, app)
		}
	}
	return matched
}

func selectSingleApplication(apps []hubapi.Application, lookup string) (hubapi.Application, error) {
	if len(apps) == 0 {
		return hubapi.Application{}, fmt.Errorf("no applications found in Hub for given input")
	}
	if len(apps) > 1 {
		return hubapi.Application{}, fmt.Errorf("multiple applications found in Hub: %s", lookup)
	}
	return apps[0], nil
}
