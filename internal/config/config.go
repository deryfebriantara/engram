package config

import (
	"os"
	"strconv"
)

type Config struct {
	ChromaURL      string
	OllamaURL      string
	OllamaModel    string
	CollectionName string
	DedupThreshold float64
}

func Load() *Config {
	return &Config{
		ChromaURL:      getEnv("CHROMA_URL", "http://localhost:8000"),
		OllamaURL:      getEnv("OLLAMA_URL", "http://127.0.0.1:11434"),
		OllamaModel:    getEnv("OLLAMA_MODEL", "nomic-embed-text"),
		CollectionName: getEnv("COLLECTION_NAME", "claude_memories"),
		DedupThreshold: getEnvFloat("DEDUP_THRESHOLD", 0.2),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}
