// Package clonezilla checks and downloads Clonezilla Live ISOs. Versions
// and checksums come from clonezilla.org's combined CHECKSUMS.TXT; the
// actual file is served from SourceForge (which redirects to a mirror).
package clonezilla

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/arnaudcharles/iso-auto-downloader/internal/download"
	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
	"github.com/arnaudcharles/iso-auto-downloader/internal/scrape"
)

// checksumsURL and downloadBaseURL are vars (not consts) so tests can point
// them at an httptest server.
//
// downloadBaseURL deliberately points at downloads.sourceforge.net (the
// direct mirror-selector endpoint) rather than
// sourceforge.net/projects/.../files/.../download (the front-end page a
// browser would land on first). The latter intermittently serves an HTML
// "your download will begin shortly" interstitial with a 200 status
// instead of redirecting through to an actual mirror when requested by
// Go's http.Client — each interstitial has a different embedded token, so
// even retrying failed checksum verification every time. Hitting the
// direct endpoint skips that page and reliably returns the real ISO.
var checksumsURL = "https://clonezilla.org/downloads/stable/data/CHECKSUMS.TXT"
var downloadBaseURL = "https://downloads.sourceforge.net/project/clonezilla/clonezilla_live_stable/"

type Clonezilla struct{}

func init() {
	provider.Register(Clonezilla{})
}

func (Clonezilla) ID() string                  { return "clonezilla" }
func (Clonezilla) Name() string                { return "Clonezilla" }
func (Clonezilla) Category() provider.Category { return provider.CategoryDiskUtility }

func (Clonezilla) Variants() []provider.Variant {
	return []provider.Variant{{Arch: "amd64"}}
}

func versionRe(arch string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`clonezilla-live-([\d.-]+)-%s\.iso`, regexp.QuoteMeta(arch)))
}

func (Clonezilla) Check(ctx context.Context, v provider.Variant) (provider.CheckResult, error) {
	body, err := scrape.FetchString(ctx, checksumsURL)
	if err != nil {
		return provider.CheckResult{}, err
	}
	m := versionRe(v.Arch).FindStringSubmatch(body)
	if m == nil {
		return provider.CheckResult{}, fmt.Errorf("clonezilla: no version found at %s", checksumsURL)
	}
	return provider.CheckResult{Version: m[1]}, nil
}

func (Clonezilla) Checksum(ctx context.Context, v provider.Variant, version string) (*download.Checksum, error) {
	filename := fmt.Sprintf("clonezilla-live-%s-%s.iso", version, v.Arch)

	checksums, err := scrape.FetchString(ctx, checksumsURL)
	if err != nil {
		return nil, err
	}
	section := scrape.ExtractLabeledSection(checksums, "SHA256SUMS")
	checksum, err := scrape.ChecksumForFile(section, filename)
	if err != nil {
		return nil, err
	}
	return &download.Checksum{Algo: download.SHA256, Hex: checksum}, nil
}

func (Clonezilla) Download(ctx context.Context, v provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	filename := fmt.Sprintf("clonezilla-live-%s-%s.iso", res.Version, v.Arch)
	fileURL := downloadBaseURL + res.Version + "/" + filename

	checksum, err := (Clonezilla{}).Checksum(ctx, v, res.Version)
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

func (Clonezilla) LocalVersion(filename string, v provider.Variant) (string, bool) {
	re := regexp.MustCompile(fmt.Sprintf(`^clonezilla-live-([\d.-]+)-%s\.iso$`, regexp.QuoteMeta(v.Arch)))
	return scrape.MatchVersion(re, filename)
}
