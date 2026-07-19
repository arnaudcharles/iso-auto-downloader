// Package rescuezilla checks and downloads Rescuezilla ISOs from GitHub
// Releases (unlike the other providers, which scrape HTML directory
// listings, this one hits the GitHub API). "rolling-release.*" tags are
// pre-releases and are skipped in favor of the newest proper version tag.
package rescuezilla

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/arnaudcharles/iso-auto-downloader/internal/download"
	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
	"github.com/arnaudcharles/iso-auto-downloader/internal/scrape"
)

// releasesAPIURL and downloadBaseURL are vars (not consts) so tests can
// point them at an httptest server.
var releasesAPIURL = "https://api.github.com/repos/rescuezilla/rescuezilla/releases?per_page=30"
var downloadBaseURL = "https://github.com/rescuezilla/rescuezilla/releases/download/"

type ghAsset struct {
	Name string `json:"name"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type Rescuezilla struct{}

func init() {
	provider.Register(Rescuezilla{})
}

func (Rescuezilla) ID() string                  { return "rescuezilla" }
func (Rescuezilla) Name() string                { return "Rescuezilla" }
func (Rescuezilla) Category() provider.Category { return provider.CategoryDiskUtility }

func (Rescuezilla) Variants() []provider.Variant {
	// "noble" (Ubuntu 24.04 LTS base) is the most conservative default of
	// the editions GitHub currently ships.
	return []provider.Variant{{Edition: "noble", Arch: "64bit"}}
}

func isoFilename(version string, v provider.Variant) string {
	return fmt.Sprintf("rescuezilla-%s-%s.%s.iso", version, v.Arch, v.Edition)
}

// latestRelease returns the newest non-"rolling-release" tag that has an
// ISO asset for v.
func latestRelease(ctx context.Context, v provider.Variant) (ghRelease, error) {
	body, err := scrape.FetchString(ctx, releasesAPIURL)
	if err != nil {
		return ghRelease{}, err
	}

	var releases []ghRelease
	if err := json.Unmarshal([]byte(body), &releases); err != nil {
		return ghRelease{}, fmt.Errorf("rescuezilla: parse releases API response: %w", err)
	}

	for _, r := range releases {
		if strings.Contains(r.TagName, "rolling") {
			continue
		}
		want := isoFilename(r.TagName, v)
		for _, a := range r.Assets {
			if a.Name == want {
				return r, nil
			}
		}
	}
	return ghRelease{}, fmt.Errorf("rescuezilla: no release with a %s/%s ISO found", v.Edition, v.Arch)
}

func (Rescuezilla) Check(ctx context.Context, v provider.Variant) (provider.CheckResult, error) {
	r, err := latestRelease(ctx, v)
	if err != nil {
		return provider.CheckResult{}, err
	}
	return provider.CheckResult{Version: r.TagName}, nil
}

func (Rescuezilla) Checksum(ctx context.Context, v provider.Variant, version string) (*download.Checksum, error) {
	filename := isoFilename(version, v)
	releaseDir := downloadBaseURL + version + "/"

	sums, err := scrape.FetchString(ctx, releaseDir+"SHA256SUM")
	if err != nil {
		return nil, err
	}
	checksum, err := scrape.ChecksumForFile(sums, filename)
	if err != nil {
		return nil, err
	}
	return &download.Checksum{Algo: download.SHA256, Hex: checksum}, nil
}

func (Rescuezilla) Download(ctx context.Context, v provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	filename := isoFilename(res.Version, v)
	releaseDir := downloadBaseURL + res.Version + "/"
	fileURL := releaseDir + filename

	checksum, err := (Rescuezilla{}).Checksum(ctx, v, res.Version)
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

func (Rescuezilla) LocalVersion(filename string, v provider.Variant) (string, bool) {
	suffix := fmt.Sprintf("-%s.%s.iso", v.Arch, v.Edition)
	if !strings.HasPrefix(filename, "rescuezilla-") || !strings.HasSuffix(filename, suffix) {
		return "", false
	}
	version := strings.TrimSuffix(strings.TrimPrefix(filename, "rescuezilla-"), suffix)
	if version == "" {
		return "", false
	}
	return version, true
}
