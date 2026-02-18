package main

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/corpus"
)

func TestSourceRemoteURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source corpus.SourceConfig
		want   string
	}{
		{
			name: "repository url preferred",
			source: corpus.SourceConfig{
				Repository:    "github.com/acme/ignored",
				RepositoryURL: "https://github.com/acme/preferred",
			},
			want: "https://github.com/acme/preferred.git",
		},
		{
			name: "repository url trailing slash",
			source: corpus.SourceConfig{
				RepositoryURL: "https://github.com/acme/preferred/",
			},
			want: "https://github.com/acme/preferred.git",
		},
		{
			name: "github repository path",
			source: corpus.SourceConfig{
				Repository: "github.com/acme/repo",
			},
			want: "https://github.com/acme/repo.git",
		},
		{
			name: "full https repository",
			source: corpus.SourceConfig{
				Repository: "https://github.com/acme/repo",
			},
			want: "https://github.com/acme/repo.git",
		},
		{
			name: "ssh repository untouched",
			source: corpus.SourceConfig{
				Repository: "git@github.com:acme/repo.git",
			},
			want: "git@github.com:acme/repo.git",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := sourceRemoteURL(tt.source)
			if err != nil {
				t.Fatalf("sourceRemoteURL: %v", err)
			}
			if got != tt.want {
				t.Fatalf("remote url = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSourceRemoteURL_RequiresRepository(t *testing.T) {
	t.Parallel()

	_, err := sourceRemoteURL(corpus.SourceConfig{})
	if err == nil || !strings.Contains(err.Error(), "repository or repository_url is required") {
		t.Fatalf("expected repository error, got %v", err)
	}
}

func TestFetchPinnedSource_ValidatesInput(t *testing.T) {
	t.Parallel()

	if err := fetchPinnedSource(t.TempDir(), corpus.SourceConfig{}); err == nil {
		t.Fatal("expected source-name validation error")
	}
	err := fetchPinnedSource(t.TempDir(), corpus.SourceConfig{Name: "seed"})
	if err == nil || !strings.Contains(err.Error(), "commit_sha is required") {
		t.Fatalf("expected commit validation error, got %v", err)
	}
}

func TestResolveFetchRoot(t *testing.T) {
	t.Parallel()

	resolved := resolveFetchRoot(defaultFetchRoot, "v2026-02-17")
	want := defaultFetchRoot + "/v2026-02-17"
	if resolved != want {
		t.Fatalf("resolveFetchRoot(default, version) = %q, want %q", resolved, want)
	}

	custom := resolveFetchRoot("/custom/fetch", "v2026-02-17")
	if custom != "/custom/fetch" {
		t.Fatalf("resolveFetchRoot(custom, version) = %q, want /custom/fetch", custom)
	}
}
