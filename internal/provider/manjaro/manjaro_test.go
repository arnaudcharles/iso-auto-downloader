package manjaro

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

	const isoContent = "fake manjaro iso bytes"
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(isoContent)))

	mux := http.NewServeMux()
	var srv *httptest.Server

	mux.HandleFunc("/products/download/x86/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<a href="%s/kde/26.0.4/manjaro-kde-26.0.4-260327-linux618.iso">KDE</a>`, srv.URL)
	})
	mux.HandleFunc("/kde/26.0.4/manjaro-kde-26.0.4-260327-linux618.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, isoContent)
	})
	mux.HandleFunc("/kde/26.0.4/manjaro-kde-26.0.4-260327-linux618.iso.sha256", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  manjaro-kde-26.0.4-260327-linux618.iso\n", sum)
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	origPage, origDomain := pageURL, downloadDomain
	pageURL = srv.URL + "/products/download/x86/"
	downloadDomain = srv.URL
	t.Cleanup(func() { pageURL, downloadDomain = origPage, origDomain })

	return srv
}

func TestManjaroCheck(t *testing.T) {
	withTestServer(t)

	res, err := Manjaro{}.Check(context.Background(), provider.Variant{Edition: "kde"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "26.0.4" {
		t.Errorf("Check() version = %q, want %q", res.Version, "26.0.4")
	}
}

func TestManjaroDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := Manjaro{}.Download(context.Background(), provider.Variant{Edition: "kde"}, provider.CheckResult{Version: "26.0.4"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "manjaro-kde-26.0.4-260327-linux618.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestManjaroLocalVersion(t *testing.T) {
	v := provider.Variant{Edition: "kde"}

	version, ok := Manjaro{}.LocalVersion("manjaro-kde-26.0.4-260327-linux618.iso", v)
	if !ok || version != "26.0.4" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"26.0.4\", true)", version, ok)
	}

	if _, ok := (Manjaro{}).LocalVersion("manjaro-xfce-26.0.4-260327-linux618.iso", v); ok {
		t.Error("LocalVersion() matched a different edition's filename")
	}
}
