// Package config parses and validates ghreport's command-line and environment settings.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DefaultAPIBaseURL is the public GitHub REST API endpoint.
const DefaultAPIBaseURL = "https://api.github.com"

var repoPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// Config is the validated configuration of one ghreport run.
type Config struct {
	Repos    []string
	Limit    int // Zero means no limit.
	Since    time.Time
	HasSince bool
	Out      string
	Trace    string
	CacheDir string
	APIBase  *url.URL
	Token    string
	Env      string
	LogLevel string
}

// Parse reads CLI arguments and environment values without side effects.
func Parse(args []string, getenv func(string) string) (Config, error) {
	var cfg Config
	fs := flag.NewFlagSet("ghreport", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var repos, since, limit string
	fs.StringVar(&repos, "repos", "", "comma-separated owner/repo list")
	fs.StringVar(&limit, "limit", "", "maximum number of issues per repository")
	fs.StringVar(&since, "since", "", "earliest creation date in UTC (YYYY-MM-DD)")
	fs.StringVar(&cfg.Out, "out", "-", "report file or - for stdout")
	fs.StringVar(&cfg.Trace, "trace", "", "JSON Lines trace file")
	fs.StringVar(&cfg.CacheDir, "cache-dir", "", "persistent ETag cache directory")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if fs.NArg() != 0 {
		return cfg, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	if repos == "" {
		return cfg, errors.New("--repos is required")
	}
	for _, repo := range strings.Split(repos, ",") {
		parts := strings.Split(repo, "/")
		if len(parts) != 2 || !repoPart.MatchString(parts[0]) || !repoPart.MatchString(parts[1]) || parts[0] == "." || parts[0] == ".." || parts[1] == "." || parts[1] == ".." {
			return cfg, fmt.Errorf("invalid repository %q: expected owner/repo", repo)
		}
		cfg.Repos = append(cfg.Repos, repo)
	}
	if limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil || n < 1 {
			return cfg, errors.New("--limit must be an integer greater than zero")
		}
		cfg.Limit = n
	}
	if since != "" {
		var err error
		cfg.Since, err = time.Parse("2006-01-02", since)
		if err != nil || cfg.Since.Format("2006-01-02") != since {
			return cfg, errors.New("--since must be a valid YYYY-MM-DD date")
		}
		cfg.HasSince = true
	}
	if cfg.Out == "" {
		return cfg, errors.New("--out must be a file or -")
	}
	base := getenv("GITHUB_API_BASE_URL")
	if base == "" {
		base = DefaultAPIBaseURL
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return cfg, errors.New("GITHUB_API_BASE_URL must be an absolute http(s) URL without credentials, query, or fragment")
	}
	cfg.APIBase = parsed
	cfg.Token = getenv("GITHUB_TOKEN")
	cfg.Env = getenv("ENV")
	cfg.LogLevel = getenv("LOG_LEVEL")
	return cfg, nil
}
