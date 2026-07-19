// Package proxmox checks and downloads the Proxmox VE installer ISO from
// Proxmox's own enterprise mirror. Single edition, amd64 only.
package proxmox

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
var baseURL = "https://enterprise.proxmox.com/iso/"

// isoRe matches only the "proxmox-ve" product, not the neighboring
// proxmox-backup-server/proxmox-mail-gateway/proxmox-datacenter-manager
// ISOs the same directory also lists. The directory listing's hrefs are
// relative ("./proxmox-ve_9.2-1.iso"), hence the optional "./" prefix.
var isoRe = regexp.MustCompile(`^(?:\./)?proxmox-ve_([\d.]+-\d+)\.iso$`)

type Proxmox struct{}

func init() {
	provider.Register(Proxmox{})
}

func (Proxmox) ID() string                  { return "proxmox" }
func (Proxmox) Name() string                { return "Proxmox VE" }
func (Proxmox) Category() provider.Category { return provider.CategoryVirtualization }

func (Proxmox) Variants() []provider.Variant {
	return []provider.Variant{{Arch: "amd64"}}
}

func (Proxmox) Check(ctx context.Context, _ provider.Variant) (provider.CheckResult, error) {
	body, err := scrape.FetchString(ctx, baseURL)
	if err != nil {
		return provider.CheckResult{}, err
	}
	best := ""
	for _, href := range scrape.Hrefs(body) {
		m := isoRe.FindStringSubmatch(href)
		if m == nil {
			continue
		}
		// CompareVersions only splits on ".", so the "-<build>" suffix
		// (e.g. "9.2-1") must be normalized to a dot first, or every
		// version after the last real dot compares as equal.
		if best == "" || scrape.CompareVersions(normalizeVersion(m[1]), normalizeVersion(best)) > 0 {
			best = m[1]
		}
	}
	if best == "" {
		return provider.CheckResult{}, fmt.Errorf("proxmox: no proxmox-ve ISO found at %s", baseURL)
	}
	return provider.CheckResult{Version: best}, nil
}

func normalizeVersion(v string) string {
	return strings.ReplaceAll(v, "-", ".")
}

func (Proxmox) LocalVersion(filename string, _ provider.Variant) (string, bool) {
	return scrape.MatchVersion(isoRe, filename)
}

func (Proxmox) Checksum(ctx context.Context, _ provider.Variant, version string) (*download.Checksum, error) {
	filename := fmt.Sprintf("proxmox-ve_%s.iso", version)

	sums, err := scrape.FetchString(ctx, baseURL+"SHA256SUMS")
	if err != nil {
		return nil, err
	}
	checksum, err := scrape.ChecksumForFile(sums, filename)
	if err != nil {
		return nil, err
	}
	return &download.Checksum{Algo: download.SHA256, Hex: checksum}, nil
}

func (Proxmox) Download(ctx context.Context, v provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	filename := fmt.Sprintf("proxmox-ve_%s.iso", res.Version)
	fileURL := baseURL + filename

	checksum, err := (Proxmox{}).Checksum(ctx, v, res.Version)
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
