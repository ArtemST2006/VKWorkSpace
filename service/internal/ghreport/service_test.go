package ghreport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync"
	"testing"
	"time"

	"git.miem.hse.ru/blank5204543/vkworkspace/service/internal/cache"
	"git.miem.hse.ru/blank5204543/vkworkspace/service/internal/config"
	"git.miem.hse.ru/blank5204543/vkworkspace/service/internal/github"
)

func TestReportTimestampReusedOnlyForCachedPages(t *testing.T) {
	etag := `"v1"`
	var mu sync.Mutex
	statuses := make([]int, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		currentETag := etag
		mu.Unlock()
		if r.Header.Get("If-None-Match") == currentETag {
			mu.Lock()
			statuses = append(statuses, http.StatusNotModified)
			mu.Unlock()
			w.WriteHeader(http.StatusNotModified)
			return
		}
		mu.Lock()
		statuses = append(statuses, http.StatusOK)
		mu.Unlock()
		w.Header().Set("ETag", currentETag)
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()

	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cfg := config.Config{Repos: []string{"a/b"}, APIBase: base, CacheDir: dir}
	client := &github.Client{Cache: cache.Store{Dir: dir}}
	firstTime := time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	laterTime := firstTime.Add(5 * time.Minute)
	service := Service{Config: cfg, Client: client, Now: func() time.Time { return firstTime }}
	first, code := service.Run(context.Background())
	if code != "" {
		t.Fatalf("first run: %s", code)
	}
	service.Now = func() time.Time { return laterTime }
	second, code := service.Run(context.Background())
	if code != "" || !reflect.DeepEqual(first, second) {
		t.Fatalf("cached report changed: code=%s, first=%+v, second=%+v", code, first, second)
	}

	mu.Lock()
	etag = `"v2"`
	mu.Unlock()
	third, code := service.Run(context.Background())
	if code != "" || third.GeneratedAt != laterTime.Format(time.RFC3339) {
		t.Fatalf("fresh response did not update timestamp: code=%s, report=%+v", code, third)
	}
	mu.Lock()
	gotStatuses := append([]int(nil), statuses...)
	mu.Unlock()
	if !reflect.DeepEqual(gotStatuses, []int{http.StatusOK, http.StatusNotModified, http.StatusOK}) {
		t.Fatalf("response statuses: %v", statuses)
	}
}
