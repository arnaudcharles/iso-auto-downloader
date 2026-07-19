// Package truenas checks and downloads the TrueNAS SCALE ISO from
// truenas.com. The download URL embeds a release codename (e.g.
// "TrueNAS-SCALE-Goldeye") not derivable from the version number alone, so
// Check and Download both re-scan the download page for the exact URL
// instead of templating it.
package truenas

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

// pageURL is a var (not a const) so tests can point it at an httptest server.
var pageURL = "https://www.truenas.com/download/"

var isoURLRe = regexp.MustCompile(`^https?://\S+/TrueNAS-SCALE-([\d.]+)\.iso$`)

type TrueNAS struct{}

func init() {
	provider.Register(TrueNAS{})
}

func (TrueNAS) ID() string                  { return "truenas" }
func (TrueNAS) Name() string                { return "TrueNAS" }
func (TrueNAS) Category() provider.Category { return provider.CategoryLinux }

func (TrueNAS) Variants() []provider.Variant {
	return []provider.Variant{{}}
}

func findISOURL(ctx context.Context) (fileURL, version string, err error) {
	body, err := scrape.FetchString(ctx, pageURL)
	if err != nil {
		return "", "", err
	}
	for _, href := range scrape.Hrefs(body) {
		if m := isoURLRe.FindStringSubmatch(href); m != nil {
			return href, m[1], nil
		}
	}
	return "", "", fmt.Errorf("truenas: no stable SCALE ISO found at %s", pageURL)
}

func (TrueNAS) Check(ctx context.Context, _ provider.Variant) (provider.CheckResult, error) {
	_, version, err := findISOURL(ctx)
	if err != nil {
		return provider.CheckResult{}, err
	}
	return provider.CheckResult{Version: version}, nil
}

// Checksum ignores the requested version and always verifies against
// whatever's currently live on the download page — same as Download below,
// which has no way to target an older SCALE release either (findISOURL
// only ever returns the current stable build).
func (TrueNAS) Checksum(ctx context.Context, _ provider.Variant, _ string) (*download.Checksum, error) {
	fileURL, _, err := findISOURL(ctx)
	if err != nil {
		return nil, err
	}

	sum, err := scrape.FetchString(ctx, fileURL+".sha256")
	if err != nil {
		return nil, err
	}
	return &download.Checksum{Algo: download.SHA256, Hex: strings.TrimSpace(sum)}, nil
}

func (TrueNAS) Download(ctx context.Context, v provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	fileURL, _, err := findISOURL(ctx)
	if err != nil {
		return provider.DownloadResult{}, err
	}
	filename := path.Base(fileURL)

	checksum, err := (TrueNAS{}).Checksum(ctx, v, res.Version)
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

func (TrueNAS) LocalVersion(filename string, _ provider.Variant) (string, bool) {
	re := regexp.MustCompile(`^TrueNAS-SCALE-([\d.]+)\.iso$`)
	return scrape.MatchVersion(re, filename)
}
