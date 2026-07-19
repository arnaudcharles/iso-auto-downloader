package ultimatebootcd

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

	mux := http.NewServeMux()
	mux.HandleFunc("/ubcd/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="ubcd539.iso">iso</a><a href="ubcdlive023b.iso">live iso</a>`)
	})
	mux.HandleFunc("/ubcd/ubcd539.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "fake ubcd iso bytes")
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	orig := mirrorURL
	mirrorURL = srv.URL + "/ubcd/"
	t.Cleanup(func() { mirrorURL = orig })
}

func TestUltimateBootCDCheckIgnoresLiveVariant(t *testing.T) {
	withTestServer(t)

	res, err := UltimateBootCD{}.Check(context.Background(), provider.Variant{})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "539" {
		t.Errorf("Check() version = %q, want %q", res.Version, "539")
	}
}

func TestUltimateBootCDDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := UltimateBootCD{}.Download(context.Background(), provider.Variant{}, provider.CheckResult{Version: "539"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "ubcd539.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestUltimateBootCDLocalVersion(t *testing.T) {
	version, ok := (UltimateBootCD{}).LocalVersion("ubcd539.iso", provider.Variant{})
	if !ok || version != "539" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"539\", true)", version, ok)
	}
	if _, ok := (UltimateBootCD{}).LocalVersion("ubcdlive023b.iso", provider.Variant{}); ok {
		t.Error("LocalVersion() matched the live variant filename")
	}
}
