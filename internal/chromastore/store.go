package chromastore

import (
	"context"
	"fmt"
	"strings"

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
	"github.com/amikos-tech/chroma-go/pkg/embeddings"

	"github.com/deryfebriantara/my-claude-memory/internal/embedder"
	"github.com/deryfebriantara/my-claude-memory/internal/memory"
)

type ChromaStore struct {
	client     chroma.Client
	collection chroma.Collection
	embedder   *embedder.OllamaEmbedder
}

func New(ctx context.Context, chromaURL, collectionName string, emb *embedder.OllamaEmbedder) (*ChromaStore, error) {
	client, err := chroma.NewHTTPClient(
		chroma.WithBaseURL(chromaURL),
	)
	if err != nil {
		return nil, fmt.Errorf("create chroma client: %w", err)
	}

	col, err := client.GetOrCreateCollection(ctx, collectionName,
		chroma.WithEmbeddingFunctionCreate(emb.EF),
		chroma.WithHNSWSpaceCreate(embeddings.COSINE),
	)
	if err != nil {
		return nil, fmt.Errorf("get or create collection: %w", err)
	}

	return &ChromaStore{
		client:     client,
		collection: col,
		embedder:   emb,
	}, nil
}

func (s *ChromaStore) Add(ctx context.Context, mem memory.Memory) error {
	return s.collection.Add(ctx,
		chroma.WithIDs(chroma.DocumentID(mem.ID)),
		chroma.WithTexts(mem.Content),
		chroma.WithMetadatas(buildMetadata(mem)),
	)
}

func (s *ChromaStore) Search(ctx context.Context, query string, category string, tags []string, limit int) ([]memory.SearchResult, error) {
	opts := []chroma.QueryOption{
		chroma.WithQueryTexts(query),
		chroma.WithNResults(limit),
		chroma.WithIncludeQuery(
			chroma.Include("documents"),
			chroma.Include("metadatas"),
			chroma.Include("distances"),
		),
	}

	where := buildWhereFilter(category, tags)
	if where != nil {
		opts = append(opts, chroma.WithWhereQuery(where))
	}

	qr, err := s.collection.Query(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("query collection: %w", err)
	}

	if qr.CountGroups() == 0 {
		return nil, nil
	}

	ids := qr.GetIDGroups()[0]
	docs := qr.GetDocumentsGroups()[0]
	metas := qr.GetMetadatasGroups()[0]
	dists := qr.GetDistancesGroups()[0]

	var results []memory.SearchResult
	for i := range ids {
		var doc string
		if i < len(docs) && docs[i] != nil {
			doc = docs[i].ContentString()
		}
		var meta chroma.DocumentMetadata
		if i < len(metas) {
			meta = metas[i]
		}
		mem := rowToMemory(ids[i], doc, meta)
		var dist float32
		if i < len(dists) {
			dist = float32(dists[i])
		}
		results = append(results, memory.SearchResult{
			Memory:   mem,
			Distance: dist,
		})
	}
	return results, nil
}

func (s *ChromaStore) List(ctx context.Context, category string, tags []string, limit, offset int) ([]memory.Memory, error) {
	opts := []chroma.GetOption{
		chroma.WithLimitGet(limit),
		chroma.WithOffsetGet(offset),
		chroma.WithIncludeGet(
			chroma.Include("documents"),
			chroma.Include("metadatas"),
		),
	}

	where := buildWhereFilter(category, tags)
	if where != nil {
		opts = append(opts, chroma.WithWhereGet(where))
	}

	gr, err := s.collection.Get(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}

	ids := gr.GetIDs()
	docs := gr.GetDocuments()
	metas := gr.GetMetadatas()

	var memories []memory.Memory
	for i := range ids {
		var doc string
		if i < len(docs) && docs[i] != nil {
			doc = docs[i].ContentString()
		}
		var meta chroma.DocumentMetadata
		if i < len(metas) {
			meta = metas[i]
		}
		memories = append(memories, rowToMemory(ids[i], doc, meta))
	}
	return memories, nil
}

func (s *ChromaStore) Get(ctx context.Context, id string) (*memory.Memory, error) {
	gr, err := s.collection.Get(ctx,
		chroma.WithIDsGet(chroma.DocumentID(id)),
		chroma.WithIncludeGet(
			chroma.Include("documents"),
			chroma.Include("metadatas"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}

	ids := gr.GetIDs()
	if len(ids) == 0 {
		return nil, nil
	}

	docs := gr.GetDocuments()
	metas := gr.GetMetadatas()

	var doc string
	if len(docs) > 0 && docs[0] != nil {
		doc = docs[0].ContentString()
	}
	var meta chroma.DocumentMetadata
	if len(metas) > 0 {
		meta = metas[0]
	}

	mem := rowToMemory(ids[0], doc, meta)
	return &mem, nil
}

func (s *ChromaStore) Update(ctx context.Context, mem memory.Memory) error {
	return s.collection.Update(ctx,
		chroma.WithIDsUpdate(chroma.DocumentID(mem.ID)),
		chroma.WithTextsUpdate(mem.Content),
		chroma.WithMetadatasUpdate(buildMetadata(mem)),
	)
}

func (s *ChromaStore) Delete(ctx context.Context, id string) error {
	return s.collection.Delete(ctx,
		chroma.WithIDsDelete(chroma.DocumentID(id)),
	)
}

func (s *ChromaStore) Count(ctx context.Context) (int, error) {
	count, err := s.collection.Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count collection: %w", err)
	}
	return count, nil
}

func (s *ChromaStore) Healthy(ctx context.Context) error {
	_, err := s.collection.Count(ctx)
	if err != nil {
		return fmt.Errorf("chromadb unhealthy: %w", err)
	}
	return nil
}

// Tags are stored twice: as one joined "tags" string for reading back, and as
// one boolean "tag:<name>" attribute per tag, because Chroma metadata has no
// list type and equality filters can't match inside a joined string.
func buildMetadata(mem memory.Memory) chroma.DocumentMetadata {
	attrs := []*chroma.MetaAttribute{
		chroma.NewStringAttribute("category", mem.Category),
		chroma.NewStringAttribute("tags", strings.Join(mem.Tags, ",")),
		chroma.NewStringAttribute("source", mem.Source),
		chroma.NewStringAttribute("created_at", mem.CreatedAt),
		chroma.NewStringAttribute("updated_at", mem.UpdatedAt),
	}
	for _, tag := range mem.Tags {
		if tag != "" {
			attrs = append(attrs, chroma.NewBoolAttribute("tag:"+tag, true))
		}
	}
	return chroma.NewDocumentMetadata(attrs...)
}

func buildWhereFilter(category string, tags []string) chroma.WhereClause {
	var clauses []chroma.WhereClause

	if category != "" {
		clauses = append(clauses, chroma.EqString("category", category))
	}

	if len(tags) > 0 {
		var tagClauses []chroma.WhereClause
		for _, tag := range tags {
			tagClauses = append(tagClauses, chroma.EqBool("tag:"+tag, true))
		}
		if len(tagClauses) == 1 {
			clauses = append(clauses, tagClauses[0])
		} else {
			clauses = append(clauses, chroma.Or(tagClauses...))
		}
	}

	if len(clauses) == 0 {
		return nil
	}
	if len(clauses) == 1 {
		return clauses[0]
	}
	return chroma.And(clauses...)
}

func rowToMemory(id chroma.DocumentID, document string, metadata chroma.DocumentMetadata) memory.Memory {
	mem := memory.Memory{
		ID:      string(id),
		Content: document,
	}
	if metadata != nil {
		if v, ok := metadata.GetString("category"); ok {
			mem.Category = v
		}
		if v, ok := metadata.GetString("tags"); ok && v != "" {
			mem.Tags = strings.Split(v, ",")
		}
		if v, ok := metadata.GetString("source"); ok {
			mem.Source = v
		}
		if v, ok := metadata.GetString("created_at"); ok {
			mem.CreatedAt = v
		}
		if v, ok := metadata.GetString("updated_at"); ok {
			mem.UpdatedAt = v
		}
	}
	return mem
}
