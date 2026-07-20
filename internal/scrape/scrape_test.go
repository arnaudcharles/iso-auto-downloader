package scrape

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// TestMain disables the real retry backoff for every test in this package
// (fetchRetryDelay defaults to a multi-second sleep in production, which
// would make the retry tests below needlessly slow).
func TestMain(m *testing.M) {
	fetchRetryDelay = func(int) time.Duration { return 0 }
	os.Exit(m.Run())
}

// TestFetchStringRetriesOnTransientNetworkErrorThenSucceeds is a regression
// test for a real report: Debian's Check failed on every architecture at
// once with "net/http: TLS handshake timeout" — a blip, since a manual
// retry moments later succeeded. FetchString (and by extension every
// provider's Check) previously gave up on the very first network-level
// failure.
func TestFetchStringRetriesOnTransientNetworkErrorThenSucceeds(t *testing.T) {
	var attempt int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempt, 1) <= 2 {
			// Simulate a connection-level failure (what a TLS handshake
			// timeout looks like to the client: Do() returns an error, no
			// response at all) by hijacking and closing without writing
			// anything.
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("test server ResponseWriter doesn't support hijacking")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			_ = conn.Close()
			return
		}
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	body, err := FetchString(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchString() error = %v", err)
	}
	if body != "ok" {
		t.Errorf("FetchString() = %q, want %q", body, "ok")
	}
	if got := atomic.LoadInt32(&attempt); got != 3 {
		t.Errorf("attempts = %d, want 3 (2 failures + 1 success)", got)
	}
}

// TestFetchStringGivesUpAfterMaxRetries confirms a persistently unreachable
// host still eventually returns an error rather than retrying forever.
func TestFetchStringGivesUpAfterMaxRetries(t *testing.T) {
	var attempt int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempt, 1)
		hj, _ := w.(http.Hijacker)
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.Close()
	}))
	defer srv.Close()

	if _, err := FetchString(context.Background(), srv.URL); err == nil {
		t.Fatal("FetchString() error = nil, want a network error")
	}
	if got := atomic.LoadInt32(&attempt); got != maxFetchRetries {
		t.Errorf("attempts = %d, want %d", got, maxFetchRetries)
	}
}

func TestLooksLikeVersion(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"24.04", true},
		{"26.04.1", true},
		{"8.10", true},
		{"20260714", true},
		{"25H2", true},
		{"unknown", false},
		{"", false},
		{"latest", false},
	}
	for _, c := range cases {
		if got := LooksLikeVersion(c.s); got != c.want {
			t.Errorf("LooksLikeVersion(%q) = %v, want %v", c.s, got, c.want)
		}
	}
}

// TestCompareVersionsWindowsFeatureUpdate is a regression test for a real
// report: the disk scan picking a stale, manually-downloaded Windows 11
// 23H2 ISO over a genuinely present, complete, newer 25H2 one this app had
// downloaded itself, because "25H2" and "23H2" each parse as a single
// non-numeric segment — strconv.Atoi fails on both, so a naive comparison
// silently treated them as equal (0), and whichever file the directory
// walk happened to see first "won" and was never displaced. A user ended
// up watching the app re-download an already-current 8GB ISO because the
// found version compared as neither older nor newer.
func TestCompareVersionsWindowsFeatureUpdate(t *testing.T) {
	if got := CompareVersions("25H2", "23H2"); got <= 0 {
		t.Errorf(`CompareVersions("25H2", "23H2") = %d, want > 0`, got)
	}
	if got := CompareVersions("23H2", "25H2"); got >= 0 {
		t.Errorf(`CompareVersions("23H2", "25H2") = %d, want < 0`, got)
	}
	if got := CompareVersions("25H2", "25H2"); got != 0 {
		t.Errorf(`CompareVersions("25H2", "25H2") = %d, want 0`, got)
	}
	if got := CompareVersions("25H2", "25H1"); got <= 0 {
		t.Errorf(`CompareVersions("25H2", "25H1") = %d, want > 0`, got)
	}
}

func TestCompareVersionsNumericDotted(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"24.04", "23.10", 1},
		{"24.04", "24.04", 0},
		{"24.04", "24.04.1", -1},
		{"24.04.1", "24.04", 1},
		{"8.10", "8.9", 1},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
