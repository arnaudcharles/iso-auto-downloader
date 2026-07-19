// Package xcpng checks and downloads the XCP-ng hypervisor install ISO
// (the full ISO, not the netinstall variant) from updates.xcp-ng.org.
package xcpng

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/arnaudcharles/iso-auto-downloader/internal/download"
	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
	"github.com/arnaudcharles/iso-auto-downloader/internal/scrape"
)

// baseURL is a var (not a const) so tests can point it at an httptest server.
var baseURL = "https://updates.xcp-ng.org/isos/"

var releaseDirRe = regexp.MustCompile(`^(\d+\.\d+)/$`)
var isoFileRe = regexp.MustCompile(`^xcp-ng-([\d.]+-[\d.]+)\.iso$`)

// localVersionRe is deliberately more lenient than isoFileRe: files
// downloaded elsewhere (or years ago) are sometimes just
// "xcp-ng-8.3.0.iso" with no build-date suffix, unlike the mirror's own
// current naming.
var localVersionRe = regexp.MustCompile(`^xcp-ng-([\d.]+(?:-[\d.]+)?)\.iso$`)

type XCPng struct{}

func init() {
	provider.Register(XCPng{})
}

func (XCPng) ID() string                  { return "xcpng" }
func (XCPng) Name() string                { return "XCP-ng" }
func (XCPng) Category() provider.Category { return provider.CategoryVirtualization }

func (XCPng) Variants() []provider.Variant {
	return []provider.Variant{{}}
}

func latestReleaseDir(ctx context.Context) (string, error) {
	body, err := scrape.FetchString(ctx, baseURL)
	if err != nil {
		return "", err
	}
	best := ""
	for _, href := range scrape.Hrefs(body) {
		m := releaseDirRe.FindStringSubmatch(href)
		if m == nil {
			continue
		}
		if best == "" || scrape.CompareVersions(m[1], best) > 0 {
			best = m[1]
		}
	}
	if best == "" {
		return "", fmt.Errorf("xcpng: no release directory found at %s", baseURL)
	}
	return best, nil
}

func (XCPng) Check(ctx context.Context, _ provider.Variant) (provider.CheckResult, error) {
	releaseDir, err := latestReleaseDir(ctx)
	if err != nil {
		return provider.CheckResult{}, err
	}

	dirURL := baseURL + releaseDir + "/"
	body, err := scrape.FetchString(ctx, dirURL)
	if err != nil {
		return provider.CheckResult{}, err
	}

	best := ""
	for _, href := range scrape.Hrefs(body) {
		m := isoFileRe.FindStringSubmatch(href)
		if m == nil {
			continue
		}
		// Build-date suffixes sort correctly with plain string comparison
		// (e.g. "8.3.0-20250606.2" > "8.3.0-20250606").
		if m[1] > best {
			best = m[1]
		}
	}
	if best == "" {
		return provider.CheckResult{}, fmt.Errorf("xcpng: no ISO found at %s", dirURL)
	}
	return provider.CheckResult{Version: best}, nil
}

func releaseDirFor(version string) string {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return version
	}
	return parts[0] + "." + parts[1]
}

func (XCPng) Checksum(ctx context.Context, _ provider.Variant, version string) (*download.Checksum, error) {
	dirURL := baseURL + releaseDirFor(version) + "/"
	filename := fmt.Sprintf("xcp-ng-%s.iso", version)

	sums, err := scrape.FetchString(ctx, dirURL+"SHA256SUMS")
	if err != nil {
		return nil, err
	}
	checksum, err := scrape.ChecksumForFile(sums, filename)
	if err != nil {
		return nil, err
	}
	return &download.Checksum{Algo: download.SHA256, Hex: checksum}, nil
}

func (XCPng) Download(ctx context.Context, v provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	dirURL := baseURL + releaseDirFor(res.Version) + "/"
	filename := fmt.Sprintf("xcp-ng-%s.iso", res.Version)
	fileURL := dirURL + filename

	checksum, err := (XCPng{}).Checksum(ctx, v, res.Version)
	if err != nil {
		return provider.DownloadResult{}, err
	}

	dest := filepath.Join(destDir, filename)
	size, err := download.Download(ctx, download.Options{
		URL:      fileURL,
		DestPath: dest,
		Checksum: checksum,
		Progress: download.ProgressFunc(progress),
	})
	if err != nil {
		return provider.DownloadResult{}, err
	}
	return provider.DownloadResult{Path: dest, Filename: filename, SizeBytes: size}, nil
}

func (XCPng) LocalVersion(filename string, _ provider.Variant) (string, bool) {
	return scrape.MatchVersion(localVersionRe, filename)
}
