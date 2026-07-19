// Package parrotos checks and downloads the ParrotOS Security and Home
// edition ISOs from deb.parrot.sh. No checksum sidecar is published for
// ISOs on this mirror, so downloads aren't checksum-verified.
package parrotos

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
var baseURL = "https://deb.parrot.sh/parrot/iso/"

var dirVersionRe = regexp.MustCompile(`^(\d+(?:\.\d+)*)/$`)

type ParrotOS struct{}

func init() {
	provider.Register(ParrotOS{})
}

func (ParrotOS) ID() string                  { return "parrotos" }
func (ParrotOS) Name() string                { return "ParrotOS" }
func (ParrotOS) Category() provider.Category { return provider.CategoryLinux }

func (ParrotOS) Variants() []provider.Variant {
	return []provider.Variant{{Edition: "security"}, {Edition: "home"}}
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
		return "", fmt.Errorf("parrotos: no version directory found at %s", baseURL)
	}
	return best, nil
}

func (ParrotOS) Check(ctx context.Context, _ provider.Variant) (provider.CheckResult, error) {
	version, err := latestVersion(ctx)
	if err != nil {
		return provider.CheckResult{}, err
	}
	return provider.CheckResult{Version: version}, nil
}

func filename(edition, version string) string {
	return fmt.Sprintf("Parrot-%s-%s_amd64.iso", edition, version)
}

func (ParrotOS) Download(ctx context.Context, v provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	fname := filename(v.Edition, res.Version)
	fileURL := baseURL + res.Version + "/" + fname

	dest := filepath.Join(destDir, fname)
	size, err := download.Download(ctx, download.Options{
		URL:      fileURL,
		DestPath: dest,
		Progress: download.ProgressFunc(progress),
	})
	if err != nil {
		return provider.DownloadResult{}, err
	}
	return provider.DownloadResult{Path: dest, Filename: fname, SizeBytes: size}, nil
}

// Checksum: no checksum sidecar is published on this mirror (see the
// package doc comment), so there's nothing to verify against.
func (ParrotOS) Checksum(context.Context, provider.Variant, string) (*download.Checksum, error) {
	return nil, nil
}

func (ParrotOS) LocalVersion(fname string, v provider.Variant) (string, bool) {
	re := regexp.MustCompile(fmt.Sprintf(`^Parrot-%s-([\d.]+)_amd64\.iso$`, regexp.QuoteMeta(v.Edition)))
	return scrape.MatchVersion(re, fname)
}
