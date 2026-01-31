package config

import "os"

type Config struct {
	ChromaURL      string
	OllamaURL      string
	OllamaModel    string
	CollectionName string
}

func Load() *Config {
	return &Config{
		ChromaURL:      getEnv("CHROMA_URL", "http://localhost:8000"),
		OllamaURL:      getEnv("OLLAMA_URL", "http://127.0.0.1:11434"),
		OllamaModel:    getEnv("OLLAMA_MODEL", "nomic-embed-text"),
		CollectionName: getEnv("COLLECTION_NAME", "claude_memories"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
