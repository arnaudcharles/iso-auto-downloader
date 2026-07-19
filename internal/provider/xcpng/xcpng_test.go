package xcpng

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

	const isoContent = "fake xcp-ng iso bytes"
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(isoContent)))

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="8.2/">8.2/</a><a href="8.3/">8.3/</a><a href="drivers/">drivers/</a>`)
	})
	mux.HandleFunc("/8.3/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="xcp-ng-8.3.0-20250606.iso">v1</a>`+
			`<a href="xcp-ng-8.3.0-20250606.2.iso">v2</a>`+
			`<a href="xcp-ng-8.3.0-20250606-netinstall.iso">netinstall</a>`)
	})
	mux.HandleFunc("/8.3/xcp-ng-8.3.0-20250606.2.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, isoContent)
	})
	mux.HandleFunc("/8.3/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  xcp-ng-8.3.0-20250606.2.iso\n", sum)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	orig := baseURL
	baseURL = srv.URL + "/"
	t.Cleanup(func() { baseURL = orig })
}

func TestXCPngCheckPicksNewestBuildAndIgnoresNetinstall(t *testing.T) {
	withTestServer(t)

	res, err := XCPng{}.Check(context.Background(), provider.Variant{})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "8.3.0-20250606.2" {
		t.Errorf("Check() version = %q, want %q", res.Version, "8.3.0-20250606.2")
	}
}

func TestXCPngDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := XCPng{}.Download(context.Background(), provider.Variant{}, provider.CheckResult{Version: "8.3.0-20250606.2"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "xcp-ng-8.3.0-20250606.2.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestXCPngLocalVersion(t *testing.T) {
	version, ok := (XCPng{}).LocalVersion("xcp-ng-8.3.0-20250606.2.iso", provider.Variant{})
	if !ok || version != "8.3.0-20250606.2" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"8.3.0-20250606.2\", true)", version, ok)
	}
	if _, ok := (XCPng{}).LocalVersion("xcp-ng-8.3.0-20250606-netinstall.iso", provider.Variant{}); ok {
		t.Error("LocalVersion() matched the netinstall filename")
	}
}

// Real report: an ISO downloaded years ago (or from elsewhere) may just be
// "xcp-ng-8.3.0.iso" with no build-date suffix, unlike the mirror's current
// naming — LocalVersion must still recognize it.
func TestXCPngLocalVersionWithoutBuildSuffix(t *testing.T) {
	version, ok := (XCPng{}).LocalVersion("xcp-ng-8.3.0.iso", provider.Variant{})
	if !ok || version != "8.3.0" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"8.3.0\", true)", version, ok)
	}
}
