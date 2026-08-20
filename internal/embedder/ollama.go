package embedder

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/amikos-tech/chroma-go/pkg/embeddings"
	"github.com/amikos-tech/chroma-go/pkg/embeddings/ollama"
)

// prefixedEF wraps an embeddings.EmbeddingFunction to add the task prefixes
// nomic-embed-text requires: it's an asymmetric model, trained so documents
// and queries are embedded differently ("search_document: " vs
// "search_query: " prefixes). chroma-go's ollama embedding function sends
// raw text with neither prefix, which measurably hurts retrieval quality.
// Every other method (Name, GetConfig, DefaultSpace, ...) is inherited
// unchanged via the embedded interface.
type prefixedEF struct {
	embeddings.EmbeddingFunction
}

func (p *prefixedEF) EmbedDocuments(ctx context.Context, texts []string) ([]embeddings.Embedding, error) {
	prefixed := make([]string, len(texts))
	for i, t := range texts {
		prefixed[i] = "search_document: " + t
	}
	return p.EmbeddingFunction.EmbedDocuments(ctx, prefixed)
}

func (p *prefixedEF) EmbedQuery(ctx context.Context, text string) (embeddings.Embedding, error) {
	return p.EmbeddingFunction.EmbedQuery(ctx, "search_query: "+text)
}

type OllamaEmbedder struct {
	EF        embeddings.EmbeddingFunction
	baseURL   string
	modelName string
}

func NewOllama(baseURL, model string) (*OllamaEmbedder, error) {
	ef, err := ollama.NewOllamaEmbeddingFunction(
		ollama.WithBaseURL(baseURL),
		ollama.WithModel(embeddings.EmbeddingModel(model)),
	)
	if err != nil {
		return nil, fmt.Errorf("create ollama embedding function: %w", err)
	}
	return &OllamaEmbedder{
		EF:        &prefixedEF{EmbeddingFunction: ef},
		baseURL:   baseURL,
		modelName: model,
	}, nil
}

func (o *OllamaEmbedder) Healthy() error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(o.baseURL + "/api/tags")
	if err != nil {
		return fmt.Errorf("ollama unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}
	return nil
}

func (o *OllamaEmbedder) Model() string {
	return o.modelName
}
