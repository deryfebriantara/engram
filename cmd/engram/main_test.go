package main

import (
	"testing"
	"time"

	"github.com/deryfebriantara/my-claude-memory/internal/memory"
)

func TestAgePenalty(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name string
		ts   string
		want float64
		tol  float64
	}{
		{"fresh", now.Format(time.RFC3339), 0, 0.0005},
		{"one year old clamps to max", now.Add(-365 * 24 * time.Hour).Format(time.RFC3339), 0.08, 0.0005},
		{"well over a year still clamps to max", now.Add(-800 * 24 * time.Hour).Format(time.RFC3339), 0.08, 0.0005},
		{"unparsable timestamp", "not-a-timestamp", 0, 0},
		{"empty timestamp (falls back to CreatedAt, also empty)", "", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := memory.Memory{UpdatedAt: tt.ts}
			got := agePenalty(m)
			diff := got - tt.want
			if diff > tt.tol || diff < -tt.tol {
				t.Fatalf("agePenalty(UpdatedAt=%q) = %v, want %v (+/-%v)", tt.ts, got, tt.want, tt.tol)
			}
		})
	}
}

func TestAgePenalty_FallsBackToCreatedAt(t *testing.T) {
	// UpdatedAt empty -> CreatedAt should be used instead.
	oldTS := time.Now().UTC().Add(-800 * 24 * time.Hour).Format(time.RFC3339)
	m := memory.Memory{CreatedAt: oldTS}
	got := agePenalty(m)
	if got < 0.0795 || got > 0.0805 {
		t.Fatalf("agePenalty using CreatedAt fallback = %v, want ~0.08", got)
	}
}

func TestHumanizeAge(t *testing.T) {
	// Subtract an extra minute beyond the target day count so the small
	// delay between computing "now" here and time.Since running inside
	// humanizeAge can never round the day count down by one.
	daysAgo := func(days int) string {
		return time.Now().Add(-time.Duration(days)*24*time.Hour - time.Minute).Format(time.RFC3339)
	}

	tests := []struct {
		name string
		ts   string
		want string
	}{
		{"today", daysAgo(0), "today"},
		{"one day ago", daysAgo(1), "1d ago"},
		{"edge of day bucket", daysAgo(29), "29d ago"},
		{"one month ago", daysAgo(30), "1mo ago"},
		{"several months ago", daysAgo(200), "6mo ago"},
		{"one year ago", daysAgo(365), "1y ago"},
		{"multiple years ago", daysAgo(800), "2y ago"},
		{"unparsable timestamp", "garbage", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := memory.Memory{UpdatedAt: tt.ts}
			got := humanizeAge(m)
			if got != tt.want {
				t.Fatalf("humanizeAge(UpdatedAt=%q) = %q, want %q", tt.ts, got, tt.want)
			}
		})
	}
}

func TestFormatRecallLine(t *testing.T) {
	tests := []struct {
		name string
		mem  memory.Memory
		want string
	}{
		{
			name: "no tags, no source, no parseable age",
			mem:  memory.Memory{Category: "fact", Content: "hello"},
			want: "- [fact] hello",
		},
		{
			name: "tags only",
			mem:  memory.Memory{Category: "fact", Content: "hello", Tags: []string{"a", "b"}},
			want: "- [fact] hello (tags: a,b)",
		},
		{
			name: "source only",
			mem:  memory.Memory{Category: "fact", Content: "hello", Source: "proj"},
			want: "- [fact] hello (source: proj)",
		},
		{
			name: "tags and source",
			mem:  memory.Memory{Category: "fact", Content: "hello", Tags: []string{"a", "b"}, Source: "proj"},
			want: "- [fact] hello (tags: a,b; source: proj)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatRecallLine(tt.mem)
			if got != tt.want {
				t.Fatalf("formatRecallLine(%+v) = %q, want %q", tt.mem, got, tt.want)
			}
		})
	}
}

func TestFormatListLine(t *testing.T) {
	m := memory.Memory{ID: "abc-123", Category: "preference", Content: "hello", Tags: []string{"x"}, Source: "proj"}
	want := "abc-123  [preference] hello (tags: x; source: proj)"
	if got := formatListLine(m); got != want {
		t.Fatalf("formatListLine(%+v) = %q, want %q", m, got, want)
	}
}
