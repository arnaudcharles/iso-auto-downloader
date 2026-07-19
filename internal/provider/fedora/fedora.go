// Package fedora checks and downloads Fedora Workstation and Server ISOs
// from download.fedoraproject.org (Silverblue, Kinoite, and several Spins
// also exist upstream, not implemented here).
//
// Unlike Ubuntu/Debian, the exact filename embeds a respin number that
// isn't derivable from the release version alone, so Download re-lists the
// ISO directory to find the exact filename instead of templating it. The
// two editions also use different filename shapes entirely:
// Workstation is "Fedora-Workstation-Live-<version>-<respin>.<arch>.iso"
// (arch as a dotted suffix at the end); Server is
// "Fedora-Server-dvd-<arch>-<version>-<respin>.iso" (arch right after
// "dvd", before the version, no "Live" in the name).
package fedora

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/arnaudcharles/iso-auto-downloader/internal/download"
	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
	"github.com/arnaudcharles/iso-auto-downloader/internal/scrape"
)

// releasesURL is a var (not a const) so tests can point it at an httptest server.
var releasesURL = "https://download.fedoraproject.org/pub/fedora/linux/releases/"

var releaseDirRe = regexp.MustCompile(`^(\d+)/?$`)

type Fedora struct{}

func init() {
	provider.Register(Fedora{})
}

func (Fedora) ID() string                  { return "fedora" }
func (Fedora) Name() string                { return "Fedora" }
func (Fedora) Category() provider.Category { return provider.CategoryLinux }

func (Fedora) Variants() []provider.Variant {
	return []provider.Variant{
		{Edition: "Workstation", Arch: "x86_64"},
		{Edition: "Workstation", Arch: "aarch64"},
		{Edition: "Server", Arch: "x86_64"},
		{Edition: "Server", Arch: "aarch64"},
	}
}

// isoFilenameRe matches the ISO filename for edition/arch. Workstation and
// Server use entirely different shapes (see package doc).
func isoFilenameRe(edition, arch string) *regexp.Regexp {
	if edition == "Server" {
		return regexp.MustCompile(fmt.Sprintf(`^Fedora-Server-dvd-%s-(\d+)-[\d.]+\.iso$`, regexp.QuoteMeta(arch)))
	}
	return regexp.MustCompile(fmt.Sprintf(`^Fedora-%s-Live-(\d+)-[\d.]+\.%s\.iso$`, edition, regexp.QuoteMeta(arch)))
}

func (Fedora) Check(ctx context.Context, _ provider.Variant) (provider.CheckResult, error) {
	body, err := scrape.FetchString(ctx, releasesURL)
	if err != nil {
		return provider.CheckResult{}, err
	}

	best := -1
	for _, href := range scrape.Hrefs(body) {
		m := releaseDirRe.FindStringSubmatch(href)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if n > best {
			best = n
		}
	}
	if best < 0 {
		return provider.CheckResult{}, fmt.Errorf("fedora: no release directory found at %s", releasesURL)
	}
	return provider.CheckResult{Version: strconv.Itoa(best)}, nil
}

func isoDirURL(version, edition, arch string) string {
	return fmt.Sprintf("%s%s/%s/%s/iso/", releasesURL, version, edition, arch)
}

func (Fedora) LocalVersion(filename string, v provider.Variant) (string, bool) {
	// Filenames embed a respin number not exposed by Check (e.g.
	// "Fedora-Workstation-Live-44-1.7.x86_64.iso" for release "44"), so
	// only the leading release number is captured here.
	return scrape.MatchVersion(isoFilenameRe(v.Edition, v.Arch), filename)
}

// findFileAndChecksum re-lists the ISO directory for edition/arch/version to
// find the exact respun filename (see package doc) and its matching
// checksum from the accompanying CHECKSUM file. Shared by Checksum and
// Download so both always agree on exactly which file they mean.
func findFileAndChecksum(ctx context.Context, edition, arch, version string) (filename string, checksum *download.Checksum, err error) {
	dirURL := isoDirURL(version, edition, arch)
	body, err := scrape.FetchString(ctx, dirURL)
	if err != nil {
		return "", nil, err
	}
	hrefs := scrape.Hrefs(body)

	isoRe := isoFilenameRe(edition, arch)
	for _, href := range hrefs {
		if isoRe.MatchString(href) {
			filename = href
			break
		}
	}
	if filename == "" {
		return "", nil, fmt.Errorf("fedora: no ISO found at %s", dirURL)
	}

	checksumFilename := ""
	for _, href := range hrefs {
		if strings.Contains(href, "CHECKSUM") {
			checksumFilename = href
			break
		}
	}
	if checksumFilename == "" {
		return "", nil, fmt.Errorf("fedora: no CHECKSUM file found at %s", dirURL)
	}

	sums, err := scrape.FetchString(ctx, dirURL+checksumFilename)
	if err != nil {
		return "", nil, err
	}
	hex, err := scrape.ChecksumForFileBSD(sums, filename)
	if err != nil {
		return "", nil, err
	}
	return filename, &download.Checksum{Algo: download.SHA256, Hex: hex}, nil
}

func (Fedora) Checksum(ctx context.Context, v provider.Variant, version string) (*download.Checksum, error) {
	_, checksum, err := findFileAndChecksum(ctx, v.Edition, v.Arch, version)
	return checksum, err
}

func (Fedora) Download(ctx context.Context, v provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	dirURL := isoDirURL(res.Version, v.Edition, v.Arch)
	filename, checksum, err := findFileAndChecksum(ctx, v.Edition, v.Arch, res.Version)
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
