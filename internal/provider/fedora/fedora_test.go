package fedora

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

	const isoContent = "fake fedora iso bytes"
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(isoContent)))
	const isoName = "Fedora-Workstation-Live-44-1.7.x86_64.iso"
	const checksumName = "Fedora-Workstation-44-1.7-x86_64-CHECKSUM"

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="43/">43/</a><a href="44/">44/</a>`)
	})
	mux.HandleFunc("/44/Workstation/x86_64/iso/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<a href="%s">iso</a><a href="%s">checksum</a>`, isoName, checksumName)
	})
	mux.HandleFunc("/44/Workstation/x86_64/iso/"+isoName, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, isoContent)
	})
	mux.HandleFunc("/44/Workstation/x86_64/iso/"+checksumName, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "-----BEGIN PGP SIGNED MESSAGE-----\nHash: SHA256\n\n# %s: 123 bytes\nSHA256 (%s) = %s\n-----BEGIN PGP SIGNATURE-----\n", isoName, isoName, sum)
	})

	const serverIsoName = "Fedora-Server-dvd-x86_64-44-1.7.iso"
	const serverChecksumName = "Fedora-Server-44-1.7-x86_64-CHECKSUM"
	mux.HandleFunc("/44/Server/x86_64/iso/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<a href="%s">iso</a><a href="%s">checksum</a>`, serverIsoName, serverChecksumName)
	})
	mux.HandleFunc("/44/Server/x86_64/iso/"+serverIsoName, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, isoContent)
	})
	mux.HandleFunc("/44/Server/x86_64/iso/"+serverChecksumName, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "-----BEGIN PGP SIGNED MESSAGE-----\nHash: SHA256\n\n# %s: 123 bytes\nSHA256 (%s) = %s\n-----BEGIN PGP SIGNATURE-----\n", serverIsoName, serverIsoName, sum)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	orig := releasesURL
	releasesURL = srv.URL + "/"
	t.Cleanup(func() { releasesURL = orig })
}

func TestFedoraCheck(t *testing.T) {
	withTestServer(t)

	res, err := Fedora{}.Check(context.Background(), provider.Variant{Edition: "Workstation", Arch: "x86_64"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "44" {
		t.Errorf("Check() version = %q, want %q", res.Version, "44")
	}
}

func TestFedoraDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := Fedora{}.Download(context.Background(), provider.Variant{Edition: "Workstation", Arch: "x86_64"}, provider.CheckResult{Version: "44"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "Fedora-Workstation-Live-44-1.7.x86_64.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestFedoraLocalVersion(t *testing.T) {
	v := provider.Variant{Edition: "Workstation", Arch: "x86_64"}

	// Only the release number is captured — the respin ("-1.7") isn't part
	// of what Check() reports, so it can't be part of the comparison.
	version, ok := Fedora{}.LocalVersion("Fedora-Workstation-Live-44-1.7.x86_64.iso", v)
	if !ok || version != "44" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"44\", true)", version, ok)
	}

	if _, ok := (Fedora{}).LocalVersion("Fedora-Workstation-Live-44-1.7.aarch64.iso", v); ok {
		t.Error("LocalVersion() matched a different arch's filename")
	}
}

func TestFedoraServerEdition(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()
	v := provider.Variant{Edition: "Server", Arch: "x86_64"}

	result, err := Fedora{}.Download(context.Background(), v, provider.CheckResult{Version: "44"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "Fedora-Server-dvd-x86_64-44-1.7.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}

	version, ok := Fedora{}.LocalVersion("Fedora-Server-dvd-x86_64-44-1.7.iso", v)
	if !ok || version != "44" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"44\", true)", version, ok)
	}
	// A Workstation filename should never match the Server variant.
	if _, ok := (Fedora{}).LocalVersion("Fedora-Workstation-Live-44-1.7.x86_64.iso", v); ok {
		t.Error("LocalVersion() matched a Workstation filename against the Server variant")
	}
}
