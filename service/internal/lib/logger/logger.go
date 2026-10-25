// Package logger configures slog for ghreport diagnostics.
package logger

import (
	"io"
	"log/slog"
	"strings"
)

// New follows AvitoLab's local text / production JSON logging split. All
// diagnostics use stderr so stdout remains a machine-readable report.
func New(out io.Writer, env, logLevel string) *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(logLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: level}
	if env == "prod" || env == "dev" {
		return slog.New(slog.NewJSONHandler(out, opts))
	}
	return slog.New(slog.NewTextHandler(out, opts))
}
