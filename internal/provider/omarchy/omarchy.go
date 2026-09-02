// Package omarchy checks and downloads the Omarchy install ISO.
//
// Omarchy publishes no directory listing and no GitHub release asset for
// its ISO: omarchy.org links straight to the current build on
// iso.omarchy.org, and that link is the only place the live version is
// stated. Check therefore reads the homepage and picks the newest
// omarchy-<version>.iso it points at; Download re-derives the URL from
// that version alone, as the Provider contract requires.
package omarchy

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/arnaudcharles/iso-auto-downloader/internal/download"
	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
	"github.com/arnaudcharles/iso-auto-downloader/internal/scrape"
)

// siteURL and isoBaseURL are vars (not consts) so tests can point them at
// an httptest server.
var siteURL = "https://omarchy.org/"
var isoBaseURL = "https://iso.omarchy.org/"

var isoFileRe = regexp.MustCompile(`^omarchy-(\d[\d.]*)\.iso$`)

type Omarchy struct{}

func init() {
	provider.Register(Omarchy{})
}

func (Omarchy) ID() string                  { return "omarchy" }
func (Omarchy) Name() string                { return "Omarchy" }
func (Omarchy) Category() provider.Category { return provider.CategoryLinux }

// Variants: Omarchy ships a single x86_64 image, so there's nothing to
// pick between.
func (Omarchy) Variants() []provider.Variant {
	return []provider.Variant{{}}
}

func isoFilename(version string) string {
	return fmt.Sprintf("omarchy-%s.iso", version)
}

func (Omarchy) Check(ctx context.Context, _ provider.Variant) (provider.CheckResult, error) {
	body, err := scrape.FetchString(ctx, siteURL)
	if err != nil {
		return provider.CheckResult{}, err
	}

	best := ""
	for _, href := range scrape.Hrefs(body) {
		// The homepage links the ISO absolutely (iso.omarchy.org), so match
		// on the filename rather than on the host — a mirror or a relative
		// link would otherwise be missed.
		name := path.Base(strings.SplitN(href, "?", 2)[0])
		m := isoFileRe.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		if best == "" || scrape.CompareVersions(m[1], best) > 0 {
			best = m[1]
		}
	}
	if best == "" {
		return provider.CheckResult{}, fmt.Errorf("omarchy: no ISO link found at %s", siteURL)
	}
	return provider.CheckResult{Version: best}, nil
}

func (Omarchy) Checksum(ctx context.Context, _ provider.Variant, version string) (*download.Checksum, error) {
	filename := isoFilename(version)

	sums, err := scrape.FetchString(ctx, isoBaseURL+filename+".sha256")
	if err != nil {
		return nil, err
	}
	checksum, err := scrape.ChecksumForFile(sums, filename)
	if err != nil {
		return nil, err
	}
	return &download.Checksum{Algo: download.SHA256, Hex: checksum}, nil
}

func (Omarchy) Download(ctx context.Context, v provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	filename := isoFilename(res.Version)
	fileURL := isoBaseURL + filename

	checksum, err := (Omarchy{}).Checksum(ctx, v, res.Version)
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

func (Omarchy) LocalVersion(filename string, _ provider.Variant) (string, bool) {
	return scrape.MatchVersion(isoFileRe, filename)
}
