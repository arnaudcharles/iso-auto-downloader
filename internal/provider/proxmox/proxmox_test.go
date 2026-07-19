package proxmox

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

	const isoContent = "fake proxmox ve iso bytes"
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(isoContent)))

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Real listing includes other Proxmox products and older builds —
		// Check must pick only the newest proxmox-ve one.
		fmt.Fprint(w, `<a href="./proxmox-ve_9.2-1.iso">iso</a>`+
			`<a href="./proxmox-ve_9.1-1.iso">iso</a>`+
			`<a href="./proxmox-ve_8.4-1.iso">iso</a>`+
			`<a href="./proxmox-backup-server_4.2-1.iso">other product</a>`)
	})
	mux.HandleFunc("/proxmox-ve_9.2-1.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, isoContent)
	})
	mux.HandleFunc("/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  proxmox-ve_9.2-1.iso\n", sum)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	orig := baseURL
	baseURL = srv.URL + "/"
	t.Cleanup(func() { baseURL = orig })
}

func TestProxmoxCheckPicksNewestVersionIgnoringBuildSuffix(t *testing.T) {
	withTestServer(t)

	res, err := Proxmox{}.Check(context.Background(), provider.Variant{Arch: "amd64"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "9.2-1" {
		t.Errorf("Check() version = %q, want %q", res.Version, "9.2-1")
	}
}

func TestProxmoxDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := Proxmox{}.Download(context.Background(), provider.Variant{Arch: "amd64"}, provider.CheckResult{Version: "9.2-1"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "proxmox-ve_9.2-1.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestProxmoxLocalVersion(t *testing.T) {
	version, ok := Proxmox{}.LocalVersion("proxmox-ve_9.2-1.iso", provider.Variant{Arch: "amd64"})
	if !ok || version != "9.2-1" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"9.2-1\", true)", version, ok)
	}
	if _, ok := (Proxmox{}).LocalVersion("proxmox-backup-server_4.2-1.iso", provider.Variant{Arch: "amd64"}); ok {
		t.Error("LocalVersion() matched a different Proxmox product's filename")
	}
}
