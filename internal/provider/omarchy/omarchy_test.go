package omarchy

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

	const isoContent = "fake omarchy iso bytes"
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(isoContent)))

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Shaped like the real homepage: the ISO is linked absolutely, next
		// to unrelated links and an older build mentioned elsewhere.
		fmt.Fprint(w, `<a href="/manual/">The Manual</a>`+
			`<a href="https://iso.omarchy.org/omarchy-4.0.1.iso">Previous</a>`+
			`<a href="https://iso.omarchy.org/omarchy-4.0.2.iso">The ISO</a>`+
			`<a href="https://github.com/omacom/omarchy">The Code</a>`)
	})
	mux.HandleFunc("/omarchy-4.0.2.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, isoContent)
	})
	mux.HandleFunc("/omarchy-4.0.2.iso.sha256", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  omarchy-4.0.2.iso\n", sum)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	origSite, origISO := siteURL, isoBaseURL
	siteURL = srv.URL + "/"
	isoBaseURL = srv.URL + "/"
	t.Cleanup(func() {
		siteURL = origSite
		isoBaseURL = origISO
	})
}

func TestOmarchyCheckPicksNewestISOLink(t *testing.T) {
	withTestServer(t)

	res, err := Omarchy{}.Check(context.Background(), provider.Variant{})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "4.0.2" {
		t.Errorf("Check() version = %q, want %q", res.Version, "4.0.2")
	}
}

func TestOmarchyCheckWithoutISOLink(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="/manual/">The Manual</a>`)
	}))
	t.Cleanup(srv.Close)

	orig := siteURL
	siteURL = srv.URL + "/"
	t.Cleanup(func() { siteURL = orig })

	if _, err := (Omarchy{}).Check(context.Background(), provider.Variant{}); err == nil {
		t.Error("Check() succeeded on a page with no ISO link")
	}
}

func TestOmarchyDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := Omarchy{}.Download(context.Background(), provider.Variant{}, provider.CheckResult{Version: "4.0.2"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "omarchy-4.0.2.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestOmarchyChecksum(t *testing.T) {
	withTestServer(t)

	checksum, err := Omarchy{}.Checksum(context.Background(), provider.Variant{}, "4.0.2")
	if err != nil {
		t.Fatalf("Checksum() error = %v", err)
	}
	if checksum == nil || len(checksum.Hex) != 64 {
		t.Errorf("Checksum() = %+v, want a sha256 digest", checksum)
	}
}

func TestOmarchyLocalVersion(t *testing.T) {
	version, ok := (Omarchy{}).LocalVersion("omarchy-4.0.2.iso", provider.Variant{})
	if !ok || version != "4.0.2" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"4.0.2\", true)", version, ok)
	}
	for _, name := range []string{"omarchy.iso", "omarchy-4.0.2.iso.sha256", "not-omarchy-4.0.2.iso"} {
		if _, ok := (Omarchy{}).LocalVersion(name, provider.Variant{}); ok {
			t.Errorf("LocalVersion(%q) matched, want no match", name)
		}
	}
}
