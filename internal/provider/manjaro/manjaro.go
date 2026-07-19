// Package manjaro checks and downloads Manjaro ISOs from manjaro.org's
// download page (server-rendered, so a plain HTTP GET sees the same
// download links a browser would).
//
// The exact filename embeds a build number and kernel version that aren't
// derivable from the release version alone (e.g.
// "manjaro-kde-26.0.4-260327-linux618.iso"), so both Check and Download
// re-scan the download page rather than templating the filename.
package manjaro

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"regexp"

	"github.com/arnaudcharles/iso-auto-downloader/internal/download"
	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
	"github.com/arnaudcharles/iso-auto-downloader/internal/scrape"
)

// pageURL and downloadDomain are vars (not consts) so tests can point them
// at an httptest server instead of the real manjaro.org / download.manjaro.org.
var pageURL = "https://manjaro.org/products/download/x86/"
var downloadDomain = "https://download.manjaro.org"

var editions = []string{"kde", "xfce", "gnome", "cinnamon", "i3"}

type Manjaro struct{}

func init() {
	provider.Register(Manjaro{})
}

func (Manjaro) ID() string                  { return "manjaro" }
func (Manjaro) Name() string                { return "Manjaro" }
func (Manjaro) Category() provider.Category { return provider.CategoryLinux }

func (Manjaro) Variants() []provider.Variant {
	variants := make([]provider.Variant, len(editions))
	for i, e := range editions {
		variants[i] = provider.Variant{Edition: e}
	}
	return variants
}

// isoURLRe matches a Manjaro download URL for the given edition, capturing
// the version component. If version is non-empty, only that exact version
// matches (used by Download to pick the same release Check resolved).
func isoURLRe(edition, version string) *regexp.Regexp {
	v := `([\d.]+)`
	if version != "" {
		v = regexp.QuoteMeta(version)
	}
	e := regexp.QuoteMeta(edition)
	return regexp.MustCompile(fmt.Sprintf(`^%s/%s/[\d.]+/manjaro-%s-%s-\d+-linux\d+\.iso$`, regexp.QuoteMeta(downloadDomain), e, e, v))
}

func (Manjaro) Check(ctx context.Context, v provider.Variant) (provider.CheckResult, error) {
	body, err := scrape.FetchString(ctx, pageURL)
	if err != nil {
		return provider.CheckResult{}, err
	}

	re := isoURLRe(v.Edition, "")
	for _, href := range scrape.Hrefs(body) {
		if m := re.FindStringSubmatch(href); m != nil {
			return provider.CheckResult{Version: m[1]}, nil
		}
	}
	return provider.CheckResult{}, fmt.Errorf("manjaro: no ISO found for edition %q at %s", v.Edition, pageURL)
}

func (Manjaro) LocalVersion(filename string, v provider.Variant) (string, bool) {
	re := regexp.MustCompile(fmt.Sprintf(`^manjaro-%s-([\d.]+)-\d+-linux\d+\.iso$`, regexp.QuoteMeta(v.Edition)))
	return scrape.MatchVersion(re, filename)
}

// findFileURL re-scrapes the download page for the ISO link matching
// edition/version — the exact filename (build number, kernel version)
// isn't derivable from the release version alone, so both Checksum and
// Download need to re-find it rather than templating it.
func findFileURL(ctx context.Context, edition, version string) (string, error) {
	body, err := scrape.FetchString(ctx, pageURL)
	if err != nil {
		return "", err
	}

	re := isoURLRe(edition, version)
	for _, href := range scrape.Hrefs(body) {
		if re.MatchString(href) {
			return href, nil
		}
	}
	return "", fmt.Errorf("manjaro: no ISO found for edition %q version %q at %s", edition, version, pageURL)
}

func (Manjaro) Checksum(ctx context.Context, v provider.Variant, version string) (*download.Checksum, error) {
	fileURL, err := findFileURL(ctx, v.Edition, version)
	if err != nil {
		return nil, err
	}
	filename := path.Base(fileURL)

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

func (Manjaro) Download(ctx context.Context, v provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	fileURL, err := findFileURL(ctx, v.Edition, res.Version)
	if err != nil {
		return provider.DownloadResult{}, err
	}
	filename := path.Base(fileURL)

	checksum, err := (Manjaro{}).Checksum(ctx, v, res.Version)
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
