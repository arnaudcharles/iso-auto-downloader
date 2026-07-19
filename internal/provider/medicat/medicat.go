// Package medicat checks and downloads MediCat USB, a multiboot Windows PE
// / diagnostics toolkit. The project's own site, medicatusb.com, publishes
// a plain server-rendered download page listing its own first-party direct
// HTTP mirror (files.medicatusb.com) alongside the version and published
// MD5/SHA1 hashes — a real, Range-capable HTTP source, confirmed live. This
// scrapes that page directly rather than a third-party snapshot, so it
// actually tracks future MediCat releases instead of being frozen at
// whatever was true the day a mirror was uploaded somewhere else.
//
// The archive is a ~21 GB .7z, not a bootable ISO like every other
// provider here — MediCat is used by extracting it and writing it to a USB
// drive with the included Ventoy2Disk tool (Windows only), not by
// burning/mounting the file directly.
package medicat

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"regexp"

	"github.com/arnaudcharles/iso-auto-downloader/internal/download"
	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
	"github.com/arnaudcharles/iso-auto-downloader/internal/scrape"
)

// pageURL is a var (not a const) so tests can point it at an httptest server.
var pageURL = "https://medicatusb.com/"

var downloadURLRe = regexp.MustCompile(`https://files\.medicatusb\.com/[^"'\s<]+\.7z`)
var sha1Re = regexp.MustCompile(`(?i)SHA1:\s*([0-9A-Fa-f]{40})`)
var filenameVersionRe = regexp.MustCompile(`^MediCat\.USB\.v([\d.]+)\.7z$`)

type Medicat struct{}

func init() {
	provider.Register(Medicat{})
}

func (Medicat) ID() string                  { return "medicat" }
func (Medicat) Name() string                { return "MediCat USB" }
func (Medicat) Category() provider.Category { return provider.CategoryDiagnostic }

func (Medicat) Variants() []provider.Variant {
	return []provider.Variant{{}}
}

// currentRelease fetches the medicatusb.com download page and returns the
// exact current direct-mirror URL, filename, version, and SHA1 hash
// together — all four come from this one page, so they can never disagree
// with each other.
func currentRelease(ctx context.Context) (downloadURL, filename, version, sha1Hex string, err error) {
	body, err := scrape.FetchString(ctx, pageURL)
	if err != nil {
		return "", "", "", "", err
	}

	downloadURL = downloadURLRe.FindString(body)
	if downloadURL == "" {
		return "", "", "", "", fmt.Errorf("medicat: no direct download mirror found on %s", pageURL)
	}
	filename = path.Base(downloadURL)

	vm := filenameVersionRe.FindStringSubmatch(filename)
	if vm == nil {
		return "", "", "", "", fmt.Errorf("medicat: mirror filename %q doesn't match the expected MediCat.USB.v<version>.7z pattern", filename)
	}
	version = vm[1]

	sm := sha1Re.FindStringSubmatch(body)
	if sm == nil {
		return "", "", "", "", fmt.Errorf("medicat: no SHA1 hash found on %s", pageURL)
	}
	sha1Hex = sm[1]

	return downloadURL, filename, version, sha1Hex, nil
}

func (Medicat) Check(ctx context.Context, _ provider.Variant) (provider.CheckResult, error) {
	_, _, version, _, err := currentRelease(ctx)
	if err != nil {
		return provider.CheckResult{}, err
	}
	return provider.CheckResult{Version: version}, nil
}

func (Medicat) Checksum(ctx context.Context, _ provider.Variant, _ string) (*download.Checksum, error) {
	_, _, _, sha1Hex, err := currentRelease(ctx)
	if err != nil {
		return nil, err
	}
	return &download.Checksum{Algo: download.SHA1, Hex: sha1Hex}, nil
}

func (Medicat) Download(ctx context.Context, _ provider.Variant, _ provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	downloadURL, filename, _, sha1Hex, err := currentRelease(ctx)
	if err != nil {
		return provider.DownloadResult{}, err
	}

	dest := filepath.Join(destDir, filename)
	size, err := download.Download(ctx, download.Options{
		URL:      downloadURL,
		DestPath: dest,
		Checksum: &download.Checksum{Algo: download.SHA1, Hex: sha1Hex},
		Progress: download.ProgressFunc(progress),
	})
	if err != nil {
		return provider.DownloadResult{}, err
	}
	return provider.DownloadResult{Path: dest, Filename: filename, SizeBytes: size}, nil
}

func (Medicat) LocalVersion(filename string, _ provider.Variant) (string, bool) {
	return scrape.MatchVersion(filenameVersionRe, filename)
}
