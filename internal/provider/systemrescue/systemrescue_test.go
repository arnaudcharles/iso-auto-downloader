package systemrescue

import (
	"context"
	"crypto/sha256"
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

	const isoContent = "fake systemrescue iso bytes"
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(isoContent)))

	mux := http.NewServeMux()
	mux.HandleFunc("/Download/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="/systemrescue-13.01-amd64.iso">download</a>`)
	})
	mux.HandleFunc("/releases/13.01/systemrescue-13.01-amd64.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, isoContent)
	})
	mux.HandleFunc("/releases/13.01/systemrescue-13.01-amd64.iso.sha256", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s *systemrescue-13.01-amd64.iso\n", sum)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	origPage, origCDN := pageURL, cdnURL
	pageURL = srv.URL + "/Download/"
	cdnURL = srv.URL + "/releases/"
	t.Cleanup(func() { pageURL, cdnURL = origPage, origCDN })
}

func TestSystemRescueCheck(t *testing.T) {
	withTestServer(t)

	res, err := SystemRescue{}.Check(context.Background(), provider.Variant{Arch: "amd64"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "13.01" {
		t.Errorf("Check() version = %q, want %q", res.Version, "13.01")
	}
}

func TestSystemRescueDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := SystemRescue{}.Download(context.Background(), provider.Variant{Arch: "amd64"}, provider.CheckResult{Version: "13.01"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "systemrescue-13.01-amd64.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestSystemRescueLocalVersion(t *testing.T) {
	version, ok := (SystemRescue{}).LocalVersion("systemrescue-13.01-amd64.iso", provider.Variant{Arch: "amd64"})
	if !ok || version != "13.01" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"13.01\", true)", version, ok)
	}
}
