// Package ultimatebootcd checks and downloads Ultimate Boot CD from its
// ClientVPS mirror. No checksum is published upstream.
package ultimatebootcd

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/arnaudcharles/iso-auto-downloader/internal/download"
	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
	"github.com/arnaudcharles/iso-auto-downloader/internal/scrape"
)

// mirrorURL is a var (not a const) so tests can point it at an httptest server.
var mirrorURL = "https://mirror.clientvps.com/ubcd/"

var versionRe = regexp.MustCompile(`^ubcd(\d+)\.iso$`)

type UltimateBootCD struct{}

func init() {
	provider.Register(UltimateBootCD{})
}

func (UltimateBootCD) ID() string                  { return "ultimatebootcd" }
func (UltimateBootCD) Name() string                { return "Ultimate Boot CD" }
func (UltimateBootCD) Category() provider.Category { return provider.CategoryDiagnostic }

func (UltimateBootCD) Variants() []provider.Variant {
	return []provider.Variant{{}}
}

func (UltimateBootCD) Check(ctx context.Context, _ provider.Variant) (provider.CheckResult, error) {
	body, err := scrape.FetchString(ctx, mirrorURL)
	if err != nil {
		return provider.CheckResult{}, err
	}

	best := ""
	for _, href := range scrape.Hrefs(body) {
		m := versionRe.FindStringSubmatch(href)
		if m == nil {
			continue
		}
		if best == "" || scrape.CompareVersions(m[1], best) > 0 {
			best = m[1]
		}
	}
	if best == "" {
		return provider.CheckResult{}, fmt.Errorf("ultimatebootcd: no version found at %s", mirrorURL)
	}
	return provider.CheckResult{Version: best}, nil
}

func (UltimateBootCD) Download(ctx context.Context, _ provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	filename := fmt.Sprintf("ubcd%s.iso", res.Version)
	dest := filepath.Join(destDir, filename)
	size, err := download.Download(ctx, download.Options{
		URL:      mirrorURL + filename,
		DestPath: dest,
		Progress: download.ProgressFunc(progress),
	})
	if err != nil {
		return provider.DownloadResult{}, err
	}
	return provider.DownloadResult{Path: dest, Filename: filename, SizeBytes: size}, nil
}

// Checksum: no checksum is published upstream (see the package doc
// comment), so there's nothing to verify against.
func (UltimateBootCD) Checksum(context.Context, provider.Variant, string) (*download.Checksum, error) {
	return nil, nil
}

func (UltimateBootCD) LocalVersion(filename string, _ provider.Variant) (string, bool) {
	return scrape.MatchVersion(versionRe, filename)
}
