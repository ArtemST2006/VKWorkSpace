package config

import (
	"strings"
	"testing"
)

func TestParseDefaultsAndValidation(t *testing.T) {
	getenv := func(string) string { return "" }
	cfg, err := Parse([]string{"--repos", "cli/cli,owner/repo", "--since", "2026-01-01"}, getenv)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Repos) != 2 || cfg.Limit != 0 || !cfg.HasSince || cfg.APIBase.String() != DefaultAPIBaseURL || cfg.Out != "-" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	for _, args := range [][]string{
		{},
		{"--repos", "bad"},
		{"--repos", "a/b,"},
		{"--repos", "a/b", "--limit", "0"},
		{"--repos", "a/b", "--limit", "oops"},
		{"--repos", "a/b", "--since", "2026-02-30"},
		{"--repos", "a/b", "unexpected"},
	} {
		if _, err := Parse(args, getenv); err == nil {
			t.Errorf("accepted invalid arguments %q", strings.Join(args, " "))
		}
	}
}

func TestRejectUnsafeBaseURL(t *testing.T) {
	for _, base := range []string{"relative", "ftp://example.org", "https://user:pass@example.org", "https://example.org/?token=x"} {
		_, err := Parse([]string{"--repos", "a/b"}, func(key string) string {
			if key == "GITHUB_API_BASE_URL" {
				return base
			}
			return ""
		})
		if err == nil {
			t.Errorf("accepted unsafe base URL %q", base)
		}
	}
}
