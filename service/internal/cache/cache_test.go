package cache

import "testing"

func TestReportTimeAcrossRuns(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	first := "2026-10-25T10:00:00Z"
	later := "2026-10-25T10:05:00Z"
	key := `{"api_base":"https://api.github.com","repos":["a/b"]}`

	if got := store.ReportTime(key, first, false); got != first {
		t.Fatalf("fresh report time = %q", got)
	}
	if got := store.ReportTime(key, later, true); got != first {
		t.Fatalf("cached report time = %q, want %q", got, first)
	}
	if got := store.ReportTime(key, later, false); got != later {
		t.Fatalf("refreshed report time = %q, want %q", got, later)
	}
	if got := store.ReportTime(key, first, true); got != later {
		t.Fatalf("cached refreshed report time = %q, want %q", got, later)
	}
	if got := store.ReportTime(key+"other", first, true); got != first {
		t.Fatalf("different report time = %q, want %q", got, first)
	}
}
