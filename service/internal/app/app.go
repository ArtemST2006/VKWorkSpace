// Package app connects CLI configuration, reporting, and output.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"git.miem.hse.ru/blank5204543/vkworkspace/service/internal/cache"
	"git.miem.hse.ru/blank5204543/vkworkspace/service/internal/config"
	"git.miem.hse.ru/blank5204543/vkworkspace/service/internal/ghreport"
	"git.miem.hse.ru/blank5204543/vkworkspace/service/internal/github"
	"git.miem.hse.ru/blank5204543/vkworkspace/service/internal/lib/logger"
)

// Run executes the CLI. It returns an exit code without terminating the
// process, which also makes the entry point testable.
func Run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	cfg, err := config.Parse(args, getenv)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "ghreport:", err)
		return 2
	}
	log := logger.New(stderr, cfg.Env, cfg.LogLevel)
	var trace io.Writer
	if cfg.Trace != "" {
		file, openErr := os.OpenFile(cfg.Trace, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		err = openErr
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "ghreport: cannot open trace:", err)
			return fatal(stdout, "io_error")
		}
		defer func() { _ = file.Close() }()
		trace = file
	}
	client := &github.Client{HTTP: &http.Client{Timeout: 30 * time.Second}, Cache: cache.Store{Dir: cfg.CacheDir}, Token: cfg.Token, Trace: trace}
	service := ghreport.Service{Config: cfg, Client: client, Log: log}
	report, code := service.Run(ctx)
	if code == github.Unauthorized {
		return fatal(stdout, "unauthorized")
	}
	if code == "all_failed" {
		return fatal(stdout, "all_failed")
	}
	output := stdout
	if cfg.Out != "-" {
		file, err := os.OpenFile(cfg.Out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "ghreport: cannot open report:", err)
			return fatal(stdout, "io_error")
		}
		defer func() { _ = file.Close() }()
		output = file
	}
	if err := json.NewEncoder(output).Encode(report); err != nil {
		_, _ = fmt.Fprintln(stderr, "ghreport: cannot write report:", err)
		return 3
	}
	if code == "partial_failure" {
		return 1
	}
	return 0
}

func fatal(out io.Writer, code string) int {
	_ = json.NewEncoder(out).Encode(struct {
		Error string `json:"error"`
	}{Error: code})
	return 3
}
