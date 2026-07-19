package tails

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
)

func withTestServer(t *testing.T) {
	t.Helper()

	const isoContent = "fake tails iso bytes"

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="iuk/">iuk/</a><a href="tails-amd64-7.9.1/">tails-amd64-7.9.1/</a>`)
	})
	mux.HandleFunc("/tails-amd64-7.9.1/tails-amd64-7.9.1.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, isoContent)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	orig := baseURL
	baseURL = srv.URL + "/"
	t.Cleanup(func() { baseURL = orig })
}

func TestTailsCheckIgnoresNonVersionDirs(t *testing.T) {
	withTestServer(t)

	res, err := Tails{}.Check(context.Background(), provider.Variant{Arch: "amd64"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "7.9.1" {
		t.Errorf("Check() version = %q, want %q", res.Version, "7.9.1")
	}
}

func TestTailsDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := Tails{}.Download(context.Background(), provider.Variant{Arch: "amd64"}, provider.CheckResult{Version: "7.9.1"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "tails-amd64-7.9.1.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestTailsLocalVersion(t *testing.T) {
	version, ok := Tails{}.LocalVersion("tails-amd64-7.9.1.iso", provider.Variant{Arch: "amd64"})
	if !ok || version != "7.9.1" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"7.9.1\", true)", version, ok)
	}
}
