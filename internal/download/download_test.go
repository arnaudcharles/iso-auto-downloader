package download

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestMain disables the real retry backoff for every test in this package
// (retryDelay defaults to a multi-second sleep in production, which would
// make the retry-exhaustion tests below needlessly slow).
func TestMain(m *testing.M) {
	retryDelay = func(int) time.Duration { return 0 }
	os.Exit(m.Run())
}

// TestDownloadRetriesOnChecksumMismatchThenSucceeds is a regression test for
// a real report: SourceForge's redirect hands out a different geo-mirror on
// every request, and one mirror served a corrupt/stale file that failed
// checksum verification. A fresh request (which lands on a different
// mirror) often succeeds, so Download should retry from scratch on a
// checksum mismatch instead of failing after a single bad mirror.
func TestDownloadRetriesOnChecksumMismatchThenSucceeds(t *testing.T) {
	good := []byte("the real iso bytes")
	bad := []byte("corrupt data from a stale mirror")
	sum := fmt.Sprintf("%x", sha256.Sum256(good))

	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&requests, 1)
		if n < maxDownloadRetries {
			_, _ = w.Write(bad)
			return
		}
		_, _ = w.Write(good)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.iso")
	size, err := Download(context.Background(), Options{
		URL:      srv.URL,
		DestPath: dest,
		Checksum: &Checksum{Algo: SHA256, Hex: sum},
	})
	if err != nil {
		t.Fatalf("Download() error = %v, want success after retrying", err)
	}
	if size != int64(len(good)) {
		t.Errorf("size = %d, want %d", size, len(good))
	}
	if got := int(atomic.LoadInt32(&requests)); got != maxDownloadRetries {
		t.Errorf("requests = %d, want %d (should succeed on the last allowed attempt)", got, maxDownloadRetries)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(data) != string(good) {
		t.Errorf("dest content = %q, want %q", data, good)
	}
}

// TestDownloadRetriesTransientNetworkErrorAndResumes is a regression test
// for a real report: a mirror reset the connection at 622 of 649 MB, and
// Download gave up immediately instead of retrying — only checksum
// mismatches were retryable, so a plain connection reset (or any other
// mid-transfer network error) went straight to the caller as a final
// failure, even though the .part file it left behind was perfectly capable
// of resuming via a Range request. This confirms a reset mid-transfer now
// gets retried, and that the retry actually resumes (a Range request for
// the missing tail) instead of restarting the whole transfer from scratch.
func TestDownloadRetriesTransientNetworkErrorAndResumes(t *testing.T) {
	first := strings.Repeat("A", 50)
	second := strings.Repeat("B", 50)
	full := first + second
	wantSum := fmt.Sprintf("%x", sha256.Sum256([]byte(full)))

	var attempt int32
	var gotRange string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempt, 1) == 1 {
			// First attempt: send half the body, then cut the connection —
			// simulating the mid-transfer reset from the real report.
			w.Header().Set("Content-Length", strconv.Itoa(len(full)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(first))
			w.(http.Flusher).Flush()
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

		gotRange = r.Header.Get("Range")
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", len(first), len(full)-1, len(full)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte(second))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.iso")
	size, err := Download(context.Background(), Options{
		URL:      srv.URL,
		DestPath: dest,
		Checksum: &Checksum{Algo: SHA256, Hex: wantSum},
	})
	if err != nil {
		t.Fatalf("Download() error = %v, want success after resuming from the reset", err)
	}
	if size != int64(len(full)) {
		t.Errorf("size = %d, want %d", size, len(full))
	}
	if gotRange != "bytes=50-" {
		t.Errorf("retry Range header = %q, want %q (must resume, not restart from scratch)", gotRange, "bytes=50-")
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(data) != full {
		t.Errorf("dest content = %q, want %q", data, full)
	}
}

// TestDownloadFailsAfterExhaustingChecksumRetries confirms a persistently
// bad server (every mirror stale) still surfaces ErrChecksumMismatch,
// rather than retrying forever.
func TestDownloadFailsAfterExhaustingChecksumRetries(t *testing.T) {
	bad := []byte("always corrupt")
	wantSum := fmt.Sprintf("%x", sha256.Sum256([]byte("never matches")))

	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		_, _ = w.Write(bad)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.iso")
	_, err := Download(context.Background(), Options{
		URL:      srv.URL,
		DestPath: dest,
		Checksum: &Checksum{Algo: SHA256, Hex: wantSum},
	})
	var mismatch *ErrChecksumMismatch
	if !errors.As(err, &mismatch) {
		t.Fatalf("error = %v, want *ErrChecksumMismatch", err)
	}
	if got := int(atomic.LoadInt32(&requests)); got != maxDownloadRetries {
		t.Errorf("requests = %d, want %d (should give up after exhausting retries)", got, maxDownloadRetries)
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("dest file should not exist after a failed download")
	}
}
