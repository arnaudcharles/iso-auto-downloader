// Package ubuntu checks and downloads Ubuntu Desktop and Server ISOs from
// a stable OVH mirror.
package ubuntu

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/arnaudcharles/iso-auto-downloader/internal/download"
	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
	"github.com/arnaudcharles/iso-auto-downloader/internal/scrape"
)

// releasesURL is a var (not a const) so tests can point it at an httptest server.
var releasesURL = "https://ubuntu.mirrors.ovh.net/releases/"

var dirVersionRe = regexp.MustCompile(`^(\d+(?:\.\d+)+)/$`)

type Ubuntu struct{}

func init() {
	provider.Register(Ubuntu{})
}

func (Ubuntu) ID() string                  { return "ubuntu" }
func (Ubuntu) Name() string                { return "Ubuntu" }
func (Ubuntu) Category() provider.Category { return provider.CategoryLinux }

func (Ubuntu) Variants() []provider.Variant {
	return []provider.Variant{
		{Edition: "desktop", Arch: "amd64"},
		{Edition: "live-server", Arch: "amd64"},
	}
}

func (Ubuntu) Check(ctx context.Context, _ provider.Variant) (provider.CheckResult, error) {
	body, err := scrape.FetchString(ctx, releasesURL)
	if err != nil {
		return provider.CheckResult{}, err
	}

	best := ""
	for _, href := range scrape.Hrefs(body) {
		m := dirVersionRe.FindStringSubmatch(href)
		if m == nil {
			continue
		}
		if best == "" || scrape.CompareVersions(m[1], best) > 0 {
			best = m[1]
		}
	}
	if best == "" {
		return provider.CheckResult{}, fmt.Errorf("ubuntu: no version directory found at %s", releasesURL)
	}
	return provider.CheckResult{Version: best}, nil
}

func (Ubuntu) LocalVersion(filename string, v provider.Variant) (string, bool) {
	re := regexp.MustCompile(fmt.Sprintf(`^ubuntu-([\d.]+)-%s-%s\.iso$`, regexp.QuoteMeta(v.Edition), regexp.QuoteMeta(v.Arch)))
	return scrape.MatchVersion(re, filename)
}

func (Ubuntu) Checksum(ctx context.Context, v provider.Variant, version string) (*download.Checksum, error) {
	filename := fmt.Sprintf("ubuntu-%s-%s-%s.iso", version, v.Edition, v.Arch)
	versionDir := releasesURL + version + "/"

	sums, err := scrape.FetchString(ctx, versionDir+"SHA256SUMS")
	if err != nil {
		return nil, err
	}
	checksum, err := scrape.ChecksumForFile(sums, filename)
	if err != nil {
		return nil, err
	}
	return &download.Checksum{Algo: download.SHA256, Hex: checksum}, nil
}

func (Ubuntu) Download(ctx context.Context, v provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	filename := fmt.Sprintf("ubuntu-%s-%s-%s.iso", res.Version, v.Edition, v.Arch)
	versionDir := releasesURL + res.Version + "/"
	fileURL := versionDir + filename

	checksum, err := (Ubuntu{}).Checksum(ctx, v, res.Version)
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
