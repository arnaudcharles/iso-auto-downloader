// Package tails checks and downloads the Tails amd64 stable ISO from
// archive.torproject.org's mirror (Tails' own download.tails.net stable
// alias redirects here). No plain checksum sidecar is published there —
// only a PGP .sig — so downloads aren't checksum-verified, same as a few
// other providers in this app.
package tails

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
var baseURL = "https://archive.torproject.org/amnesia.boum.org/tails/stable/"

var versionDirRe = regexp.MustCompile(`^tails-amd64-([\d.]+)/$`)

type Tails struct{}

func init() {
	provider.Register(Tails{})
}

func (Tails) ID() string                  { return "tails" }
func (Tails) Name() string                { return "Tails" }
func (Tails) Category() provider.Category { return provider.CategoryLinux }

func (Tails) Variants() []provider.Variant {
	return []provider.Variant{{Arch: "amd64"}}
}

func (Tails) Check(ctx context.Context, _ provider.Variant) (provider.CheckResult, error) {
	body, err := scrape.FetchString(ctx, baseURL)
	if err != nil {
		return provider.CheckResult{}, err
	}
	best := ""
	for _, href := range scrape.Hrefs(body) {
		m := versionDirRe.FindStringSubmatch(href)
		if m == nil {
			continue
		}
		if best == "" || scrape.CompareVersions(m[1], best) > 0 {
			best = m[1]
		}
	}
	if best == "" {
		return provider.CheckResult{}, fmt.Errorf("tails: no stable version directory found at %s", baseURL)
	}
	return provider.CheckResult{Version: best}, nil
}

func filename(version string) string {
	return fmt.Sprintf("tails-amd64-%s.iso", version)
}

func (Tails) LocalVersion(fname string, _ provider.Variant) (string, bool) {
	re := regexp.MustCompile(`^tails-amd64-([\d.]+)\.iso$`)
	return scrape.MatchVersion(re, fname)
}

func (Tails) Download(ctx context.Context, _ provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	fname := filename(res.Version)
	fileURL := fmt.Sprintf("%stails-amd64-%s/%s", baseURL, res.Version, fname)

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

// Checksum: only a PGP .sig is published, no plain checksum sidecar (see
// the package doc comment), so there's nothing to verify against.
func (Tails) Checksum(context.Context, provider.Variant, string) (*download.Checksum, error) {
	return nil, nil
}
