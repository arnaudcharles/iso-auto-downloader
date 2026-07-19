package parrotos

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
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="7.2/">7.2/</a><a href="7.3/">7.3/</a><a href="caine/">caine/</a><a href="latest/">latest/</a>`)
	})
	mux.HandleFunc("/7.3/Parrot-security-7.3_amd64.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "fake parrot security iso bytes")
	})
	mux.HandleFunc("/7.3/Parrot-home-7.3_amd64.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "fake parrot home iso bytes")
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	orig := baseURL
	baseURL = srv.URL + "/"
	t.Cleanup(func() { baseURL = orig })
}

func TestParrotOSCheckIgnoresNonVersionDirs(t *testing.T) {
	withTestServer(t)

	res, err := ParrotOS{}.Check(context.Background(), provider.Variant{Edition: "security"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "7.3" {
		t.Errorf("Check() version = %q, want %q", res.Version, "7.3")
	}
}

func TestParrotOSDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := ParrotOS{}.Download(context.Background(), provider.Variant{Edition: "security"}, provider.CheckResult{Version: "7.3"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "Parrot-security-7.3_amd64.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestParrotOSLocalVersion(t *testing.T) {
	version, ok := (ParrotOS{}).LocalVersion("Parrot-security-7.3_amd64.iso", provider.Variant{Edition: "security"})
	if !ok || version != "7.3" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"7.3\", true)", version, ok)
	}
}

func TestParrotOSHomeEdition(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := ParrotOS{}.Download(context.Background(), provider.Variant{Edition: "home"}, provider.CheckResult{Version: "7.3"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "Parrot-home-7.3_amd64.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}

	version, ok := (ParrotOS{}).LocalVersion("Parrot-home-7.3_amd64.iso", provider.Variant{Edition: "home"})
	if !ok || version != "7.3" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"7.3\", true)", version, ok)
	}
	// A security-edition file should never match the home variant.
	if _, ok := (ParrotOS{}).LocalVersion("Parrot-security-7.3_amd64.iso", provider.Variant{Edition: "home"}); ok {
		t.Error("LocalVersion() matched a security-edition filename against the home variant")
	}
}
