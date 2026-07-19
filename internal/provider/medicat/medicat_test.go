package medicat

import (
	"context"
	"crypto/sha1"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
)

func withTestServer(t *testing.T) {
	t.Helper()

	const content = "fake medicat 7z bytes"
	sum := fmt.Sprintf("%x", sha1.Sum([]byte(content)))

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `<html><body>
<a href=%s/files/v21.12/MediCat.USB.v21.12.7z class="btn">Mirror 2</a>
<p>MD5: db50f96a5c7b5ec6dc9ed77ea29fffb0<br>SHA1: %s</p>
</body></html>`, "http://"+r.Host, sum)
	})
	mux.HandleFunc("/files/v21.12/MediCat.USB.v21.12.7z", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, content)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// downloadURLRe is normally locked to the real official mirror host
	// (files.medicatusb.com) so it can never accidentally prefer the
	// third-party files.dog mirror also listed on the page; point it at
	// the test server's own host here instead.
	origPage, origDownloadURLRe := pageURL, downloadURLRe
	pageURL = srv.URL + "/"
	downloadURLRe = regexp.MustCompile(regexp.QuoteMeta(srv.URL) + `/[^"'\s<]+\.7z`)
	t.Cleanup(func() { pageURL, downloadURLRe = origPage, origDownloadURLRe })
}

func TestMedicatCheck(t *testing.T) {
	withTestServer(t)

	res, err := Medicat{}.Check(context.Background(), provider.Variant{})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "21.12" {
		t.Errorf("Check() version = %q, want %q", res.Version, "21.12")
	}
}

func TestMedicatDownload(t *testing.T) {
	withTestServer(t)
	dir := t.TempDir()

	res, err := Medicat{}.Check(context.Background(), provider.Variant{})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	result, err := Medicat{}.Download(context.Background(), provider.Variant{}, res, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "MediCat.USB.v21.12.7z" {
		t.Errorf("Download() filename = %q, want %q", result.Filename, "MediCat.USB.v21.12.7z")
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestMedicatLocalVersion(t *testing.T) {
	version, ok := Medicat{}.LocalVersion("MediCat.USB.v21.12.7z", provider.Variant{})
	if !ok || version != "21.12" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"21.12\", true)", version, ok)
	}

	if _, ok := (Medicat{}).LocalVersion("something-else.7z", provider.Variant{}); ok {
		t.Error("LocalVersion() matched an unrelated filename")
	}
}
