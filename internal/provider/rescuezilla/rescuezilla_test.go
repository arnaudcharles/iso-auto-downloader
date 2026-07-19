package rescuezilla

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

	const isoContent = "fake rescuezilla iso bytes"
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(isoContent)))

	mux := http.NewServeMux()
	mux.HandleFunc("/releases", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[
			{"tag_name": "rolling-release.2023-11-26", "assets": [{"name": "rescuezilla-rolling-release.2023-11-26-64bit.noble.iso"}]},
			{"tag_name": "2.6.2", "assets": [{"name": "rescuezilla-2.6.2-64bit.noble.iso"}, {"name": "rescuezilla-2.6.2-64bit.resolute.iso"}]},
			{"tag_name": "2.6.1", "assets": [{"name": "rescuezilla-2.6.1-64bit.noble.iso"}]}
		]`)
	})
	mux.HandleFunc("/download/2.6.2/rescuezilla-2.6.2-64bit.noble.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, isoContent)
	})
	mux.HandleFunc("/download/2.6.2/SHA256SUM", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  rescuezilla-2.6.2-64bit.noble.iso\n", sum)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	origAPI, origBase := releasesAPIURL, downloadBaseURL
	releasesAPIURL = srv.URL + "/releases"
	downloadBaseURL = srv.URL + "/download/"
	t.Cleanup(func() { releasesAPIURL, downloadBaseURL = origAPI, origBase })
}

func TestRescuezillaCheckSkipsRollingRelease(t *testing.T) {
	withTestServer(t)

	res, err := Rescuezilla{}.Check(context.Background(), provider.Variant{Edition: "noble", Arch: "64bit"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "2.6.2" {
		t.Errorf("Check() version = %q, want %q", res.Version, "2.6.2")
	}
}

func TestRescuezillaDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := Rescuezilla{}.Download(context.Background(), provider.Variant{Edition: "noble", Arch: "64bit"}, provider.CheckResult{Version: "2.6.2"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "rescuezilla-2.6.2-64bit.noble.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestRescuezillaLocalVersion(t *testing.T) {
	v := provider.Variant{Edition: "noble", Arch: "64bit"}
	version, ok := (Rescuezilla{}).LocalVersion("rescuezilla-2.6.2-64bit.noble.iso", v)
	if !ok || version != "2.6.2" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"2.6.2\", true)", version, ok)
	}
	if _, ok := (Rescuezilla{}).LocalVersion("rescuezilla-2.6.2-64bit.resolute.iso", v); ok {
		t.Error("LocalVersion() matched a different edition's filename")
	}
}
