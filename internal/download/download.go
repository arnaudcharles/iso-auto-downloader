// Package download fetches a file over HTTP with resume support and
// verifies its checksum before atomically replacing the destination path,
// so a failed or corrupted download never clobbers a good existing file.
package download

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxDownloadRetries bounds how many times a bad attempt (checksum
// mismatch, or a transient network error — see ErrTransientNetwork) is
// retried before giving up. Some mirror networks (e.g. SourceForge's
// redirect, which hands out a different geo-mirror on every request) put a
// stale or corrupt copy in rotation from time to time; a fresh request,
// ideally landing on a different mirror, often succeeds where the last one
// didn't. Network errors retry against the same partial file via Range
// (see download() below), so those retries are cheap regardless of mirror.
const maxDownloadRetries = 5

// retryDelay returns how long to wait before retry attempt+1, given the
// attempt (1-indexed) that just failed. A var (not a plain function) so
// tests can zero it out instead of spending real wall-clock time on retries.
var retryDelay = func(attempt int) time.Duration {
	return time.Duration(attempt) * 2 * time.Second
}

// Algo identifies a supported checksum algorithm.
type Algo string

const (
	MD5    Algo = "md5"
	SHA1   Algo = "sha1"
	SHA256 Algo = "sha256"
	SHA512 Algo = "sha512"
)

func (a Algo) newHash() (hash.Hash, error) {
	switch a {
	case MD5:
		return md5.New(), nil
	case SHA1:
		return sha1.New(), nil
	case SHA256:
		return sha256.New(), nil
	case SHA512:
		return sha512.New(), nil
	default:
		return nil, fmt.Errorf("download: unsupported checksum algorithm %q", a)
	}
}

// Checksum is the expected digest a completed download must match.
type Checksum struct {
	Algo Algo
	Hex  string
}

// ProgressFunc is invoked as bytes are written to disk; total is 0 if the
// server didn't report a Content-Length.
type ProgressFunc func(downloaded, total int64)

// Options configures a single download.
type Options struct {
	URL      string
	DestPath string
	Checksum *Checksum
	Progress ProgressFunc
	Client   *http.Client
}

// ErrChecksumMismatch is returned when the downloaded file doesn't match
// the expected checksum. The partial/complete temp file is removed.
type ErrChecksumMismatch struct {
	Want, Got string
}

func (e *ErrChecksumMismatch) Error() string {
	return fmt.Sprintf("download: checksum mismatch: want %s, got %s", e.Want, e.Got)
}

// ErrTransientNetwork wraps a network-level failure while a request was in
// flight or a transfer was mid-copy: a dropped/reset connection, a timeout,
// a DNS hiccup. Real report: a mirror reset the connection at 622 of 649
// MB — the .part file survives a network error (see download() below), so
// a retry against it resumes with a Range request instead of restarting the
// whole transfer; this class of error just needs to actually reach that
// retry loop, which is the point of wrapping it distinctly here.
type ErrTransientNetwork struct {
	Err error
}

func (e *ErrTransientNetwork) Error() string {
	return fmt.Sprintf("download: transient network error: %v", e.Err)
}

func (e *ErrTransientNetwork) Unwrap() error {
	return e.Err
}

func isRetryable(err error) bool {
	var mismatch *ErrChecksumMismatch
	var transient *ErrTransientNetwork
	return errors.As(err, &mismatch) || errors.As(err, &transient)
}

// Download fetches opts.URL to opts.DestPath, resuming a previous partial
// download if one is found at DestPath+".part". On success DestPath is
// replaced atomically; on failure DestPath is left untouched. A checksum
// mismatch or a transient network error (see ErrTransientNetwork) is
// retried, with a short backoff between attempts, up to maxDownloadRetries
// times before being returned to the caller.
func Download(ctx context.Context, opts Options) (int64, error) {
	var lastErr error
	for attempt := 1; attempt <= maxDownloadRetries; attempt++ {
		size, err := download(ctx, opts)
		if err == nil {
			return size, nil
		}
		if !isRetryable(err) {
			return 0, err
		}
		lastErr = err

		if attempt < maxDownloadRetries {
			select {
			case <-time.After(retryDelay(attempt)):
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		}
	}
	return 0, lastErr
}

func download(ctx context.Context, opts Options) (int64, error) {
	client := opts.Client
	if client == nil {
		client = http.DefaultClient
	}

	if err := os.MkdirAll(filepath.Dir(opts.DestPath), 0o755); err != nil {
		return 0, fmt.Errorf("download: create dest dir: %w", err)
	}

	partPath := opts.DestPath + ".part"
	var startOffset int64
	if fi, err := os.Stat(partPath); err == nil {
		startOffset = fi.Size()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.URL, nil)
	if err != nil {
		return 0, fmt.Errorf("download: build request: %w", err)
	}
	if startOffset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startOffset))
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, &ErrTransientNetwork{Err: fmt.Errorf("request %s: %w", opts.URL, err)}
	}
	defer func() { _ = resp.Body.Close() }()

	var flags int
	switch resp.StatusCode {
	case http.StatusOK:
		// Server ignored our Range request (or we didn't send one): start over.
		startOffset = 0
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	case http.StatusPartialContent:
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	default:
		return 0, fmt.Errorf("download: unexpected status %s for %s", resp.Status, opts.URL)
	}

	total := startOffset + resp.ContentLength
	if resp.ContentLength < 0 {
		total = 0
	}

	f, err := os.OpenFile(partPath, flags, 0o644)
	if err != nil {
		return 0, fmt.Errorf("download: open %s: %w", partPath, err)
	}

	downloaded := startOffset
	pw := &progressWriter{w: f, progress: opts.Progress, downloaded: &downloaded, total: total}
	_, copyErr := io.Copy(pw, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		return 0, &ErrTransientNetwork{Err: fmt.Errorf("write %s: %w", partPath, copyErr)}
	}
	if closeErr != nil {
		return 0, fmt.Errorf("download: close %s: %w", partPath, closeErr)
	}

	if opts.Checksum != nil {
		got, err := hashFile(partPath, opts.Checksum.Algo)
		if err != nil {
			return 0, err
		}
		if !strings.EqualFold(got, opts.Checksum.Hex) {
			_ = os.Remove(partPath)
			return 0, &ErrChecksumMismatch{Want: opts.Checksum.Hex, Got: got}
		}
	}

	if err := os.Rename(partPath, opts.DestPath); err != nil {
		return 0, fmt.Errorf("download: finalize %s: %w", opts.DestPath, err)
	}

	fi, err := os.Stat(opts.DestPath)
	if err != nil {
		return 0, fmt.Errorf("download: stat finalized file: %w", err)
	}
	return fi.Size(), nil
}

// HashFile computes the hex digest of the file at path using algo. Exported
// so callers can re-verify a file already on disk (e.g. a standalone
// "validate what I already downloaded" action) without going through
// Download again.
func HashFile(path string, algo Algo) (string, error) {
	return hashFile(path, algo)
}

func hashFile(path string, algo Algo) (string, error) {
	h, err := algo.newHash()
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("download: open %s for checksum: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("download: hash %s: %w", path, err)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

type progressWriter struct {
	w          io.Writer
	progress   ProgressFunc
	downloaded *int64
	total      int64
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	*p.downloaded += int64(n)
	if p.progress != nil {
		p.progress(*p.downloaded, p.total)
	}
	return n, err
}
