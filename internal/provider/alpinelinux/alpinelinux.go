// Package alpinelinux checks and downloads the Alpine Linux standard ISO
// from the OVH mirror.
package alpinelinux

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
var baseURL = "https://alpinelinux.mirrors.ovh.net/latest-stable/releases/"

type AlpineLinux struct{}

func init() {
	provider.Register(AlpineLinux{})
}

func (AlpineLinux) ID() string                  { return "alpinelinux" }
func (AlpineLinux) Name() string                { return "Alpine Linux" }
func (AlpineLinux) Category() provider.Category { return provider.CategoryLinux }

func (AlpineLinux) Variants() []provider.Variant {
	return []provider.Variant{{Edition: "standard", Arch: "x86_64"}}
}

func dirURL(arch string) string {
	return fmt.Sprintf("%s%s/", baseURL, arch)
}

func isoRe(edition, arch string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`^alpine-%s-([\d.]+)-%s\.iso$`, regexp.QuoteMeta(edition), regexp.QuoteMeta(arch)))
}

func (AlpineLinux) Check(ctx context.Context, v provider.Variant) (provider.CheckResult, error) {
	dir := dirURL(v.Arch)
	body, err := scrape.FetchString(ctx, dir)
	if err != nil {
		return provider.CheckResult{}, err
	}

	re := isoRe(v.Edition, v.Arch)
	best := ""
	for _, href := range scrape.Hrefs(body) {
		m := re.FindStringSubmatch(href)
		if m == nil {
			continue
		}
		if best == "" || scrape.CompareVersions(m[1], best) > 0 {
			best = m[1]
		}
	}
	if best == "" {
		return provider.CheckResult{}, fmt.Errorf("alpinelinux: no ISO found at %s", dir)
	}
	return provider.CheckResult{Version: best}, nil
}

func (AlpineLinux) Checksum(ctx context.Context, v provider.Variant, version string) (*download.Checksum, error) {
	filename := fmt.Sprintf("alpine-%s-%s-%s.iso", v.Edition, version, v.Arch)
	fileURL := dirURL(v.Arch) + filename

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

func (AlpineLinux) Download(ctx context.Context, v provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	dir := dirURL(v.Arch)
	filename := fmt.Sprintf("alpine-%s-%s-%s.iso", v.Edition, res.Version, v.Arch)
	fileURL := dir + filename

	checksum, err := (AlpineLinux{}).Checksum(ctx, v, res.Version)
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

func (AlpineLinux) LocalVersion(filename string, v provider.Variant) (string, bool) {
	return scrape.MatchVersion(isoRe(v.Edition, v.Arch), filename)
}
