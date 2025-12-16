package profile

// an application in the Hub
type Application struct {
	ID         uint        `json:"id" yaml:"id"`
	Name       string      `json:"name" yaml:"name"`
	Repository *Repository `json:"repository,omitempty" yaml:"repository,omitempty"`
}

type Resource struct {
	ID uint `json:"id" yaml:"id"`
}

type Repository struct {
	URL    string `json:"url" yaml:"url"`
	Branch string `json:"branch,omitempty" yaml:"branch,omitempty"`
}
