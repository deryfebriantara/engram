package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	store          Store
	dedupThreshold float64
}

func NewService(store Store, dedupThreshold float64) *Service {
	return &Service{store: store, dedupThreshold: dedupThreshold}
}

func (s *Service) Store(ctx context.Context, req StoreRequest) (*StoreOutcome, error) {
	if req.Content == "" {
		return nil, fmt.Errorf("content is required")
	}

	if req.Category == "" {
		req.Category = CategoryFact
	}
	if !ValidCategories[req.Category] {
		return nil, fmt.Errorf("invalid category: %s", req.Category)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if s.dedupThreshold > 0 {
		// Best-effort: the new memory doesn't exist yet, so there's no
		// self-match risk. If the dedup lookup fails, fall through to a
		// normal insert rather than blocking the store on it.
		if results, err := s.store.Search(ctx, req.Content, "", nil, 1); err == nil && len(results) > 0 {
			top := results[0]
			if float64(top.Distance) <= s.dedupThreshold {
				mem := top.Memory
				mem.Content = req.Content
				mem.Category = req.Category
				mem.Tags = unionTags(mem.Tags, req.Tags)
				if req.Source != "" {
					mem.Source = req.Source
				}
				mem.UpdatedAt = now

				if err := s.store.Update(ctx, mem); err != nil {
					return nil, fmt.Errorf("update memory: %w", err)
				}
				return &StoreOutcome{Memory: &mem, SupersededID: mem.ID}, nil
			}
		}
	}

	mem := Memory{
		ID:        uuid.New().String(),
		Content:   req.Content,
		Category:  req.Category,
		Tags:      req.Tags,
		Source:    req.Source,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if mem.Tags == nil {
		mem.Tags = []string{}
	}

	if err := s.store.Add(ctx, mem); err != nil {
		return nil, fmt.Errorf("store memory: %w", err)
	}
	return &StoreOutcome{Memory: &mem}, nil
}

// unionTags merges b into a, preserving a's order, then appending any of b's
// tags not already present, with no duplicates.
func unionTags(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	result := make([]string, 0, len(a)+len(b))
	for _, t := range a {
		if !seen[t] {
			seen[t] = true
			result = append(result, t)
		}
	}
	for _, t := range b {
		if !seen[t] {
			seen[t] = true
			result = append(result, t)
		}
	}
	return result
}

func (s *Service) Search(ctx context.Context, req SearchRequest) ([]SearchResult, error) {
	if req.Query == "" {
		return nil, fmt.Errorf("query is required")
	}

	if req.Category != "" && !ValidCategories[req.Category] {
		return nil, fmt.Errorf("invalid category: %s", req.Category)
	}

	if req.Limit <= 0 {
		req.Limit = 5
	}
	if req.Limit > 20 {
		req.Limit = 20
	}

	return s.store.Search(ctx, req.Query, req.Category, req.Tags, req.Limit)
}

func (s *Service) List(ctx context.Context, req ListRequest) ([]Memory, error) {
	if req.Category != "" && !ValidCategories[req.Category] {
		return nil, fmt.Errorf("invalid category: %s", req.Category)
	}

	if req.Limit <= 0 {
		req.Limit = 10
	}
	if req.Limit > 50 {
		req.Limit = 50
	}
	if req.Offset < 0 {
		req.Offset = 0
	}

	return s.store.List(ctx, req.Category, req.Tags, req.Limit, req.Offset)
}

func (s *Service) Get(ctx context.Context, id string) (*Memory, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	return s.store.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}

	existing, err := s.store.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("lookup memory: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("memory not found: %s", id)
	}

	return s.store.Delete(ctx, id)
}

func (s *Service) Update(ctx context.Context, req UpdateRequest) (*Memory, error) {
	if req.ID == "" {
		return nil, fmt.Errorf("id is required")
	}

	existing, err := s.store.Get(ctx, req.ID)
	if err != nil {
		return nil, fmt.Errorf("lookup memory: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("memory not found: %s", req.ID)
	}

	if req.Content != nil {
		if *req.Content == "" {
			return nil, fmt.Errorf("content cannot be empty")
		}
		existing.Content = *req.Content
	}
	if req.Category != nil {
		if !ValidCategories[*req.Category] {
			return nil, fmt.Errorf("invalid category: %s", *req.Category)
		}
		existing.Category = *req.Category
	}
	if req.Tags != nil {
		existing.Tags = req.Tags
	}
	existing.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := s.store.Update(ctx, *existing); err != nil {
		return nil, fmt.Errorf("update memory: %w", err)
	}
	return existing, nil
}

func (s *Service) Count(ctx context.Context) (int, error) {
	return s.store.Count(ctx)
}

func (s *Service) Healthy(ctx context.Context) error {
	return s.store.Healthy(ctx)
}
