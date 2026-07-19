package truenas

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

	const isoContent = "fake truenas iso bytes"
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(isoContent)))

	mux := http.NewServeMux()
	var srv *httptest.Server

	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<a href="%s/TrueNAS-SCALE-Goldeye/25.10.4/TrueNAS-SCALE-25.10.4.iso">stable</a>`+
			`<a href="%s/TrueNAS-26-BETA/26.0.0-BETA.2/TrueNAS-26.0.0-BETA.2.iso">beta</a>`, srv.URL, srv.URL)
	})
	mux.HandleFunc("/TrueNAS-SCALE-Goldeye/25.10.4/TrueNAS-SCALE-25.10.4.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, isoContent)
	})
	mux.HandleFunc("/TrueNAS-SCALE-Goldeye/25.10.4/TrueNAS-SCALE-25.10.4.iso.sha256", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, checksum)
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	orig := pageURL
	pageURL = srv.URL + "/download/"
	t.Cleanup(func() { pageURL = orig })

	return srv
}

func TestTrueNASCheckIgnoresBeta(t *testing.T) {
	withTestServer(t)

	res, err := TrueNAS{}.Check(context.Background(), provider.Variant{})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "25.10.4" {
		t.Errorf("Check() version = %q, want %q", res.Version, "25.10.4")
	}
}

func TestTrueNASDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := TrueNAS{}.Download(context.Background(), provider.Variant{}, provider.CheckResult{Version: "25.10.4"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "TrueNAS-SCALE-25.10.4.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestTrueNASLocalVersion(t *testing.T) {
	version, ok := (TrueNAS{}).LocalVersion("TrueNAS-SCALE-25.10.4.iso", provider.Variant{})
	if !ok || version != "25.10.4" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"25.10.4\", true)", version, ok)
	}
}
