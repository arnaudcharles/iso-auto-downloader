// Package memtest86plus checks and downloads MemTest86+ ISOs from
// memtest.org. The upstream distribution ships the ISO inside a zip, so
// Download fetches and checksums the zip, then extracts the .iso and
// discards the zip.
package memtest86plus

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/arnaudcharles/iso-auto-downloader/internal/download"
	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
	"github.com/arnaudcharles/iso-auto-downloader/internal/scrape"
)

// baseURL is a var (not a const) so tests can point it at an httptest server.
var baseURL = "https://www.memtest.org/"

type MemTest86Plus struct{}

func init() {
	provider.Register(MemTest86Plus{})
}

func (MemTest86Plus) ID() string                  { return "memtest86plus" }
func (MemTest86Plus) Name() string                { return "MemTest86+" }
func (MemTest86Plus) Category() provider.Category { return provider.CategoryDiagnostic }

func (MemTest86Plus) Variants() []provider.Variant {
	return []provider.Variant{{Arch: "x86_64"}, {Arch: "i586"}}
}

func (MemTest86Plus) Check(ctx context.Context, v provider.Variant) (provider.CheckResult, error) {
	body, err := scrape.FetchString(ctx, baseURL)
	if err != nil {
		return provider.CheckResult{}, err
	}

	re := regexp.MustCompile(fmt.Sprintf(`mt86plus_([\d.]+)_%s\.iso\.zip`, regexp.QuoteMeta(v.Arch)))
	for _, href := range scrape.Hrefs(body) {
		if m := re.FindStringSubmatch(href); m != nil {
			return provider.CheckResult{Version: m[1]}, nil
		}
	}
	return provider.CheckResult{}, fmt.Errorf("memtest86plus: no download link found for arch %q on %s", v.Arch, baseURL)
}

func (MemTest86Plus) LocalVersion(filename string, v provider.Variant) (string, bool) {
	re := regexp.MustCompile(fmt.Sprintf(`^mt86plus_([\d.]+)_%s\.iso$`, regexp.QuoteMeta(v.Arch)))
	return scrape.MatchVersion(re, filename)
}

// Checksum: the only published checksum covers the .zip Download fetches
// (verified inline there), not the .iso extracted from it and kept on disk
// — the zip itself is discarded once extraction succeeds. There's nothing
// to compare the kept file against without re-fetching and re-extracting
// the zip, which would defeat the point of a standalone, no-download
// validation.
func (MemTest86Plus) Checksum(context.Context, provider.Variant, string) (*download.Checksum, error) {
	return nil, nil
}

func (MemTest86Plus) Download(ctx context.Context, v provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	version := res.Version
	zipFilename := fmt.Sprintf("mt86plus_%s_%s.iso.zip", version, v.Arch)
	versionDir := fmt.Sprintf("%sdownload/v%s/", baseURL, version)
	zipURL := versionDir + zipFilename

	sums, err := scrape.FetchString(ctx, versionDir+"sha256sum.txt")
	if err != nil {
		return provider.DownloadResult{}, err
	}
	checksum, err := scrape.ChecksumForFile(sums, zipFilename)
	if err != nil {
		return provider.DownloadResult{}, err
	}

	zipDest := filepath.Join(destDir, zipFilename)
	if _, err := download.Download(ctx, download.Options{
		URL:      zipURL,
		DestPath: zipDest,
		Checksum: &download.Checksum{Algo: download.SHA256, Hex: checksum},
		Progress: download.ProgressFunc(progress),
	}); err != nil {
		return provider.DownloadResult{}, err
	}
	defer func() { _ = os.Remove(zipDest) }()

	isoFilename := fmt.Sprintf("mt86plus_%s_%s.iso", version, v.Arch)
	isoDest := filepath.Join(destDir, isoFilename)
	if err := extractISO(zipDest, isoDest); err != nil {
		return provider.DownloadResult{}, fmt.Errorf("memtest86plus: extract iso from %s: %w", zipDest, err)
	}

	fi, err := os.Stat(isoDest)
	if err != nil {
		return provider.DownloadResult{}, fmt.Errorf("memtest86plus: stat extracted iso: %w", err)
	}
	return provider.DownloadResult{Path: isoDest, Filename: isoFilename, SizeBytes: fi.Size()}, nil
}

// extractISO pulls the single .iso entry out of zipPath and writes it to destPath.
func extractISO(zipPath, destPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		if !strings.HasSuffix(f.Name, ".iso") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func() { _ = rc.Close() }()

		tmp := destPath + ".part"
		out, err := os.Create(tmp)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			_ = out.Close()
			_ = os.Remove(tmp)
			return err
		}
		if err := out.Close(); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		return os.Rename(tmp, destPath)
	}
	return fmt.Errorf("no .iso entry found in %s", zipPath)
}
