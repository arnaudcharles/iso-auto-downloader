package ubuntu

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

func withTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	const isoContent = "fake ubuntu iso bytes"
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(isoContent)))

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="24.04/">24.04/</a><a href="24.04.1/">24.04.1/</a><a href="22.04/">22.04/</a>`)
	})
	mux.HandleFunc("/24.04.1/ubuntu-24.04.1-desktop-amd64.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, isoContent)
	})
	mux.HandleFunc("/24.04.1/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s *ubuntu-24.04.1-desktop-amd64.iso\n", sum)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	orig := releasesURL
	releasesURL = srv.URL + "/"
	t.Cleanup(func() { releasesURL = orig })

	return srv
}

func TestUbuntuCheck(t *testing.T) {
	withTestServer(t)

	res, err := Ubuntu{}.Check(context.Background(), provider.Variant{Edition: "desktop", Arch: "amd64"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "24.04.1" {
		t.Errorf("Check() version = %q, want %q (highest of 22.04/24.04/24.04.1)", res.Version, "24.04.1")
	}
}

func TestUbuntuDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	res := provider.CheckResult{Version: "24.04.1"}
	result, err := Ubuntu{}.Download(context.Background(), provider.Variant{Edition: "desktop", Arch: "amd64"}, res, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	if result.Filename != "ubuntu-24.04.1-desktop-amd64.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestUbuntuDownloadChecksumMismatch(t *testing.T) {
	srv := withTestServer(t)
	_ = srv

	// Point at a version whose checksum file doesn't match its content.
	mux := http.NewServeMux()
	mux.HandleFunc("/24.99/ubuntu-24.99-desktop-amd64.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "corrupted content")
	})
	mux.HandleFunc("/24.99/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "0000000000000000000000000000000000000000000000000000000000000000 *ubuntu-24.99-desktop-amd64.iso\n")
	})
	badSrv := httptest.NewServer(mux)
	defer badSrv.Close()
	releasesURL = badSrv.URL + "/"

	dir := t.TempDir()
	_, err := Ubuntu{}.Download(context.Background(), provider.Variant{Edition: "desktop", Arch: "amd64"}, provider.CheckResult{Version: "24.99"}, dir, nil)
	if err == nil {
		t.Fatal("Download() expected checksum mismatch error, got nil")
	}
}

func TestUbuntuLocalVersion(t *testing.T) {
	v := provider.Variant{Edition: "desktop", Arch: "amd64"}

	version, ok := Ubuntu{}.LocalVersion("ubuntu-24.04.1-desktop-amd64.iso", v)
	if !ok || version != "24.04.1" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"24.04.1\", true)", version, ok)
	}

	if _, ok := (Ubuntu{}).LocalVersion("ubuntu-24.04.1-live-server-amd64.iso", v); ok {
		t.Error("LocalVersion() matched a different edition's filename")
	}
}
