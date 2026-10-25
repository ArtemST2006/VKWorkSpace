// Package cache persists public GitHub responses for conditional requests.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Entry contains only a URL, public response metadata, and the response body.
// Authentication is never serialized.
type Entry struct {
	URL  string `json:"url"`
	ETag string `json:"etag"`
	Link string `json:"link"`
	Body []byte `json:"body"`
}

// Store keeps one JSON entry per request URL.
type Store struct{ Dir string }

func (s Store) path(url string) string {
	hash := sha256.Sum256([]byte(url))
	return filepath.Join(s.Dir, hex.EncodeToString(hash[:])+".json")
}

// Load returns a valid cached entry or reports a cache miss.
func (s Store) Load(url string) (Entry, bool) {
	if s.Dir == "" {
		return Entry{}, false
	}
	data, err := os.ReadFile(s.path(url))
	if err != nil {
		return Entry{}, false
	}
	var entry Entry
	if json.Unmarshal(data, &entry) != nil || entry.URL != url || entry.ETag == "" || !json.Valid(entry.Body) {
		return Entry{}, false
	}
	return entry, true
}

// Save atomically replaces an entry when its response has an ETag.
func (s Store) Save(entry Entry) error {
	if s.Dir == "" || entry.ETag == "" {
		return nil
	}
	if !json.Valid(entry.Body) {
		return errors.New("cache body is not valid JSON")
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return s.writeFile(s.path(entry.URL), data)
}

// ReportTime keeps the timestamp of the last fresh report for a cache key.
// A report assembled entirely from 304 responses reuses that timestamp.
func (s Store) ReportTime(key, current string, fromCache bool) string {
	if s.Dir == "" {
		return current
	}
	hash := sha256.Sum256([]byte(key))
	path := filepath.Join(s.Dir, "report-"+hex.EncodeToString(hash[:])+".txt")
	if fromCache {
		// #nosec G304 -- the filename is a SHA-256 digest inside the configured cache directory.
		if data, err := os.ReadFile(path); err == nil {
			if saved := string(data); saved != "" {
				if _, err := time.Parse(time.RFC3339, saved); err == nil {
					return saved
				}
			}
		}
	}
	_ = s.writeFile(path, []byte(current))
	return current
}

func (s Store) writeFile(path string, data []byte) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Dir, ".ghreport-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
