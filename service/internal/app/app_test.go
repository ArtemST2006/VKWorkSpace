package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"git.miem.hse.ru/blank5204543/vkworkspace/service/internal/report"
)

func issue(number int, created string) string {
	return fmt.Sprintf(`{"number":%d,"title":"Issue %d","user":{"login":"octocat"},"labels":[{"name":"bug"}],"created_at":%q,"html_url":"https://example.org/issues/%d"}`, number, number, created, number)
}

func run(t *testing.T, base string, args ...string) (int, []byte, []byte) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, func(key string) string {
		if key == "GITHUB_API_BASE_URL" {
			return base
		}
		return ""
	}, &stdout, &stderr)
	return code, stdout.Bytes(), stderr.Bytes()
}

func TestPaginationFilteringDeduplicationAndLimit(t *testing.T) {
	server := httptest.NewUnstartedServer(nil)
	defer server.Close()
	requests := 0
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("state") != "open" || r.URL.Query().Get("sort") != "created" || r.URL.Query().Get("direction") != "asc" || r.URL.Query().Get("per_page") != "100" || r.URL.Query().Has("since") {
			t.Errorf("wrong query: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("page") == "2" {
			_, _ = fmt.Fprintf(w, "[%s,%s]", issue(2, "2026-01-02T00:00:00Z"), issue(1, "2026-01-02T00:00:00Z"))
			return
		}
		next := *r.URL
		query := next.Query()
		query.Set("page", "2")
		next.RawQuery = query.Encode()
		w.Header().Set("Link", fmt.Sprintf(`<%s%s>; rel="next"`, server.URL, next.String()))
		_, _ = fmt.Fprintf(w, `[%s,{"pull_request":null},%s]`, issue(9, "2025-12-31T00:00:00Z"), issue(2, "2026-01-02T00:00:00Z"))
	})
	server.Start()
	code, output, diagnostic := run(t, server.URL, "--repos", "a/b", "--since", "2026-01-01", "--limit", "2")
	if code != 0 || requests != 2 {
		t.Fatalf("code=%d, requests=%d, stderr=%s", code, requests, diagnostic)
	}
	var result report.Report
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Repos) != 1 || result.Repos[0].Count != 2 || result.Repos[0].Issues[0].Number != 1 || result.Repos[0].Issues[1].Number != 2 {
		t.Fatalf("unexpected report: %s", output)
	}
}

func TestCacheAndTraceAcrossRuns(t *testing.T) {
	server := httptest.NewUnstartedServer(nil)
	defer server.Close()
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		etag := `"v1-page-1"`
		number := 1
		if page == "2" {
			etag = `"v1-page-2"`
			number = 2
		}
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		if page == "" {
			next := *r.URL
			query := next.Query()
			query.Set("page", "2")
			next.RawQuery = query.Encode()
			w.Header().Set("Link", fmt.Sprintf(`<%s%s>; rel="next"`, server.URL, next.String()))
		}
		_, _ = fmt.Fprintf(w, "[%s]", issue(number, "2026-01-01T00:00:00Z"))
	})
	server.Start()
	cacheDir := t.TempDir()
	traceFile := t.TempDir() + "/trace.jsonl"
	args := []string{"--repos", "a/b", "--cache-dir", cacheDir, "--trace", traceFile}
	firstCode, first, _ := run(t, server.URL, args...)
	secondCode, second, _ := run(t, server.URL, args...)
	if firstCode != 0 || secondCode != 0 {
		t.Fatalf("exit codes: %d, %d", firstCode, secondCode)
	}
	var firstReport report.Report
	if json.Unmarshal(first, &firstReport) != nil || firstReport.Repos[0].Count != 2 || !bytes.Equal(first, second) {
		t.Fatalf("cache changed report: %s / %s", first, second)
	}
	// #nosec G304 -- traceFile is created inside this test's temporary directory.
	data, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"from_cache":true`)) || !bytes.Contains(data, []byte(`"status":304`)) {
		t.Fatalf("cache trace missing: %s", data)
	}
	if bytes.Count(data, []byte(`"from_cache":true`)) != 2 {
		t.Fatalf("not all pages used cache: %s", data)
	}
}

func TestFatalUnauthorizedAndPartialNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/missing/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if strings.Contains(r.URL.Path, "/unauthorized/") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()
	code, output, _ := run(t, server.URL, "--repos", "missing/repo,ok/repo")
	if code != 1 || !bytes.Contains(output, []byte(`"error":"not_found"`)) {
		t.Fatalf("partial failure: code=%d, output=%s", code, output)
	}
	code, output, _ = run(t, server.URL, "--repos", "unauthorized/repo")
	if code != 3 || strings.TrimSpace(string(output)) != `{"error":"unauthorized"}` {
		t.Fatalf("unauthorized: code=%d, output=%s", code, output)
	}
}

func TestInvalidArgumentsLeaveStdoutEmpty(t *testing.T) {
	code, output, diagnostic := run(t, "http://example.test", "--repos", "a/b", "--limit", "0")
	if code != 2 || len(output) != 0 || len(diagnostic) == 0 {
		t.Fatalf("code=%d, stdout=%q, stderr=%q", code, output, diagnostic)
	}
}

func TestReportWrittenToFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()
	path := t.TempDir() + "/report.json"
	code, output, diagnostic := run(t, server.URL, "--repos", "a/b", "--out", path)
	if code != 0 || len(output) != 0 {
		t.Fatalf("code=%d, stdout=%q, stderr=%q", code, output, diagnostic)
	}
	// #nosec G304 -- path is created inside this test's temporary directory.
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(data, []byte(`"repo":"a/b"`)) {
		t.Fatalf("report file: %s, error: %v", data, err)
	}
}
