package cloud_foundry

// From https://github.com/cloudfoundry/korifi/blob/main/api/presenter/info.go#L10

type InfoV3Response struct {
	Build       string                 `json:"build"`
	CLIVersion  InfoCLIVersion         `json:"cli_version"`
	Description string                 `json:"description"`
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Custom      map[string]interface{} `json:"custom"`

	Links map[string]Link `json:"links"`
}

type InfoCLIVersion struct {
	Minimum     string `json:"minimum"`
	Recommended string `json:"recommended"`
}

type Link struct {
	HRef   string `json:"href,omitempty"`
	Method string `json:"method,omitempty"`
}

// -----------------------------------------------
type Lifecycle struct {
	// The CF Lifecycle type.
	// Only "buildpack" and "docker" are currently allowed
	Type LifecycleType `json:"type"`
	// Data used to specify details for the Lifecycle
	Data LifecycleData `json:"data"`
}

// LifecycleType inform the platform of how to build droplets and run apps
// allow only values "buildpack" or "docker"
// +kubebuilder:validation:Enum=buildpack;docker
type LifecycleType string

// LifecycleData is shared by CFApp and CFBuild
type LifecycleData struct {
	// Buildpacks to include in auto-detection when building the app image.
	// If no values are specified, then all available buildpacks will be used for auto-detection
	Buildpacks []string `json:"buildpacks,omitempty"`

	// Stack to use when building the app image
	Stack string `json:"stack"`
}

type Metadata struct {
	Annotations map[string]string `json:"annotations" yaml:"annotations"`
	Labels      map[string]string `json:"labels"      yaml:"labels"`
}

type PageRef struct {
	HREF string `json:"href"`
}

type ListResponse[T any] struct {
	PaginationData PaginationData   `json:"pagination"`
	Resources      []T              `json:"resources"`
	Included       map[string][]any `json:"included,omitempty"`
}

type PaginationData struct {
	TotalResults int     `json:"total_results"`
	TotalPages   int     `json:"total_pages"`
	First        PageRef `json:"first"`
	Last         PageRef `json:"last"`
	Next         *int    `json:"next"`
	Previous     *int    `json:"previous"`
}

type AppResponse struct {
	Name  string `json:"name"`
	GUID  string `json:"guid"`
	State string `json:"state"`

	CreatedAt     string                       `json:"created_at"`
	UpdatedAt     string                       `json:"updated_at"`
	Relationships map[string]ToOneRelationship `json:"relationships"`
	Lifecycle     Lifecycle                    `json:"lifecycle"`
	Metadata      Metadata                     `json:"metadata"`
	Links         AppLinks                     `json:"links"`
}

type AppLinks struct {
	Self                 Link `json:"self"`
	Space                Link `json:"space"`
	Processes            Link `json:"processes"`
	Packages             Link `json:"packages"`
	EnvironmentVariables Link `json:"environment_variables"`
	CurrentDroplet       Link `json:"current_droplet"`
	Droplets             Link `json:"droplets"`
	Tasks                Link `json:"tasks"`
	StartAction          Link `json:"start"`
	StopAction           Link `json:"stop"`
	Revisions            Link `json:"revisions"`
	DeployedRevisions    Link `json:"deployed_revisions"`
	Features             Link `json:"features"`
}

type Relationship struct {
	GUID string `json:"guid"`
}

type ToOneRelationship struct {
	Data Relationship `json:"data"`
}

// ---------------------

type AppEnvResponse struct {
	EnvironmentVariables map[string]string `json:"environment_variables"`
	StagingEnvJSON       map[string]string `json:"staging_env_json"`
	RunningEnvJSON       map[string]string `json:"running_env_json"`
	SystemEnvJSON        map[string]any    `json:"system_env_json"`
	ApplicationEnvJSON   map[string]any    `json:"application_env_json"`
}

type VCAPApplicationEnv struct {
	ApplicationId    string `json:"application_id"`
	ApplicationName  string `json:"application_name"`
	Name             string `json:"name"`
	OrganizationId   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
	SpaceId          string `json:"space_id"`
	SpaceName        string `json:"space_name"`
	URIs             string `json:"uris"`
	ApplicationURIs  string `json:"application_uris"`
}
