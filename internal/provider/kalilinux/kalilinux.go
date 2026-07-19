// Package kalilinux checks and downloads the Kali Linux installer ISO from
// cdimage.kali.org's "current" release path. Only the plain "installer"
// edition is implemented for now (installer-everything, installer-netinst,
// installer-purple, and live also exist upstream, not implemented here).
package kalilinux

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/arnaudcharles/iso-auto-downloader/internal/download"
	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
	"github.com/arnaudcharles/iso-auto-downloader/internal/scrape"
)

// currentURL is a var (not a const) so tests can point it at an httptest server.
var currentURL = "https://cdimage.kali.org/current/"

type KaliLinux struct{}

func init() {
	provider.Register(KaliLinux{})
}

func (KaliLinux) ID() string                  { return "kalilinux" }
func (KaliLinux) Name() string                { return "Kali Linux" }
func (KaliLinux) Category() provider.Category { return provider.CategoryLinux }

func (KaliLinux) Variants() []provider.Variant {
	return []provider.Variant{
		{Edition: "installer", Arch: "amd64"},
		{Edition: "installer", Arch: "arm64"},
	}
}

func (KaliLinux) Check(ctx context.Context, v provider.Variant) (provider.CheckResult, error) {
	body, err := scrape.FetchString(ctx, currentURL)
	if err != nil {
		return provider.CheckResult{}, err
	}

	re := regexp.MustCompile(fmt.Sprintf(`^kali-linux-([\d.]+)-%s-%s\.iso$`, regexp.QuoteMeta(v.Edition), regexp.QuoteMeta(v.Arch)))
	for _, href := range scrape.Hrefs(body) {
		if m := re.FindStringSubmatch(href); m != nil {
			return provider.CheckResult{Version: m[1]}, nil
		}
	}
	return provider.CheckResult{}, fmt.Errorf("kalilinux: no ISO found for %s/%s at %s", v.Edition, v.Arch, currentURL)
}

func (KaliLinux) LocalVersion(filename string, v provider.Variant) (string, bool) {
	re := regexp.MustCompile(fmt.Sprintf(`^kali-linux-([\d.]+)-%s-%s\.iso$`, regexp.QuoteMeta(v.Edition), regexp.QuoteMeta(v.Arch)))
	return scrape.MatchVersion(re, filename)
}

func (KaliLinux) Checksum(ctx context.Context, v provider.Variant, version string) (*download.Checksum, error) {
	filename := fmt.Sprintf("kali-linux-%s-%s-%s.iso", version, v.Edition, v.Arch)

	sums, err := scrape.FetchString(ctx, currentURL+"SHA256SUMS")
	if err != nil {
		return nil, err
	}
	checksum, err := scrape.ChecksumForFile(sums, filename)
	if err != nil {
		return nil, err
	}
	return &download.Checksum{Algo: download.SHA256, Hex: checksum}, nil
}

func (KaliLinux) Download(ctx context.Context, v provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	filename := fmt.Sprintf("kali-linux-%s-%s-%s.iso", res.Version, v.Edition, v.Arch)
	fileURL := currentURL + filename

	checksum, err := (KaliLinux{}).Checksum(ctx, v, res.Version)
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
