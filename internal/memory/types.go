package memory

import "context"

const (
	CategoryPreference = "preference"
	CategoryProject    = "project"
	CategoryPattern    = "pattern"
	CategoryDecision   = "decision"
	CategoryFact       = "fact"
)

var ValidCategories = map[string]bool{
	CategoryPreference: true,
	CategoryProject:    true,
	CategoryPattern:    true,
	CategoryDecision:   true,
	CategoryFact:       true,
}

type Memory struct {
	ID        string   `json:"id"`
	Content   string   `json:"content"`
	Category  string   `json:"category"`
	Tags      []string `json:"tags"`
	Source    string   `json:"source,omitempty"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

type StoreRequest struct {
	Content  string   `json:"content"`
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
	Source   string   `json:"source"`
}

type SearchRequest struct {
	Query    string   `json:"query"`
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
	Limit    int      `json:"limit"`
}

type SearchResult struct {
	Memory   Memory  `json:"memory"`
	Distance float32 `json:"distance"`
}

type ListRequest struct {
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
	Limit    int      `json:"limit"`
	Offset   int      `json:"offset"`
}

type UpdateRequest struct {
	ID       string   `json:"id"`
	Content  *string  `json:"content,omitempty"`
	Category *string  `json:"category,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

type Store interface {
	Add(ctx context.Context, mem Memory) error
	Search(ctx context.Context, query string, category string, tags []string, limit int) ([]SearchResult, error)
	List(ctx context.Context, category string, tags []string, limit, offset int) ([]Memory, error)
	Get(ctx context.Context, id string) (*Memory, error)
	Update(ctx context.Context, mem Memory) error
	Delete(ctx context.Context, id string) error
	Count(ctx context.Context) (int, error)
	Healthy(ctx context.Context) error
}
