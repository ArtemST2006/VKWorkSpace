// Package report defines the JSON output model.
package report

// Issue is the public representation of a GitHub issue.
type Issue struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	Author    string   `json:"author"`
	Labels    []string `json:"labels"`
	CreatedAt string   `json:"created_at"`
	URL       string   `json:"url"`
}

// Repository holds the result for one requested repository.
type Repository struct {
	Repo   string  `json:"repo"`
	Error  *string `json:"error"`
	Count  int     `json:"count"`
	Issues []Issue `json:"issues"`
}

// Report is the single JSON document produced by ghreport.
type Report struct {
	GeneratedAt string       `json:"generated_at"`
	Repos       []Repository `json:"repos"`
}

// NewRepository creates an empty successful repository result.
func NewRepository(name string) Repository {
	return Repository{Repo: name, Issues: []Issue{}}
}

// Fail replaces a partial result with a repository error.
func (r *Repository) Fail(code string) {
	r.Error = &code
	r.Issues = []Issue{}
	r.Count = 0
}
