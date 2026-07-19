// Package opensuse checks and downloads the openSUSE Leap (stable,
// versioned) and Tumbleweed (rolling) DVD ISOs from download.opensuse.org.
//
// Both editions publish a stable "Current.iso" alias that always points
// (via HTTP redirect) at whatever the latest actual build is, so Download
// fetches that alias directly rather than discovering the exact respin
// filename first — the downloaded file is saved locally under this
// package's own naming, not the ever-changing "Current"/"BuildNNN"/
// "SnapshotYYYYMMDD" name on the mirror.
package opensuse

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

// leapIndexURL and tumbleweedURL are vars (not consts) so tests can point
// them at an httptest server.
var (
	leapIndexURL  = "https://download.opensuse.org/distribution/leap/"
	tumbleweedURL = "https://download.opensuse.org/tumbleweed/iso/"
)

var leapVersionDirRe = regexp.MustCompile(`^\./?(\d+\.\d+)/$`)
var tumbleweedSnapshotRe = regexp.MustCompile(`openSUSE-Tumbleweed-DVD-x86_64-Snapshot(\d{8})-Media\.iso`)
var sha256LineRe = regexp.MustCompile(`^([a-fA-F0-9]{64})\s`)

type OpenSUSE struct{}

func init() {
	provider.Register(OpenSUSE{})
}

func (OpenSUSE) ID() string                  { return "opensuse" }
func (OpenSUSE) Name() string                { return "openSUSE" }
func (OpenSUSE) Category() provider.Category { return provider.CategoryLinux }

func (OpenSUSE) Variants() []provider.Variant {
	return []provider.Variant{
		{Edition: "leap", Arch: "x86_64"},
		{Edition: "tumbleweed", Arch: "x86_64"},
	}
}

func latestLeapVersion(ctx context.Context) (string, error) {
	body, err := scrape.FetchString(ctx, leapIndexURL)
	if err != nil {
		return "", err
	}
	best := ""
	for _, href := range scrape.Hrefs(body) {
		m := leapVersionDirRe.FindStringSubmatch(href)
		if m == nil {
			continue
		}
		// Leap briefly used a "42.x" numbering scheme (2016-2018) before
		// reverting to the current "15.x"/"16.x" scheme aligned with SLE —
		// a leftover "42.3" archive directory still sorts numerically
		// higher than "16.1", so it's excluded explicitly.
		if strings.HasPrefix(m[1], "42.") {
			continue
		}
		if best == "" || scrape.CompareVersions(m[1], best) > 0 {
			best = m[1]
		}
	}
	if best == "" {
		return "", fmt.Errorf("opensuse: no Leap version directory found at %s", leapIndexURL)
	}
	return best, nil
}

func latestTumbleweedSnapshot(ctx context.Context) (string, error) {
	body, err := scrape.FetchString(ctx, tumbleweedURL)
	if err != nil {
		return "", err
	}
	best := ""
	for _, href := range scrape.Hrefs(body) {
		m := tumbleweedSnapshotRe.FindStringSubmatch(href)
		if m == nil {
			continue
		}
		if m[1] > best {
			best = m[1]
		}
	}
	if best == "" {
		return "", fmt.Errorf("opensuse: no Tumbleweed snapshot found at %s", tumbleweedURL)
	}
	return best, nil
}

func (OpenSUSE) Check(ctx context.Context, v provider.Variant) (provider.CheckResult, error) {
	if v.Edition == "tumbleweed" {
		version, err := latestTumbleweedSnapshot(ctx)
		if err != nil {
			return provider.CheckResult{}, err
		}
		return provider.CheckResult{Version: version}, nil
	}
	version, err := latestLeapVersion(ctx)
	if err != nil {
		return provider.CheckResult{}, err
	}
	return provider.CheckResult{Version: version}, nil
}

// currentISOURL and checksumURL return the mirror's stable "Current" alias
// for edition/version, and its dedicated single-hash checksum sidecar.
//
// Leap 16.x dropped the classic "iso/openSUSE-Leap-<version>-DVD-x86_64"
// layout entirely (that directory 404s now) in favor of an Agama-installer
// ISO under "installer/iso/"; "agama-installer.x86_64-Leap_<version>.iso"
// (no build number) is the stable alias that always redirects to the latest
// respin, same idea as Tumbleweed's own "-Current" alias below.
func currentISOURL(edition, version string) string {
	if edition == "tumbleweed" {
		return tumbleweedURL + "openSUSE-Tumbleweed-DVD-x86_64-Current.iso"
	}
	return fmt.Sprintf("%s%s/installer/iso/agama-installer.x86_64-Leap_%s.iso", leapIndexURL, version, version)
}

func localFilename(edition, version string) string {
	if edition == "tumbleweed" {
		return fmt.Sprintf("openSUSE-Tumbleweed-%s-DVD-x86_64.iso", version)
	}
	return fmt.Sprintf("openSUSE-Leap-%s-installer-x86_64.iso", version)
}

func (OpenSUSE) LocalVersion(filename string, v provider.Variant) (string, bool) {
	return scrape.MatchVersion(localVersionRe(v.Edition), filename)
}

func localVersionRe(edition string) *regexp.Regexp {
	if edition == "tumbleweed" {
		return regexp.MustCompile(`^openSUSE-Tumbleweed-(\d{8})-DVD-x86_64\.iso$`)
	}
	return regexp.MustCompile(`^openSUSE-Leap-([\d.]+)-installer-x86_64\.iso$`)
}

func firstSHA256(text string) (string, error) {
	m := sha256LineRe.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return "", fmt.Errorf("opensuse: could not find a SHA256 hash in checksum response")
	}
	return m[1], nil
}

func (OpenSUSE) Checksum(ctx context.Context, v provider.Variant, version string) (*download.Checksum, error) {
	isoURL := currentISOURL(v.Edition, version)

	sums, err := scrape.FetchString(ctx, isoURL+".sha256")
	if err != nil {
		return nil, err
	}
	checksum, err := firstSHA256(sums)
	if err != nil {
		return nil, err
	}
	return &download.Checksum{Algo: download.SHA256, Hex: checksum}, nil
}

func (OpenSUSE) Download(ctx context.Context, v provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	isoURL := currentISOURL(v.Edition, res.Version)

	checksum, err := (OpenSUSE{}).Checksum(ctx, v, res.Version)
	if err != nil {
		return provider.DownloadResult{}, err
	}

	filename := localFilename(v.Edition, res.Version)
	dest := filepath.Join(destDir, filename)
	size, err := download.Download(ctx, download.Options{
		URL:      isoURL,
		DestPath: dest,
		Checksum: checksum,
		Progress: download.ProgressFunc(progress),
	})
	if err != nil {
		return provider.DownloadResult{}, err
	}
	return provider.DownloadResult{Path: dest, Filename: filename, SizeBytes: size}, nil
}
