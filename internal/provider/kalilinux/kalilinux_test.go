package kalilinux

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

	const isoContent = "fake kali iso bytes"
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(isoContent)))

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="kali-linux-2026.2-installer-amd64.iso">iso</a>`+
			`<a href="kali-linux-2026.2-installer-netinst-amd64.iso">netinst</a>`+
			`<a href="kali-linux-2026.2-installer-everything-amd64.iso">everything</a>`)
	})
	mux.HandleFunc("/kali-linux-2026.2-installer-amd64.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, isoContent)
	})
	mux.HandleFunc("/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  kali-linux-2026.2-installer-amd64.iso\n", sum)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	orig := currentURL
	currentURL = srv.URL + "/"
	t.Cleanup(func() { currentURL = orig })
}

func TestKaliLinuxCheckIgnoresNetinstAndEverythingVariants(t *testing.T) {
	withTestServer(t)

	res, err := KaliLinux{}.Check(context.Background(), provider.Variant{Edition: "installer", Arch: "amd64"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "2026.2" {
		t.Errorf("Check() version = %q, want %q", res.Version, "2026.2")
	}
}

func TestKaliLinuxDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := KaliLinux{}.Download(context.Background(), provider.Variant{Edition: "installer", Arch: "amd64"}, provider.CheckResult{Version: "2026.2"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "kali-linux-2026.2-installer-amd64.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestKaliLinuxLocalVersion(t *testing.T) {
	v := provider.Variant{Edition: "installer", Arch: "amd64"}

	version, ok := KaliLinux{}.LocalVersion("kali-linux-2026.2-installer-amd64.iso", v)
	if !ok || version != "2026.2" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"2026.2\", true)", version, ok)
	}

	if _, ok := (KaliLinux{}).LocalVersion("kali-linux-2026.2-installer-netinst-amd64.iso", v); ok {
		t.Error("LocalVersion() matched the netinst variant")
	}
}
