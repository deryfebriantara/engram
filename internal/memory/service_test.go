package memory

import (
	"context"
	"errors"
	"testing"
)

// mockStore implements the Store interface with per-call function fields, so
// each test only wires up the behavior it actually needs. A nil function
// field that ends up being called will panic with a clear "nil func" message
// via Go's own runtime, which is enough signal to fail the test loudly.
type mockStore struct {
	AddFunc     func(ctx context.Context, mem Memory) error
	SearchFunc  func(ctx context.Context, query, category string, tags []string, limit int) ([]SearchResult, error)
	ListFunc    func(ctx context.Context, category string, tags []string, limit, offset int) ([]Memory, error)
	GetFunc     func(ctx context.Context, id string) (*Memory, error)
	UpdateFunc  func(ctx context.Context, mem Memory) error
	DeleteFunc  func(ctx context.Context, id string) error
	CountFunc   func(ctx context.Context) (int, error)
	HealthyFunc func(ctx context.Context) error

	addCalled    bool
	updateCalled bool
	searchCalled bool
}

func (m *mockStore) Add(ctx context.Context, mem Memory) error {
	m.addCalled = true
	return m.AddFunc(ctx, mem)
}

func (m *mockStore) Search(ctx context.Context, query, category string, tags []string, limit int) ([]SearchResult, error) {
	m.searchCalled = true
	return m.SearchFunc(ctx, query, category, tags, limit)
}

func (m *mockStore) List(ctx context.Context, category string, tags []string, limit, offset int) ([]Memory, error) {
	return m.ListFunc(ctx, category, tags, limit, offset)
}

func (m *mockStore) Get(ctx context.Context, id string) (*Memory, error) {
	return m.GetFunc(ctx, id)
}

func (m *mockStore) Update(ctx context.Context, mem Memory) error {
	m.updateCalled = true
	return m.UpdateFunc(ctx, mem)
}

func (m *mockStore) Delete(ctx context.Context, id string) error {
	return m.DeleteFunc(ctx, id)
}

func (m *mockStore) Count(ctx context.Context) (int, error) {
	return m.CountFunc(ctx)
}

func (m *mockStore) Healthy(ctx context.Context) error {
	return m.HealthyFunc(ctx)
}

func TestStore_DedupMergePath(t *testing.T) {
	existing := Memory{
		ID:        "existing-id",
		Content:   "old content",
		Category:  CategoryFact,
		Tags:      []string{"a", "b"},
		Source:    "orig-source",
		CreatedAt: "2020-01-01T00:00:00Z",
		UpdatedAt: "2020-01-01T00:00:00Z",
	}

	var capturedUpdate Memory
	store := &mockStore{
		SearchFunc: func(ctx context.Context, query, category string, tags []string, limit int) ([]SearchResult, error) {
			return []SearchResult{{Memory: existing, Distance: 0.1}}, nil
		},
		UpdateFunc: func(ctx context.Context, mem Memory) error {
			capturedUpdate = mem
			return nil
		},
		AddFunc: func(ctx context.Context, mem Memory) error {
			t.Fatal("Add should not be called on a dedup-merge path")
			return nil
		},
	}

	svc := NewService(store, 0.2)
	outcome, err := svc.Store(context.Background(), StoreRequest{
		Content:  "new content",
		Category: CategoryPreference,
		Tags:     []string{"b", "c"},
		// Source intentionally empty: existing source must be kept.
	})
	if err != nil {
		t.Fatalf("Store returned error: %v", err)
	}

	if !store.updateCalled {
		t.Fatal("expected store.Update to be called")
	}
	if outcome.SupersededID != "existing-id" {
		t.Fatalf("SupersededID = %q, want %q", outcome.SupersededID, "existing-id")
	}

	wantTags := []string{"a", "b", "c"}
	if !equalSlices(capturedUpdate.Tags, wantTags) {
		t.Fatalf("Tags = %v, want %v (order-stable union)", capturedUpdate.Tags, wantTags)
	}
	if capturedUpdate.Source != "orig-source" {
		t.Fatalf("Source = %q, want existing source %q kept (request source was empty)", capturedUpdate.Source, "orig-source")
	}
	if capturedUpdate.Content != "new content" {
		t.Fatalf("Content = %q, want %q", capturedUpdate.Content, "new content")
	}
	if capturedUpdate.Category != CategoryPreference {
		t.Fatalf("Category = %q, want %q", capturedUpdate.Category, CategoryPreference)
	}
}

func TestStore_NoMergeAboveThreshold(t *testing.T) {
	existing := Memory{ID: "existing-id", Content: "old content", Category: CategoryFact}

	var addedMem Memory
	store := &mockStore{
		SearchFunc: func(ctx context.Context, query, category string, tags []string, limit int) ([]SearchResult, error) {
			return []SearchResult{{Memory: existing, Distance: 0.9}}, nil
		},
		AddFunc: func(ctx context.Context, mem Memory) error {
			addedMem = mem
			return nil
		},
		UpdateFunc: func(ctx context.Context, mem Memory) error {
			t.Fatal("Update should not be called when nearest match is above threshold")
			return nil
		},
	}

	svc := NewService(store, 0.2)
	outcome, err := svc.Store(context.Background(), StoreRequest{Content: "totally different", Category: CategoryFact})
	if err != nil {
		t.Fatalf("Store returned error: %v", err)
	}
	if !store.addCalled {
		t.Fatal("expected store.Add to be called")
	}
	if outcome.SupersededID != "" {
		t.Fatalf("SupersededID = %q, want empty (fresh insert)", outcome.SupersededID)
	}
	if addedMem.Content != "totally different" {
		t.Fatalf("added Content = %q, want %q", addedMem.Content, "totally different")
	}
	if addedMem.ID == "" {
		t.Fatal("expected a generated ID on fresh insert")
	}
}

func TestStore_DedupThresholdZeroSkipsSearch(t *testing.T) {
	store := &mockStore{
		AddFunc: func(ctx context.Context, mem Memory) error { return nil },
		SearchFunc: func(ctx context.Context, query, category string, tags []string, limit int) ([]SearchResult, error) {
			t.Fatal("Search should not be called when dedupThreshold is 0")
			return nil, nil
		},
	}

	svc := NewService(store, 0)
	_, err := svc.Store(context.Background(), StoreRequest{Content: "anything", Category: CategoryFact})
	if err != nil {
		t.Fatalf("Store returned error: %v", err)
	}
	if store.searchCalled {
		t.Fatal("Search was called despite dedupThreshold being 0")
	}
	if !store.addCalled {
		t.Fatal("expected store.Add to be called")
	}
}

func TestStore_DedupSearchErrorFallsBackToInsert(t *testing.T) {
	var addCalled bool
	store := &mockStore{
		SearchFunc: func(ctx context.Context, query, category string, tags []string, limit int) ([]SearchResult, error) {
			return nil, errors.New("chroma is down")
		},
		AddFunc: func(ctx context.Context, mem Memory) error {
			addCalled = true
			return nil
		},
		UpdateFunc: func(ctx context.Context, mem Memory) error {
			t.Fatal("Update should not be called when the dedup search itself failed")
			return nil
		},
	}

	svc := NewService(store, 0.2)
	outcome, err := svc.Store(context.Background(), StoreRequest{Content: "fine", Category: CategoryFact})
	if err != nil {
		t.Fatalf("Store returned error: %v", err)
	}
	if !addCalled {
		t.Fatal("expected fallback insert via Add after a failed dedup search")
	}
	if outcome.SupersededID != "" {
		t.Fatalf("SupersededID = %q, want empty", outcome.SupersededID)
	}
}

func TestStore_InvalidCategory(t *testing.T) {
	svc := NewService(&mockStore{}, 0.2)
	_, err := svc.Store(context.Background(), StoreRequest{Content: "x", Category: "not-a-real-category"})
	if err == nil {
		t.Fatal("expected an error for an invalid category")
	}
}

func TestStore_EmptyContent(t *testing.T) {
	svc := NewService(&mockStore{}, 0.2)
	_, err := svc.Store(context.Background(), StoreRequest{Content: "", Category: CategoryFact})
	if err == nil {
		t.Fatal("expected an error for empty content")
	}
}

func TestUpdate_NotFound(t *testing.T) {
	store := &mockStore{
		GetFunc: func(ctx context.Context, id string) (*Memory, error) { return nil, nil },
	}
	svc := NewService(store, 0.2)

	newContent := "updated"
	_, err := svc.Update(context.Background(), UpdateRequest{ID: "missing-id", Content: &newContent})
	if err == nil {
		t.Fatal("expected an error when updating a memory that doesn't exist")
	}
}

func TestDelete_NotFound(t *testing.T) {
	store := &mockStore{
		GetFunc: func(ctx context.Context, id string) (*Memory, error) { return nil, nil },
	}
	svc := NewService(store, 0.2)

	if err := svc.Delete(context.Background(), "missing-id"); err == nil {
		t.Fatal("expected an error when deleting a memory that doesn't exist")
	}
}

func TestSearch_LimitClamping(t *testing.T) {
	tests := []struct {
		name      string
		reqLimit  int
		wantLimit int
	}{
		{"zero defaults to 5", 0, 5},
		{"negative defaults to 5", -3, 5},
		{"within range unchanged", 10, 10},
		{"above max clamps to 20", 50, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotLimit int
			store := &mockStore{
				SearchFunc: func(ctx context.Context, query, category string, tags []string, limit int) ([]SearchResult, error) {
					gotLimit = limit
					return nil, nil
				},
			}
			svc := NewService(store, 0.2)
			_, err := svc.Search(context.Background(), SearchRequest{Query: "q", Limit: tt.reqLimit})
			if err != nil {
				t.Fatalf("Search returned error: %v", err)
			}
			if gotLimit != tt.wantLimit {
				t.Fatalf("limit passed to store.Search = %d, want %d", gotLimit, tt.wantLimit)
			}
		})
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
