// Package windowsserver checks and downloads the Windows Server Evaluation
// ISOs (2016/2019/2022/2025) from Microsoft's Evaluation Center.
//
// Each version is fronted by a stable go.microsoft.com/fwlink redirector
// that resolves to the current eval ISO on Microsoft's CDN. Since the eval
// ISOs get refreshed periodically and Microsoft's own filenames are wildly
// inconsistent (some embed a build number, some don't), the version is
// taken from the CDN response's Last-Modified date (a HEAD request, no
// download needed), and files are saved under a clean, self-consistent
// name so on-disk detection works regardless of Microsoft's naming.
//
// No checksum is published for these eval ISOs; integrity rests on the
// HTTPS transfer direct from Microsoft, same as the other checksum-less
// providers (UltimateBootCD, ParrotOS).
package windowsserver

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"time"

	"github.com/arnaudcharles/iso-auto-downloader/internal/download"
	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
	"github.com/arnaudcharles/iso-auto-downloader/internal/scrape"
)

// fwlinkBase is a var (not a const) so tests can point it at an httptest
// server. Each edition's fwlink query is appended to it.
var fwlinkBase = "https://go.microsoft.com/fwlink/"

type edition struct {
	id    string // provider ID and filename prefix, e.g. "windows-server-2025"
	name  string // display name
	query string // fwlink query string identifying the en-us x64 ISO
}

var editions = []edition{
	{"windows-server-2016", "Windows Server 2016", "?LinkID=2195174&clcid=0x409&culture=en-us&country=US"},
	{"windows-server-2019", "Windows Server 2019", "?LinkID=2195167&clcid=0x409&culture=en-us&country=US"},
	{"windows-server-2022", "Windows Server 2022", "?LinkID=2195280&clcid=0x409&culture=en-us&country=US"},
	{"windows-server-2025", "Windows Server 2025", "?linkid=2345730&clcid=0x409&culture=en-us&country=us"},
}

// WindowsServer is one Windows Server eval edition.
type WindowsServer struct {
	ed edition
}

func init() {
	for _, ed := range editions {
		provider.Register(WindowsServer{ed: ed})
	}
}

func (w WindowsServer) ID() string                 { return w.ed.id }
func (w WindowsServer) Name() string               { return w.ed.name }
func (WindowsServer) Category() provider.Category  { return provider.CategoryWindows }
func (WindowsServer) Variants() []provider.Variant { return []provider.Variant{{Arch: "x64"}} }

func (w WindowsServer) fwlink() string { return fwlinkBase + w.ed.query }

// version resolves the fwlink and turns the CDN's Last-Modified header into
// a YYYY-MM-DD version string.
func (w WindowsServer) version(ctx context.Context) (string, error) {
	_, header, err := scrape.Resolve(ctx, w.fwlink())
	if err != nil {
		return "", err
	}
	lm := header.Get("Last-Modified")
	if lm == "" {
		return "", fmt.Errorf("%s: no Last-Modified header on the ISO", w.ed.id)
	}
	t, err := time.Parse(time.RFC1123, lm)
	if err != nil {
		return "", fmt.Errorf("%s: parse Last-Modified %q: %w", w.ed.id, lm, err)
	}
	return t.Format("2006-01-02"), nil
}

func (w WindowsServer) filename(version string) string {
	return fmt.Sprintf("%s-%s-x64.iso", w.ed.id, version)
}

func (w WindowsServer) Check(ctx context.Context, _ provider.Variant) (provider.CheckResult, error) {
	version, err := w.version(ctx)
	if err != nil {
		return provider.CheckResult{}, err
	}
	return provider.CheckResult{Version: version}, nil
}

func (w WindowsServer) Download(ctx context.Context, _ provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	// Resolve to the concrete CDN URL first so download resume works
	// against a stable URL rather than re-running the fwlink redirect.
	finalURL, _, err := scrape.Resolve(ctx, w.fwlink())
	if err != nil {
		return provider.DownloadResult{}, err
	}

	filename := w.filename(res.Version)
	dest := filepath.Join(destDir, filename)
	size, err := download.Download(ctx, download.Options{
		URL:      finalURL,
		DestPath: dest,
		Progress: download.ProgressFunc(progress),
	})
	if err != nil {
		return provider.DownloadResult{}, err
	}
	return provider.DownloadResult{Path: dest, Filename: filename, SizeBytes: size}, nil
}

// Checksum: no checksum is published for these eval ISOs (see the package
// doc comment), so there's nothing to verify against.
func (WindowsServer) Checksum(context.Context, provider.Variant, string) (*download.Checksum, error) {
	return nil, nil
}

func (w WindowsServer) LocalVersion(filename string, _ provider.Variant) (string, bool) {
	re := regexp.MustCompile(fmt.Sprintf(`^%s-(\d{4}-\d{2}-\d{2})-x64\.iso$`, regexp.QuoteMeta(w.ed.id)))
	return scrape.MatchVersion(re, filename)
}
