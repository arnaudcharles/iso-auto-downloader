// Package rockylinux checks and downloads Rocky Linux ISOs from the OVH
// mirror. Each bare major-version directory (e.g. "9/", "10/") on this
// mirror always holds the latest minor release's files, so that's used
// instead of scanning every minor version directory individually. Editions
// occasionally get a trailing "1" in their filename (e.g. "dvd1.iso"
// instead of "dvd.iso"), a known upstream quirk, so filenames are matched
// with an optional trailing "1".
package rockylinux

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
var baseURL = "https://rockylinux.mirrors.ovh.net/"

var majorDirRe = regexp.MustCompile(`^(\d+)/$`)

type RockyLinux struct{}

func init() {
	provider.Register(RockyLinux{})
}

func (RockyLinux) ID() string                  { return "rockylinux" }
func (RockyLinux) Name() string                { return "Rocky Linux" }
func (RockyLinux) Category() provider.Category { return provider.CategoryLinux }

func (RockyLinux) Variants() []provider.Variant {
	editions := []string{"dvd", "boot", "minimal"}
	variants := make([]provider.Variant, len(editions))
	for i, e := range editions {
		variants[i] = provider.Variant{Edition: e, Arch: "x86_64"}
	}
	return variants
}

func latestMajorDir(ctx context.Context) (string, error) {
	body, err := scrape.FetchString(ctx, baseURL)
	if err != nil {
		return "", err
	}
	best := ""
	for _, href := range scrape.Hrefs(body) {
		m := majorDirRe.FindStringSubmatch(href)
		if m == nil {
			continue
		}
		if best == "" || scrape.CompareVersions(m[1], best) > 0 {
			best = m[1]
		}
	}
	if best == "" {
		return "", fmt.Errorf("rockylinux: no version directory found at %s", baseURL)
	}
	return best, nil
}

func isoRe(arch, edition string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`^Rocky-([\d.]+)-%s-%s1?\.iso$`, regexp.QuoteMeta(arch), regexp.QuoteMeta(edition)))
}

// findISO returns the exact filename and version for v's latest ISO.
func findISO(ctx context.Context, v provider.Variant) (dirURL, filename, version string, err error) {
	major, err := latestMajorDir(ctx)
	if err != nil {
		return "", "", "", err
	}
	dirURL = fmt.Sprintf("%s%s/isos/%s/", baseURL, major, v.Arch)

	body, err := scrape.FetchString(ctx, dirURL)
	if err != nil {
		return "", "", "", err
	}

	re := isoRe(v.Arch, v.Edition)
	for _, href := range scrape.Hrefs(body) {
		if m := re.FindStringSubmatch(href); m != nil {
			if version == "" || scrape.CompareVersions(m[1], version) > 0 {
				version = m[1]
				filename = href
			}
		}
	}
	if filename == "" {
		return "", "", "", fmt.Errorf("rockylinux: no %s/%s ISO found at %s", v.Edition, v.Arch, dirURL)
	}
	return dirURL, filename, version, nil
}

func (RockyLinux) Check(ctx context.Context, v provider.Variant) (provider.CheckResult, error) {
	_, _, version, err := findISO(ctx, v)
	if err != nil {
		return provider.CheckResult{}, err
	}
	return provider.CheckResult{Version: version}, nil
}

// Checksum ignores the requested version, same reasoning as Download below:
// this mirror's bare major-version directory always holds the latest minor
// release, so findISO has no way to target an older one anyway.
func (RockyLinux) Checksum(ctx context.Context, v provider.Variant, _ string) (*download.Checksum, error) {
	dirURL, filename, _, err := findISO(ctx, v)
	if err != nil {
		return nil, err
	}

	sums, err := scrape.FetchString(ctx, dirURL+"CHECKSUM")
	if err != nil {
		return nil, err
	}
	hex, err := scrape.ChecksumForFileBSD(sums, filename)
	if err != nil {
		return nil, err
	}
	return &download.Checksum{Algo: download.SHA256, Hex: hex}, nil
}

func (RockyLinux) Download(ctx context.Context, v provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	dirURL, filename, _, err := findISO(ctx, v)
	if err != nil {
		return provider.DownloadResult{}, err
	}

	checksum, err := (RockyLinux{}).Checksum(ctx, v, res.Version)
	if err != nil {
		return provider.DownloadResult{}, err
	}

	dest := filepath.Join(destDir, filename)
	size, err := download.Download(ctx, download.Options{
		URL:      dirURL + filename,
		DestPath: dest,
		Checksum: checksum,
		Progress: download.ProgressFunc(progress),
	})
	if err != nil {
		return provider.DownloadResult{}, err
	}
	return provider.DownloadResult{Path: dest, Filename: filename, SizeBytes: size}, nil
}

func (RockyLinux) LocalVersion(filename string, v provider.Variant) (string, bool) {
	return scrape.MatchVersion(isoRe(v.Arch, v.Edition), filename)
}
