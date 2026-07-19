package alpinelinux

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

	const isoContent = "fake alpine iso bytes"
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(isoContent)))

	mux := http.NewServeMux()
	mux.HandleFunc("/x86_64/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="alpine-standard-3.24.1-x86_64.iso">iso</a>`+
			`<a href="alpine-standard-3.24.0_rc2-x86_64.iso">rc</a>`)
	})
	mux.HandleFunc("/x86_64/alpine-standard-3.24.1-x86_64.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, isoContent)
	})
	mux.HandleFunc("/x86_64/alpine-standard-3.24.1-x86_64.iso.sha256", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  alpine-standard-3.24.1-x86_64.iso\n", sum)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	orig := baseURL
	baseURL = srv.URL + "/"
	t.Cleanup(func() { baseURL = orig })
}

func TestAlpineLinuxCheckIgnoresRC(t *testing.T) {
	withTestServer(t)

	res, err := AlpineLinux{}.Check(context.Background(), provider.Variant{Edition: "standard", Arch: "x86_64"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "3.24.1" {
		t.Errorf("Check() version = %q, want %q", res.Version, "3.24.1")
	}
}

func TestAlpineLinuxDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := AlpineLinux{}.Download(context.Background(), provider.Variant{Edition: "standard", Arch: "x86_64"}, provider.CheckResult{Version: "3.24.1"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "alpine-standard-3.24.1-x86_64.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestAlpineLinuxLocalVersion(t *testing.T) {
	version, ok := (AlpineLinux{}).LocalVersion("alpine-standard-3.24.1-x86_64.iso", provider.Variant{Edition: "standard", Arch: "x86_64"})
	if !ok || version != "3.24.1" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"3.24.1\", true)", version, ok)
	}
}
