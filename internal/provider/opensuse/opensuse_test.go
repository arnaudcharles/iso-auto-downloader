package opensuse

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

	const leapContent = "fake leap iso bytes"
	leapSum := fmt.Sprintf("%x", sha256.Sum256([]byte(leapContent)))
	const tumbleweedContent = "fake tumbleweed iso bytes"
	tumbleweedSum := fmt.Sprintf("%x", sha256.Sum256([]byte(tumbleweedContent)))

	mux := http.NewServeMux()
	mux.HandleFunc("/distribution/leap/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="./15.6/">15.6/</a><a href="./16.1/">16.1/</a><a href="./42.3/">42.3/</a>`)
	})
	mux.HandleFunc("/distribution/leap/16.1/installer/iso/agama-installer.x86_64-Leap_16.1.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, leapContent)
	})
	mux.HandleFunc("/distribution/leap/16.1/installer/iso/agama-installer.x86_64-Leap_16.1.iso.sha256", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  agama-installer.x86_64-22.0.0-Leap_16.1-Build35.9.iso\n", leapSum)
	})

	mux.HandleFunc("/tumbleweed/iso/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a href="openSUSE-Tumbleweed-DVD-x86_64-Snapshot20260714-Media.iso">iso</a>`)
	})
	mux.HandleFunc("/tumbleweed/iso/openSUSE-Tumbleweed-DVD-x86_64-Current.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, tumbleweedContent)
	})
	mux.HandleFunc("/tumbleweed/iso/openSUSE-Tumbleweed-DVD-x86_64-Current.iso.sha256", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  openSUSE-Tumbleweed-DVD-x86_64-Snapshot20260714-Media.iso\n", tumbleweedSum)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	origLeap, origTumbleweed := leapIndexURL, tumbleweedURL
	leapIndexURL = srv.URL + "/distribution/leap/"
	tumbleweedURL = srv.URL + "/tumbleweed/iso/"
	t.Cleanup(func() { leapIndexURL, tumbleweedURL = origLeap, origTumbleweed })
}

func TestOpenSUSELeapCheckExcludesLegacy42Series(t *testing.T) {
	withTestServer(t)

	res, err := OpenSUSE{}.Check(context.Background(), provider.Variant{Edition: "leap", Arch: "x86_64"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "16.1" {
		t.Errorf("Check() version = %q, want %q (the leftover 42.3 archive dir should be excluded)", res.Version, "16.1")
	}
}

func TestOpenSUSELeapDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := OpenSUSE{}.Download(context.Background(), provider.Variant{Edition: "leap", Arch: "x86_64"}, provider.CheckResult{Version: "16.1"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "openSUSE-Leap-16.1-installer-x86_64.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestOpenSUSETumbleweedCheckAndDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()
	v := provider.Variant{Edition: "tumbleweed", Arch: "x86_64"}

	res, err := OpenSUSE{}.Check(context.Background(), v)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "20260714" {
		t.Errorf("Check() version = %q, want %q", res.Version, "20260714")
	}

	result, err := OpenSUSE{}.Download(context.Background(), v, res, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "openSUSE-Tumbleweed-20260714-DVD-x86_64.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestOpenSUSELocalVersion(t *testing.T) {
	leap := provider.Variant{Edition: "leap", Arch: "x86_64"}
	version, ok := OpenSUSE{}.LocalVersion("openSUSE-Leap-16.1-installer-x86_64.iso", leap)
	if !ok || version != "16.1" {
		t.Errorf("LocalVersion(leap) = (%q, %v), want (\"16.1\", true)", version, ok)
	}

	tumbleweed := provider.Variant{Edition: "tumbleweed", Arch: "x86_64"}
	version, ok = OpenSUSE{}.LocalVersion("openSUSE-Tumbleweed-20260714-DVD-x86_64.iso", tumbleweed)
	if !ok || version != "20260714" {
		t.Errorf("LocalVersion(tumbleweed) = (%q, %v), want (\"20260714\", true)", version, ok)
	}

	// A leap file should never match the tumbleweed variant and vice versa.
	if _, ok := (OpenSUSE{}).LocalVersion("openSUSE-Leap-16.1-installer-x86_64.iso", tumbleweed); ok {
		t.Error("LocalVersion() matched a Leap filename against the tumbleweed variant")
	}
}
