package debian

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

	const isoContent = "fake debian iso bytes"
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(isoContent)))

	mux := http.NewServeMux()
	mux.HandleFunc("/amd64/iso-cd/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="debian-13.6.0-amd64-netinst.iso">iso</a>`+
			`<a href="debian-edu-13.6.0-amd64-netinst.iso">edu iso</a>`)
	})
	mux.HandleFunc("/amd64/iso-cd/debian-13.6.0-amd64-netinst.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, isoContent)
	})
	mux.HandleFunc("/amd64/iso-cd/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  debian-13.6.0-amd64-netinst.iso\n", sum)
	})
	mux.HandleFunc("/amd64/iso-dvd/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="debian-13.6.0-amd64-DVD-1.iso">iso</a>`)
	})
	mux.HandleFunc("/amd64/iso-dvd/debian-13.6.0-amd64-DVD-1.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, isoContent)
	})
	mux.HandleFunc("/amd64/iso-dvd/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  debian-13.6.0-amd64-DVD-1.iso\n", sum)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	orig := baseURL
	baseURL = srv.URL + "/"
	t.Cleanup(func() { baseURL = orig })
}

func TestDebianCheckIgnoresEduAndMacVariants(t *testing.T) {
	withTestServer(t)

	res, err := Debian{}.Check(context.Background(), provider.Variant{Arch: "amd64"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "13.6.0" {
		t.Errorf("Check() version = %q, want %q", res.Version, "13.6.0")
	}
}

func TestDebianDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := Debian{}.Download(context.Background(), provider.Variant{Arch: "amd64"}, provider.CheckResult{Version: "13.6.0"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "debian-13.6.0-amd64-netinst.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestDebianLocalVersion(t *testing.T) {
	v := provider.Variant{Arch: "amd64"}

	version, ok := Debian{}.LocalVersion("debian-13.6.0-amd64-netinst.iso", v)
	if !ok || version != "13.6.0" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"13.6.0\", true)", version, ok)
	}

	if _, ok := (Debian{}).LocalVersion("debian-edu-13.6.0-amd64-netinst.iso", v); ok {
		t.Error("LocalVersion() matched the edu variant")
	}
}

func TestDebianDVDEdition(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()
	v := provider.Variant{Edition: "dvd", Arch: "amd64"}

	res, err := Debian{}.Check(context.Background(), v)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "13.6.0" {
		t.Errorf("Check() version = %q, want %q", res.Version, "13.6.0")
	}

	result, err := Debian{}.Download(context.Background(), v, provider.CheckResult{Version: "13.6.0"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "debian-13.6.0-amd64-DVD-1.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}

	version, ok := Debian{}.LocalVersion("debian-13.6.0-amd64-DVD-1.iso", v)
	if !ok || version != "13.6.0" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"13.6.0\", true)", version, ok)
	}
	// A netinst filename should never match the dvd variant.
	if _, ok := (Debian{}).LocalVersion("debian-13.6.0-amd64-netinst.iso", v); ok {
		t.Error("LocalVersion() matched a netinst filename against the dvd variant")
	}
}
