// Package systemrescue checks and downloads SystemRescue ISOs. The version
// is resolved from the download page, but the actual file is served from
// Fastly's CDN.
package systemrescue

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/arnaudcharles/iso-auto-downloader/internal/download"
	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
	"github.com/arnaudcharles/iso-auto-downloader/internal/scrape"
)

// pageURL and cdnURL are vars (not consts) so tests can point them at an
// httptest server.
var pageURL = "https://www.system-rescue.org/Download/"
var cdnURL = "https://fastly-cdn.system-rescue.org/releases/"

var versionRe = regexp.MustCompile(`systemrescue-([\d.]+)-amd64\.iso`)

type SystemRescue struct{}

func init() {
	provider.Register(SystemRescue{})
}

func (SystemRescue) ID() string                  { return "systemrescue" }
func (SystemRescue) Name() string                { return "SystemRescue" }
func (SystemRescue) Category() provider.Category { return provider.CategoryDiagnostic }

func (SystemRescue) Variants() []provider.Variant {
	return []provider.Variant{{Arch: "amd64"}}
}

func (SystemRescue) Check(ctx context.Context, _ provider.Variant) (provider.CheckResult, error) {
	body, err := scrape.FetchString(ctx, pageURL)
	if err != nil {
		return provider.CheckResult{}, err
	}

	best := ""
	for _, m := range versionRe.FindAllStringSubmatch(body, -1) {
		if best == "" || scrape.CompareVersions(m[1], best) > 0 {
			best = m[1]
		}
	}
	if best == "" {
		return provider.CheckResult{}, fmt.Errorf("systemrescue: no version found on %s", pageURL)
	}
	return provider.CheckResult{Version: best}, nil
}

func (SystemRescue) Checksum(ctx context.Context, _ provider.Variant, version string) (*download.Checksum, error) {
	filename := fmt.Sprintf("systemrescue-%s-amd64.iso", version)
	fileURL := fmt.Sprintf("%s%s/%s", cdnURL, version, filename)

	sums, err := scrape.FetchString(ctx, fileURL+".sha256")
	if err != nil {
		return nil, err
	}
	checksum, err := scrape.ChecksumForFile(sums, filename)
	if err != nil {
		return nil, err
	}
	return &download.Checksum{Algo: download.SHA256, Hex: checksum}, nil
}

func (SystemRescue) Download(ctx context.Context, v provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	filename := fmt.Sprintf("systemrescue-%s-amd64.iso", res.Version)
	fileURL := fmt.Sprintf("%s%s/%s", cdnURL, res.Version, filename)

	checksum, err := (SystemRescue{}).Checksum(ctx, v, res.Version)
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

func (SystemRescue) LocalVersion(filename string, _ provider.Variant) (string, bool) {
	return scrape.MatchVersion(regexp.MustCompile(`^systemrescue-([\d.]+)-amd64\.iso$`), filename)
}
