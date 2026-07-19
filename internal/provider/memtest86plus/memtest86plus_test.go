package memtest86plus

import (
	"archive/zip"
	"bytes"
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

func buildZip(t *testing.T, isoName, isoContent string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create(isoName)
	if err != nil {
		t.Fatalf("zip.Create: %v", err)
	}
	if _, err := f.Write([]byte(isoContent)); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func withTestServer(t *testing.T) {
	t.Helper()

	zipBytes := buildZip(t, "mt86plus_8.10_x86_64.iso", "fake memtest iso bytes")
	sum := fmt.Sprintf("%x", sha256.Sum256(zipBytes))

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="/download/v8.10/mt86plus_8.10_x86_64.iso.zip">iso</a>`)
	})
	mux.HandleFunc("/download/v8.10/mt86plus_8.10_x86_64.iso.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Write(zipBytes)
	})
	mux.HandleFunc("/download/v8.10/sha256sum.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  v8.10/mt86plus_8.10_x86_64.iso.zip\n", sum)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	orig := baseURL
	baseURL = srv.URL + "/"
	t.Cleanup(func() { baseURL = orig })
}

func TestMemTest86PlusCheck(t *testing.T) {
	withTestServer(t)

	res, err := MemTest86Plus{}.Check(context.Background(), provider.Variant{Arch: "x86_64"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "8.10" {
		t.Errorf("Check() version = %q, want %q", res.Version, "8.10")
	}
}

func TestMemTest86PlusDownloadExtractsISO(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := MemTest86Plus{}.Download(context.Background(), provider.Variant{Arch: "x86_64"}, provider.CheckResult{Version: "8.10"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "mt86plus_8.10_x86_64.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("extracted iso missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mt86plus_8.10_x86_64.iso.zip")); !os.IsNotExist(err) {
		t.Errorf("expected zip to be removed after extraction, stat err = %v", err)
	}
}

func TestMemTest86PlusLocalVersion(t *testing.T) {
	v := provider.Variant{Arch: "x86_64"}

	version, ok := MemTest86Plus{}.LocalVersion("mt86plus_8.10_x86_64.iso", v)
	if !ok || version != "8.10" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"8.10\", true)", version, ok)
	}

	if _, ok := (MemTest86Plus{}).LocalVersion("mt86plus_8.10_i586.iso", v); ok {
		t.Error("LocalVersion() matched a different arch's filename")
	}
}
