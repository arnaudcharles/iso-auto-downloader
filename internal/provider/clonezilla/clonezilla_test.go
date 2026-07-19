package clonezilla

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

	const isoContent = "fake clonezilla iso bytes"
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(isoContent)))

	mux := http.NewServeMux()
	mux.HandleFunc("/CHECKSUMS.TXT", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "### MD5SUMS:\nsomehash  clonezilla-live-3.3.3-15-amd64.iso\n\n"+
			"### SHA256SUMS:\n%s  clonezilla-live-3.3.3-15-amd64.iso\n%s  clonezilla-live-3.3.3-15-amd64.zip\n\n"+
			"### SHA512SUMS:\nsomehash  clonezilla-live-3.3.3-15-amd64.iso\n", sum, sum)
	})
	mux.HandleFunc("/clonezilla_live_stable/3.3.3-15/clonezilla-live-3.3.3-15-amd64.iso", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, isoContent)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	origSums, origBase := checksumsURL, downloadBaseURL
	checksumsURL = srv.URL + "/CHECKSUMS.TXT"
	downloadBaseURL = srv.URL + "/clonezilla_live_stable/"
	t.Cleanup(func() { checksumsURL, downloadBaseURL = origSums, origBase })
}

func TestClonezillaCheck(t *testing.T) {
	withTestServer(t)

	res, err := Clonezilla{}.Check(context.Background(), provider.Variant{Arch: "amd64"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "3.3.3-15" {
		t.Errorf("Check() version = %q, want %q", res.Version, "3.3.3-15")
	}
}

func TestClonezillaDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	result, err := Clonezilla{}.Download(context.Background(), provider.Variant{Arch: "amd64"}, provider.CheckResult{Version: "3.3.3-15"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "clonezilla-live-3.3.3-15-amd64.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestClonezillaLocalVersion(t *testing.T) {
	version, ok := (Clonezilla{}).LocalVersion("clonezilla-live-3.3.3-15-amd64.iso", provider.Variant{Arch: "amd64"})
	if !ok || version != "3.3.3-15" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"3.3.3-15\", true)", version, ok)
	}
}
