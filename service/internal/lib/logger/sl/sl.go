// Package sl contains helpers for structured slog attributes.
package sl

import "log/slog"

// Err wraps an error as a structured slog attribute.
func Err(err error) slog.Attr {
	return slog.String("error", err.Error())
}
