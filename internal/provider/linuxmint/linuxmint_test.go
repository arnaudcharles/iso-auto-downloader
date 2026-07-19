package linuxmint

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

	const isoContent = "fake mint iso bytes"
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(isoContent)))

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="22.3/">22.3/</a><a href="22.2/">22.2/</a><a href="../">..</a>`)
	})
	mux.HandleFunc("/22.3/linuxmint-22.3-cinnamon-64bit.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, isoContent)
	})
	mux.HandleFunc("/22.3/sha256sum.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s *linuxmint-22.3-cinnamon-64bit.iso\n", sum)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	orig := baseURL
	baseURL = srv.URL + "/"
	t.Cleanup(func() { baseURL = orig })
}

func TestLinuxMintCheck(t *testing.T) {
	withTestServer(t)

	res, err := LinuxMint{}.Check(context.Background(), provider.Variant{Edition: "cinnamon"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "22.3" {
		t.Errorf("Check() version = %q, want %q", res.Version, "22.3")
	}
}

func TestLinuxMintDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := LinuxMint{}.Download(context.Background(), provider.Variant{Edition: "cinnamon"}, provider.CheckResult{Version: "22.3"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "linuxmint-22.3-cinnamon-64bit.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestLinuxMintLocalVersion(t *testing.T) {
	v := provider.Variant{Edition: "cinnamon"}

	version, ok := LinuxMint{}.LocalVersion("linuxmint-22.3-cinnamon-64bit.iso", v)
	if !ok || version != "22.3" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"22.3\", true)", version, ok)
	}

	if _, ok := (LinuxMint{}).LocalVersion("linuxmint-22.3-mate-64bit.iso", v); ok {
		t.Error("LocalVersion() matched a different edition's filename")
	}
}
