package rockylinux

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

	const isoContent = "fake rocky iso bytes"
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(isoContent)))

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="9/">9/</a><a href="10/">10/</a><a href="9.8/">9.8/</a>`)
	})
	mux.HandleFunc("/10/isos/x86_64/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="Rocky-10-latest-x86_64-dvd.iso">latest alias</a>`+
			`<a href="Rocky-10.2-x86_64-dvd1.iso">dvd1</a>`+
			`<a href="Rocky-10.2-x86_64-boot.iso">boot</a>`+
			`<a href="Rocky-10.2-x86_64-minimal.iso">minimal</a>`)
	})
	mux.HandleFunc("/10/isos/x86_64/Rocky-10.2-x86_64-dvd1.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, isoContent)
	})
	mux.HandleFunc("/10/isos/x86_64/CHECKSUM", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "SHA256 (Rocky-10.2-x86_64-dvd1.iso) = %s\n", sum)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	orig := baseURL
	baseURL = srv.URL + "/"
	t.Cleanup(func() { baseURL = orig })
}

func TestRockyLinuxCheckHandlesTrailingOneAndIgnoresLatestAlias(t *testing.T) {
	withTestServer(t)

	res, err := RockyLinux{}.Check(context.Background(), provider.Variant{Edition: "dvd", Arch: "x86_64"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "10.2" {
		t.Errorf("Check() version = %q, want %q", res.Version, "10.2")
	}
}

func TestRockyLinuxDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := RockyLinux{}.Download(context.Background(), provider.Variant{Edition: "dvd", Arch: "x86_64"}, provider.CheckResult{Version: "10.2"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "Rocky-10.2-x86_64-dvd1.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestRockyLinuxLocalVersion(t *testing.T) {
	v := provider.Variant{Edition: "dvd", Arch: "x86_64"}
	version, ok := (RockyLinux{}).LocalVersion("Rocky-10.2-x86_64-dvd1.iso", v)
	if !ok || version != "10.2" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"10.2\", true)", version, ok)
	}
	version, ok = (RockyLinux{}).LocalVersion("Rocky-9.8-x86_64-dvd.iso", v)
	if !ok || version != "9.8" {
		t.Errorf("LocalVersion() (no trailing 1) = (%q, %v), want (\"9.8\", true)", version, ok)
	}
}
