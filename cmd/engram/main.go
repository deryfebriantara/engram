// Command engram is a CLI front-end for the memory service, meant to be
// called from scripts (in particular Claude Code hooks) rather than humans.
// Every subcommand keeps stdout machine-clean: only the requested output
// goes to stdout, all diagnostics go to stderr, so a hook can pipe stdout
// straight into an LLM context without leaking noise.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/deryfebriantara/my-claude-memory/internal/chromastore"
	"github.com/deryfebriantara/my-claude-memory/internal/config"
	"github.com/deryfebriantara/my-claude-memory/internal/embedder"
	"github.com/deryfebriantara/my-claude-memory/internal/memory"
)

const opTimeout = 20 * time.Second

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd, args := os.Args[1], os.Args[2:]

	switch cmd {
	case "recall":
		runRecall(args)
	case "store":
		runStore(args)
	case "delete":
		runDelete(args)
	case "health":
		runHealth(args)
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "engram: unknown command %q\n", cmd)
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage:
  engram recall [-limit N] [-threshold F] [-source S] <query...>
  engram store [-category C] [-tags a,b] [-source S] <content...>
  engram delete <id>
  engram health`)
}

// newService wires config/embedder/chromastore/service exactly like the MCP
// server's main, so both entry points share identical behavior.
func newService(ctx context.Context) (*memory.Service, *embedder.OllamaEmbedder, error) {
	cfg := config.Load()

	emb, err := embedder.NewOllama(cfg.OllamaURL, cfg.OllamaModel)
	if err != nil {
		return nil, nil, fmt.Errorf("create embedder: %w", err)
	}

	store, err := chromastore.New(ctx, cfg.ChromaURL, cfg.CollectionName, emb)
	if err != nil {
		return nil, nil, fmt.Errorf("create store: %w", err)
	}

	return memory.NewService(store, cfg.DedupThreshold), emb, nil
}

func runRecall(args []string) {
	fs := flag.NewFlagSet("recall", flag.ExitOnError)
	limit := fs.Int("limit", 3, "max results to print (max 10)")
	threshold := fs.Float64("threshold", 0.55, "max raw distance to consider")
	source := fs.String("source", "", "source to boost when it matches a memory's source")
	fs.Parse(args)

	query := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(query) == "" {
		fmt.Fprintln(os.Stderr, "engram recall: query is required")
		os.Exit(1)
	}

	if *limit <= 0 {
		*limit = 3
	}
	if *limit > 10 {
		*limit = 10
	}

	fetch := *limit * 3
	if fetch > 10 {
		fetch = 10
	}

	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	svc, _, err := newService(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "engram recall: %v\n", err)
		os.Exit(1)
	}

	results, err := svc.Search(ctx, memory.SearchRequest{Query: query, Limit: fetch})
	if err != nil {
		fmt.Fprintf(os.Stderr, "engram recall: %v\n", err)
		os.Exit(1)
	}

	type candidate struct {
		mem      memory.Memory
		adjusted float64
	}

	var candidates []candidate
	for _, r := range results {
		distance := float64(r.Distance)
		if distance > *threshold {
			continue
		}

		var sourceBoost float64
		if *source != "" && *source == r.Memory.Source {
			sourceBoost = 0.05
		}

		candidates = append(candidates, candidate{
			mem:      r.Memory,
			adjusted: distance + agePenalty(r.Memory) - sourceBoost,
		})
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].adjusted < candidates[j].adjusted })

	if len(candidates) > *limit {
		candidates = candidates[:*limit]
	}

	for _, c := range candidates {
		fmt.Println(formatRecallLine(c.mem))
	}
}

// agePenalty grows linearly from 0 (fresh) to 0.08 (a year or older),
// keeping older memories from crowding out fresher, more relevant ones.
// A timestamp that fails to parse is treated as "no penalty" rather than
// erroring the whole recall.
func agePenalty(m memory.Memory) float64 {
	ts := m.UpdatedAt
	if ts == "" {
		ts = m.CreatedAt
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return 0
	}

	days := time.Since(t).Hours() / 24
	if days < 0 {
		days = 0
	}
	return min(0.08, days/365*0.08)
}

func humanizeAge(m memory.Memory) string {
	ts := m.UpdatedAt
	if ts == "" {
		ts = m.CreatedAt
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ""
	}

	days := int(time.Since(t).Hours() / 24)
	switch {
	case days <= 0:
		return "today"
	case days < 30:
		return fmt.Sprintf("%dd ago", days)
	case days < 365:
		return fmt.Sprintf("%dmo ago", days/30)
	default:
		return fmt.Sprintf("%dy ago", days/365)
	}
}

func formatRecallLine(m memory.Memory) string {
	var parts []string
	if len(m.Tags) > 0 {
		parts = append(parts, "tags: "+strings.Join(m.Tags, ","))
	}
	if m.Source != "" {
		parts = append(parts, "source: "+m.Source)
	}
	if age := humanizeAge(m); age != "" {
		parts = append(parts, age)
	}

	suffix := ""
	if len(parts) > 0 {
		suffix = " (" + strings.Join(parts, "; ") + ")"
	}
	return fmt.Sprintf("- [%s] %s%s", m.Category, m.Content, suffix)
}

func runStore(args []string) {
	fs := flag.NewFlagSet("store", flag.ExitOnError)
	category := fs.String("category", memory.CategoryFact, "memory category")
	tags := fs.String("tags", "", "comma-separated tags")
	source := fs.String("source", "", "source or project context")
	fs.Parse(args)

	content := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(content) == "" {
		fmt.Fprintln(os.Stderr, "engram store: content is required")
		os.Exit(1)
	}

	var tagList []string
	for _, t := range strings.Split(*tags, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tagList = append(tagList, t)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	svc, _, err := newService(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "engram store: %v\n", err)
		os.Exit(1)
	}

	outcome, err := svc.Store(ctx, memory.StoreRequest{
		Content:  content,
		Category: *category,
		Tags:     tagList,
		Source:   *source,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "engram store: %v\n", err)
		os.Exit(1)
	}

	if outcome.SupersededID != "" {
		fmt.Printf("merged into %s\n", outcome.SupersededID)
	} else {
		fmt.Printf("stored %s\n", outcome.Memory.ID)
	}
}

func runDelete(args []string) {
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintln(os.Stderr, "engram delete: id is required")
		os.Exit(1)
	}
	id := args[0]

	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	svc, _, err := newService(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "engram delete: %v\n", err)
		os.Exit(1)
	}

	if err := svc.Delete(ctx, id); err != nil {
		fmt.Fprintf(os.Stderr, "engram delete: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("deleted %s\n", id)
}

func runHealth(args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	svc, emb, err := newService(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "engram health: %v\n", err)
		os.Exit(1)
	}

	chromaStatus := "ok"
	chromaHealthy := true
	if err := svc.Healthy(ctx); err != nil {
		chromaStatus = fmt.Sprintf("error: %s", err)
		chromaHealthy = false
	}

	ollamaStatus := "ok"
	ollamaHealthy := true
	if err := emb.Healthy(); err != nil {
		ollamaStatus = fmt.Sprintf("error: %s", err)
		ollamaHealthy = false
	}

	count := -1
	if chromaHealthy {
		if c, err := svc.Count(ctx); err == nil {
			count = c
		}
	}

	fmt.Printf("chroma:    %s\n", chromaStatus)
	fmt.Printf("ollama:    %s (model: %s)\n", ollamaStatus, emb.Model())
	fmt.Printf("memories:  %d\n", count)

	if !chromaHealthy || !ollamaHealthy {
		os.Exit(1)
	}
}
