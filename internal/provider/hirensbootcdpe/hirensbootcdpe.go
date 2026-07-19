// Package hirensbootcdpe checks and downloads Hiren's BootCD PE from
// hirensbootcd.org.
package hirensbootcdpe

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/arnaudcharles/iso-auto-downloader/internal/download"
	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
	"github.com/arnaudcharles/iso-auto-downloader/internal/scrape"
)

// pageURL and fileURL are vars (not consts) so tests can point them at an
// httptest server.
var pageURL = "https://www.hirensbootcd.org/download/"
var fileURL = "https://www.hirensbootcd.org/files/HBCD_PE_x64.iso"

// officialFilename is the name Hiren's own site always serves the file
// under — fixed, never embeds a version. This app instead saves it locally
// as versionedFilename(version), baking in the version Check() already
// resolved so LocalVersion can recognize a file it downloaded the normal
// way every other provider does, instead of falling back to a placeholder.
// officialFilename is still recognized by LocalVersion below, for a file
// that predates this or was downloaded by hand from Hiren's own site.
const officialFilename = "HBCD_PE_x64.iso"

func versionedFilename(version string) string {
	return fmt.Sprintf("HBCD_PE_x64_v%s.iso", version)
}

var versionedFilenameRe = regexp.MustCompile(`^HBCD_PE_x64_v([\d.]+)\.iso$`)

var versionRe = regexp.MustCompile(`\(v([\d.]+)\)`)
var sha256Re = regexp.MustCompile(`\b[a-fA-F0-9]{64}\b`)

type HirensBootCDPE struct{}

func init() {
	provider.Register(HirensBootCDPE{})
}

func (HirensBootCDPE) ID() string                  { return "hirensbootcdpe" }
func (HirensBootCDPE) Name() string                { return "Hiren's BootCD PE" }
func (HirensBootCDPE) Category() provider.Category { return provider.CategoryDiagnostic }

func (HirensBootCDPE) Variants() []provider.Variant {
	return []provider.Variant{{Arch: "x64"}}
}

func (HirensBootCDPE) Check(ctx context.Context, _ provider.Variant) (provider.CheckResult, error) {
	body, err := scrape.FetchString(ctx, pageURL)
	if err != nil {
		return provider.CheckResult{}, err
	}
	m := versionRe.FindStringSubmatch(body)
	if m == nil {
		return provider.CheckResult{}, fmt.Errorf("hirensbootcdpe: no version found on %s", pageURL)
	}
	return provider.CheckResult{Version: m[1]}, nil
}

// Checksum ignores the requested version: the page only ever lists the
// current build's hash, same limitation as LocalVersion below.
func (HirensBootCDPE) Checksum(ctx context.Context, _ provider.Variant, _ string) (*download.Checksum, error) {
	body, err := scrape.FetchString(ctx, pageURL)
	if err != nil {
		return nil, err
	}
	m := sha256Re.FindString(body)
	if m == "" {
		return nil, fmt.Errorf("hirensbootcdpe: no checksum found on %s", pageURL)
	}
	return &download.Checksum{Algo: download.SHA256, Hex: m}, nil
}

func (HirensBootCDPE) Download(ctx context.Context, v provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	checksum, err := (HirensBootCDPE{}).Checksum(ctx, v, res.Version)
	if err != nil {
		return provider.DownloadResult{}, err
	}

	localFilename := versionedFilename(res.Version)
	dest := filepath.Join(destDir, localFilename)
	size, err := download.Download(ctx, download.Options{
		URL:      fileURL,
		DestPath: dest,
		Checksum: checksum,
		Progress: download.ProgressFunc(progress),
	})
	if err != nil {
		return provider.DownloadResult{}, err
	}
	return provider.DownloadResult{Path: dest, Filename: localFilename, SizeBytes: size}, nil
}

// unknownLocalVersion is what LocalVersion reports for a legacy/manual file
// (Hiren's own official filename, which never embeds a version) it can
// confirm exists but can't determine the actual version of — deliberately
// not a real-looking version string, so it never accidentally string-equals
// a real "latest" version and gets misreported as up to date.
const unknownLocalVersion = "unknown"

// LocalVersion recognizes two shapes: this app's own versioned filename
// (see versionedFilename above), reporting the real embedded version like
// every other provider, or Hiren's official fixed filename (no version
// ever embedded — the same limitation upstream's own filename-pattern
// detection has), reporting the unknownLocalVersion placeholder for a file
// that predates this or was downloaded by hand from Hiren's own site.
// Real report: before either of these existed, a genuinely present and
// current file always showed "Not found", forcing a full re-download just
// to make it show up.
func (HirensBootCDPE) LocalVersion(fname string, _ provider.Variant) (string, bool) {
	if m := versionedFilenameRe.FindStringSubmatch(fname); m != nil {
		return m[1], true
	}
	if fname == officialFilename {
		return unknownLocalVersion, true
	}
	return "", false
}
