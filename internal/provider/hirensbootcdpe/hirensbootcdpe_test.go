package hirensbootcdpe

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

	const isoContent = "fake hirens iso bytes"
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(isoContent)))

	mux := http.NewServeMux()
	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "<p>Hiren's BootCD PE (v1.0.8)</p><p>SHA256: %s</p>", sum)
	})
	mux.HandleFunc("/files/HBCD_PE_x64.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, isoContent)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	origPage, origFile := pageURL, fileURL
	pageURL = srv.URL + "/download/"
	fileURL = srv.URL + "/files/HBCD_PE_x64.iso"
	t.Cleanup(func() { pageURL, fileURL = origPage, origFile })
}

func TestHirensBootCDPECheck(t *testing.T) {
	withTestServer(t)

	res, err := HirensBootCDPE{}.Check(context.Background(), provider.Variant{Arch: "x64"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "1.0.8" {
		t.Errorf("Check() version = %q, want %q", res.Version, "1.0.8")
	}
}

func TestHirensBootCDPEDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := HirensBootCDPE{}.Download(context.Background(), provider.Variant{Arch: "x64"}, provider.CheckResult{Version: "1.0.8"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "HBCD_PE_x64_v1.0.8.iso" {
		t.Errorf("Download() filename = %q, want the version baked in so LocalVersion can recover it later", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

// TestHirensBootCDPELocalVersionRecoversRealVersion is a regression test
// for a real report: Hiren's own filename never embeds a version, so every
// download landed under the exact same name with no way to tell which
// version was actually on disk without re-downloading it. Download now
// bakes the version Check() resolved into the saved filename (see
// versionedFilename), so LocalVersion can recover the real version like
// every other provider — no placeholder needed for anything this app
// downloads itself going forward.
func TestHirensBootCDPELocalVersionRecoversRealVersion(t *testing.T) {
	version, ok := (HirensBootCDPE{}).LocalVersion("HBCD_PE_x64_v1.0.8.iso", provider.Variant{Arch: "x64"})
	if !ok || version != "1.0.8" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"1.0.8\", true)", version, ok)
	}
}

// TestHirensBootCDPELocalVersionRecognizesLegacyFile is a regression test
// for the same real report as above: a file already present on disk (from
// before this app baked a version into the filename, or downloaded by hand
// from Hiren's own site, which always uses this exact fixed name) always
// showed "Not found", forcing a full re-download just to make it show up.
// It must still be recognized, via a placeholder version that can never be
// mistaken for a real "up to date" match.
func TestHirensBootCDPELocalVersionRecognizesLegacyFile(t *testing.T) {
	version, ok := (HirensBootCDPE{}).LocalVersion("HBCD_PE_x64.iso", provider.Variant{Arch: "x64"})
	if !ok {
		t.Fatal("LocalVersion() should recognize the legacy official filename as present")
	}
	if version == "" || version == "1.0.8" {
		t.Errorf("LocalVersion() version = %q, want a placeholder that can never string-equal a real version", version)
	}

	if _, ok := (HirensBootCDPE{}).LocalVersion("something-else.iso", provider.Variant{Arch: "x64"}); ok {
		t.Error("LocalVersion() matched an unrelated filename")
	}
}
