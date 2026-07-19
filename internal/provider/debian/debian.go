// Package debian checks and downloads Debian netinst and full-DVD ISOs from
// cdimage.debian.org's "current" release path (netinst lives under
// "iso-cd/", the full DVD-1 set under "iso-dvd/", same base URL).
package debian

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
var baseURL = "https://cdimage.debian.org/debian-cd/current/"

var archs = []string{"amd64", "arm64", "armhf", "ppc64el", "riscv64", "s390x"}

type Debian struct{}

func init() {
	provider.Register(Debian{})
}

func (Debian) ID() string                  { return "debian" }
func (Debian) Name() string                { return "Debian" }
func (Debian) Category() provider.Category { return provider.CategoryLinux }

var editions = []string{"netinst", "dvd"}

func (Debian) Variants() []provider.Variant {
	variants := make([]provider.Variant, 0, len(archs)*len(editions))
	for _, e := range editions {
		for _, a := range archs {
			variants = append(variants, provider.Variant{Edition: e, Arch: a})
		}
	}
	return variants
}

// dirURL and isoFilenameRe differ per edition: netinst lives under
// "iso-cd/" and is named "debian-<version>-<arch>-netinst.iso"; the full
// DVD-1 set lives under "iso-dvd/" and is named
// "debian-<version>-<arch>-DVD-1.iso".
func dirURL(edition, arch string) string {
	if edition == "dvd" {
		return fmt.Sprintf("%s%s/iso-dvd/", baseURL, arch)
	}
	return fmt.Sprintf("%s%s/iso-cd/", baseURL, arch)
}

func isoFilenameRe(edition, arch string) *regexp.Regexp {
	if edition == "dvd" {
		return regexp.MustCompile(fmt.Sprintf(`^debian-([\d.]+)-%s-DVD-1\.iso$`, regexp.QuoteMeta(arch)))
	}
	return regexp.MustCompile(fmt.Sprintf(`^debian-([\d.]+)-%s-netinst\.iso$`, regexp.QuoteMeta(arch)))
}

func (Debian) Check(ctx context.Context, v provider.Variant) (provider.CheckResult, error) {
	dir := dirURL(v.Edition, v.Arch)
	body, err := scrape.FetchString(ctx, dir)
	if err != nil {
		return provider.CheckResult{}, err
	}

	re := isoFilenameRe(v.Edition, v.Arch)
	for _, href := range scrape.Hrefs(body) {
		if m := re.FindStringSubmatch(href); m != nil {
			return provider.CheckResult{Version: m[1]}, nil
		}
	}
	return provider.CheckResult{}, fmt.Errorf("debian: no %s ISO found at %s", v.Edition, dir)
}

func (Debian) LocalVersion(filename string, v provider.Variant) (string, bool) {
	return scrape.MatchVersion(isoFilenameRe(v.Edition, v.Arch), filename)
}

func filenameFor(edition, arch, version string) string {
	suffix := "netinst"
	if edition == "dvd" {
		suffix = "DVD-1"
	}
	return fmt.Sprintf("debian-%s-%s-%s.iso", version, arch, suffix)
}

func (Debian) Checksum(ctx context.Context, v provider.Variant, version string) (*download.Checksum, error) {
	dir := dirURL(v.Edition, v.Arch)
	filename := filenameFor(v.Edition, v.Arch, version)

	sums, err := scrape.FetchString(ctx, dir+"SHA256SUMS")
	if err != nil {
		return nil, err
	}
	checksum, err := scrape.ChecksumForFile(sums, filename)
	if err != nil {
		return nil, err
	}
	return &download.Checksum{Algo: download.SHA256, Hex: checksum}, nil
}

func (Debian) Download(ctx context.Context, v provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	dir := dirURL(v.Edition, v.Arch)
	filename := filenameFor(v.Edition, v.Arch, res.Version)
	fileURL := dir + filename

	checksum, err := (Debian{}).Checksum(ctx, v, res.Version)
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
