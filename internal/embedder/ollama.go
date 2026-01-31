package embedder

import (
	"fmt"
	"net/http"
	"time"

	"github.com/amikos-tech/chroma-go/pkg/embeddings"
	"github.com/amikos-tech/chroma-go/pkg/embeddings/ollama"
)

type OllamaEmbedder struct {
	EF        *ollama.OllamaEmbeddingFunction
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
		EF:        ef,
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
