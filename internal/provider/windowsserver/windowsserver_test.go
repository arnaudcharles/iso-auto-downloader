package windowsserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
)

// ws2025 is the edition used across these tests.
func ws2025() WindowsServer {
	for _, ed := range editions {
		if ed.id == "windows-server-2025" {
			return WindowsServer{ed: ed}
		}
	}
	panic("windows-server-2025 edition not found")
}

func withTestServer(t *testing.T) {
	t.Helper()

	const isoContent = "fake windows server iso bytes"

	mux := http.NewServeMux()
	// The fwlink query is preserved; respond as the "CDN" with a
	// Last-Modified header and the ISO body.
	mux.HandleFunc("/fwlink/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", "Tue, 20 Jan 2026 13:56:22 GMT")
		w.Header().Set("Content-Disposition", `attachment; filename=SERVER_EVAL_x64FRE_en-us.iso`)
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write([]byte(isoContent))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	orig := fwlinkBase
	fwlinkBase = srv.URL + "/fwlink/"
	t.Cleanup(func() { fwlinkBase = orig })
}

func TestWindowsServerRegistersAllFourEditions(t *testing.T) {
	if len(editions) != 4 {
		t.Fatalf("expected 4 editions, got %d", len(editions))
	}
	for _, want := range []string{"windows-server-2016", "windows-server-2019", "windows-server-2022", "windows-server-2025"} {
		if _, ok := provider.Get(want); !ok {
			t.Errorf("provider %q not registered", want)
		}
	}
}

func TestWindowsServerCheckUsesLastModifiedDate(t *testing.T) {
	withTestServer(t)

	res, err := ws2025().Check(context.Background(), provider.Variant{Arch: "x64"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "2026-01-20" {
		t.Errorf("Check() version = %q, want %q", res.Version, "2026-01-20")
	}
}

func TestWindowsServerDownloadUsesCleanFilename(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := ws2025().Download(context.Background(), provider.Variant{Arch: "x64"}, provider.CheckResult{Version: "2026-01-20"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "windows-server-2025-2026-01-20-x64.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestWindowsServerLocalVersion(t *testing.T) {
	w := ws2025()
	version, ok := w.LocalVersion("windows-server-2025-2026-01-20-x64.iso", provider.Variant{Arch: "x64"})
	if !ok || version != "2026-01-20" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"2026-01-20\", true)", version, ok)
	}
	// A different edition's file must not match.
	if _, ok := w.LocalVersion("windows-server-2022-2026-01-20-x64.iso", provider.Variant{Arch: "x64"}); ok {
		t.Error("LocalVersion() matched a different edition's filename")
	}
}
