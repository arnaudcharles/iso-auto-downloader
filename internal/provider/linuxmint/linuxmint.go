// Package linuxmint checks and downloads Linux Mint ISOs from
// pub.linuxmint.io's "stable" release path.
package linuxmint

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/arnaudcharles/iso-auto-downloader/internal/download"
	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
	"github.com/arnaudcharles/iso-auto-downloader/internal/scrape"
)

// baseURL is a var (not a const) so tests can point it at an httptest server.
var baseURL = "https://pub.linuxmint.io/stable/"

var editions = []string{"cinnamon", "mate", "xfce"}

var dirVersionRe = regexp.MustCompile(`^([\d.]+)/$`)

type LinuxMint struct{}

func init() {
	provider.Register(LinuxMint{})
}

func (LinuxMint) ID() string                  { return "linuxmint" }
func (LinuxMint) Name() string                { return "Linux Mint" }
func (LinuxMint) Category() provider.Category { return provider.CategoryLinux }

func (LinuxMint) Variants() []provider.Variant {
	variants := make([]provider.Variant, len(editions))
	for i, e := range editions {
		variants[i] = provider.Variant{Edition: e}
	}
	return variants
}

func latestVersion(ctx context.Context) (string, error) {
	body, err := scrape.FetchString(ctx, baseURL)
	if err != nil {
		return "", err
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
		return "", fmt.Errorf("linuxmint: no version directory found at %s", baseURL)
	}
	return best, nil
}

func (LinuxMint) Check(ctx context.Context, _ provider.Variant) (provider.CheckResult, error) {
	version, err := latestVersion(ctx)
	if err != nil {
		return provider.CheckResult{}, err
	}
	return provider.CheckResult{Version: version}, nil
}

func (LinuxMint) LocalVersion(filename string, v provider.Variant) (string, bool) {
	re := regexp.MustCompile(fmt.Sprintf(`^linuxmint-([\d.]+)-%s-64bit\.iso$`, regexp.QuoteMeta(v.Edition)))
	return scrape.MatchVersion(re, filename)
}

func (LinuxMint) Checksum(ctx context.Context, v provider.Variant, version string) (*download.Checksum, error) {
	versionDir := baseURL + version + "/"
	filename := fmt.Sprintf("linuxmint-%s-%s-64bit.iso", version, v.Edition)

	sums, err := scrape.FetchString(ctx, versionDir+"sha256sum.txt")
	if err != nil {
		return nil, err
	}
	checksum, err := scrape.ChecksumForFile(sums, filename)
	if err != nil {
		return nil, err
	}
	return &download.Checksum{Algo: download.SHA256, Hex: checksum}, nil
}

func (LinuxMint) Download(ctx context.Context, v provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	versionDir := baseURL + res.Version + "/"
	filename := fmt.Sprintf("linuxmint-%s-%s-64bit.iso", res.Version, v.Edition)
	fileURL := versionDir + filename

	checksum, err := (LinuxMint{}).Checksum(ctx, v, res.Version)
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
