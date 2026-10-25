package github

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRetryRateLimitAndTrace(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q", got)
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" || r.Header.Get("X-GitHub-Api-Version") != APIVersion || r.Header.Get("User-Agent") == "" {
			t.Error("required headers missing")
		}
		if attempts == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()
	var trace bytes.Buffer
	var waits []time.Duration
	client := Client{Token: "secret", Trace: &trace, Sleep: func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		return nil
	}}
	_, code := client.Fetch(context.Background(), server.URL)
	if code != "" || attempts != 2 || len(waits) != 1 || waits[0] != 2*time.Second {
		t.Fatalf("code=%q, attempts=%d, waits=%v", code, attempts, waits)
	}
	if strings.Contains(trace.String(), "secret") {
		t.Fatal("token leaked into trace")
	}
	var records []TraceRecord
	for _, line := range bytes.Split(bytes.TrimSpace(trace.Bytes()), []byte("\n")) {
		var record TraceRecord
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if len(records) != 2 || records[0].Status != 429 || records[1].Status != 200 || records[1].Attempt != 2 {
		t.Fatalf("unexpected trace: %+v", records)
	}
}

func TestNetworkAndServerRetries(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 4 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()
	var waits []time.Duration
	client := Client{Sleep: func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		return nil
	}}
	_, code := client.Fetch(context.Background(), server.URL)
	if code != "" || attempts != 4 || len(waits) != 3 || waits[0] != time.Second || waits[1] != 2*time.Second || waits[2] != 4*time.Second {
		t.Fatalf("code=%q, attempts=%d, waits=%v", code, attempts, waits)
	}
}

func TestUnexpected304AndInvalidJSON(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   ErrorCode
	}{
		{http.StatusNotModified, "", InvalidResponse},
		{http.StatusOK, "{bad", InvalidResponse},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		_, got := (&Client{}).Fetch(context.Background(), server.URL)
		server.Close()
		if got != tc.want {
			t.Errorf("status %d: got %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestParseNext(t *testing.T) {
	link := `<https://api.example.test/issues?page=7>; rel="last", <https://api.example.test/issues?page=2>; rel="next"`
	next, err := parseNext("https://api.example.test/issues", link)
	if err != nil || next != "https://api.example.test/issues?page=2" {
		t.Fatalf("next=%q, err=%v", next, err)
	}
}

func TestRedirects(t *testing.T) {
	for _, status := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/redirect" {
					w.Header().Set("Location", "/target")
					w.WriteHeader(status)
					return
				}
				if r.URL.Path != "/target" || r.Header.Get("User-Agent") != UserAgent {
					t.Errorf("bad redirected request: %s", r.URL)
				}
				_, _ = w.Write([]byte("[]"))
			}))
			defer server.Close()
			var trace bytes.Buffer
			_, code := (&Client{Trace: &trace}).Fetch(context.Background(), server.URL+"/redirect")
			if code != "" {
				t.Fatalf("redirect %d failed: %s", status, code)
			}
			var records []TraceRecord
			for _, line := range bytes.Split(bytes.TrimSpace(trace.Bytes()), []byte("\n")) {
				var record TraceRecord
				if err := json.Unmarshal(line, &record); err != nil {
					t.Fatal(err)
				}
				records = append(records, record)
			}
			if len(records) != 2 || records[0].Status != status || records[0].URL != server.URL+"/redirect" || records[0].Attempt != 1 || records[1].Status != http.StatusOK || records[1].URL != server.URL+"/target" || records[1].Attempt != 2 {
				t.Fatalf("redirect trace: %+v", records)
			}
		})
	}
}
