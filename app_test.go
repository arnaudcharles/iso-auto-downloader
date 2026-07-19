package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/arnaudcharles/iso-auto-downloader/internal/config"
	"github.com/arnaudcharles/iso-auto-downloader/internal/download"
	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
)

// TestGoDownloadLimitsConcurrency is a regression test for a real report:
// clicking "Download All" fired one goroutine per visible variant
// simultaneously (40+ at once across every provider), which was enough to
// overwhelm the local DNS resolver and network stack well before any single
// mirror was actually at fault. goDownload must cap how many run at once.
func TestGoDownloadLimitsConcurrency(t *testing.T) {
	a := NewApp()
	maxSeen := runConcurrencyProbe(t, maxConcurrentDownloads*3, a.goDownload)

	if maxSeen != maxConcurrentDownloads {
		t.Errorf("max concurrent downloads = %d, want exactly %d", maxSeen, maxConcurrentDownloads)
	}
}

// TestGoCheckLimitsConcurrency is goCheck's equivalent of the above — same
// unbounded-fan-out problem applies to Check All, just with cheaper,
// shorter-lived requests, hence the higher (but still capped) limit.
func TestGoCheckLimitsConcurrency(t *testing.T) {
	a := NewApp()
	maxSeen := runConcurrencyProbe(t, maxConcurrentChecks*3, a.goCheck)

	if maxSeen != maxConcurrentChecks {
		t.Errorf("max concurrent checks = %d, want exactly %d", maxSeen, maxConcurrentChecks)
	}
}

// TestDownloadOneShowsDownloadingImmediatelyEvenWhenQueued is a regression
// test for a real report: after adding the concurrency cap above, Download
// buttons "seemed buggy" — clicking one did nothing visible. The real cause:
// downloadOne only flipped Status to "downloading" once it actually started
// running, so a click that had to queue behind maxConcurrentDownloads other
// downloads left the button showing its old, clickable state for however
// long it sat in the queue. DownloadOne must mark the variant busy
// synchronously, before goDownload ever queues the real work.
func TestDownloadOneShowsDownloadingImmediatelyEvenWhenQueued(t *testing.T) {
	a := NewApp()

	// Fill every download slot with a goroutine that blocks forever, so the
	// real download below has no choice but to sit queued behind them.
	var started sync.WaitGroup
	started.Add(maxConcurrentDownloads)
	for i := 0; i < maxConcurrentDownloads; i++ {
		a.goDownload(func() {
			started.Done()
			select {}
		})
	}
	started.Wait()

	p, ok := provider.Get("ubuntu")
	if !ok {
		t.Fatal(`provider "ubuntu" not registered`)
	}
	a.setStatus(VariantStatus{ProviderID: p.ID(), VariantIndex: 0, Status: StatusNotFound, LatestVersion: "26.04"})

	if err := a.DownloadOne(p.ID(), 0); err != nil {
		t.Fatalf("DownloadOne() error = %v", err)
	}

	got := a.statusFor(p.ID(), 0, p.Variants()[0])
	if got.Status != StatusDownloading {
		t.Errorf("Status immediately after DownloadOne() = %q, want %q (queued work must still show as busy)", got.Status, StatusDownloading)
	}
}

// runConcurrencyProbe fires n jobs through spawn (goCheck or goDownload) and
// returns the highest number that were ever actually running at once.
func runConcurrencyProbe(t *testing.T, n int, spawn func(func())) int {
	t.Helper()

	var mu sync.Mutex
	var current, maxSeen int
	var wg sync.WaitGroup

	wg.Add(n)
	for i := 0; i < n; i++ {
		spawn(func() {
			defer wg.Done()
			mu.Lock()
			current++
			if current > maxSeen {
				maxSeen = current
			}
			mu.Unlock()

			time.Sleep(20 * time.Millisecond)

			mu.Lock()
			current--
			mu.Unlock()
		})
	}
	wg.Wait()

	return maxSeen
}

// TestScanForFoundVersionsFindsFilesRegardlessOfNesting mirrors a real
// report: a user's external drive has "_ISO/Ubuntu/<file>.iso" and expects
// it to be detected regardless of how deep it's nested under the
// destination root.
func TestScanForFoundVersionsFindsFilesRegardlessOfNesting(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "_ISO", "Ubuntu")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	isoPath := filepath.Join(nested, "ubuntu-24.04-desktop-amd64.iso")
	if err := os.WriteFile(isoPath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	versions := scanForFoundVersions(root)

	ubuntu, ok := versions["ubuntu"]
	if !ok {
		t.Fatalf("no versions found for ubuntu; got %v", versions)
	}
	if ubuntu[0].Version != "24.04" {
		t.Errorf("ubuntu desktop/amd64 (variant 0) version = %q, want \"24.04\"", ubuntu[0].Version)
	}
	if ubuntu[0].Dir != nested {
		t.Errorf("ubuntu desktop/amd64 (variant 0) dir = %q, want %q", ubuntu[0].Dir, nested)
	}
}

func TestScanForFoundVersionsEmptyRoot(t *testing.T) {
	if got := scanForFoundVersions(""); len(got) != 0 {
		t.Errorf("scanForFoundVersions(\"\") = %v, want empty", got)
	}
}

func TestScanForFoundVersionsNoMatches(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "readme.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := scanForFoundVersions(root); len(got) != 0 {
		t.Errorf("scanForFoundVersions() = %v, want empty", got)
	}
}

// fakeCheckProvider is a minimal Provider whose Check() returns a fixed
// version, used to drive checkOne directly without hitting the network.
// checksum/checksumErr let tests control what Checksum() reports (a real
// hash, nil for "unsupported", or an error) without a real provider.
type fakeCheckProvider struct {
	version     string
	checksum    *download.Checksum
	checksumErr error
}

func (fakeCheckProvider) ID() string                  { return "fake" }
func (fakeCheckProvider) Name() string                { return "Fake" }
func (fakeCheckProvider) Category() provider.Category { return provider.CategoryLinux }
func (fakeCheckProvider) Variants() []provider.Variant {
	return []provider.Variant{{}}
}
func (f fakeCheckProvider) Check(context.Context, provider.Variant) (provider.CheckResult, error) {
	return provider.CheckResult{Version: f.version}, nil
}
func (fakeCheckProvider) Download(context.Context, provider.Variant, provider.CheckResult, string, provider.ProgressFunc) (provider.DownloadResult, error) {
	return provider.DownloadResult{}, nil
}
func (f fakeCheckProvider) Checksum(context.Context, provider.Variant, string) (*download.Checksum, error) {
	return f.checksum, f.checksumErr
}
func (fakeCheckProvider) LocalVersion(string, provider.Variant) (string, bool) {
	return "", false
}

// fakeDownloadProvider wraps fakeCheckProvider with a Download that reports
// which destDir it was actually called with.
type fakeDownloadProvider struct {
	fakeCheckProvider
	onDownload func(destDir string)
}

func (f fakeDownloadProvider) Download(_ context.Context, _ provider.Variant, _ provider.CheckResult, destDir string, _ provider.ProgressFunc) (provider.DownloadResult, error) {
	if f.onDownload != nil {
		f.onDownload(destDir)
	}
	return provider.DownloadResult{}, nil
}

// TestCheckOnePreservesFoundVersion is a regression test for a real bug: the
// first status update inside checkOne (marking the variant "checking") wrote
// a brand-new VariantStatus with a blank FoundVersion, wiping out whatever
// scanFound() had just set. checkOne then read FoundVersion back from that
// same clobbered state, so it always saw "" — every provider reported
// "Not found" on Check even when scanFound had correctly detected the file
// moments earlier. Real report: XCP-ng showed "Not found" despite
// xcp-ng-8.3.0.iso being present on disk.
func TestCheckOnePreservesFoundVersion(t *testing.T) {
	a := NewApp()
	p := fakeCheckProvider{version: "2.0"}
	v := provider.Variant{}

	a.setFoundVersion(p.ID(), 0, v, "1.0")

	a.checkOne(p, 0, v)

	got := a.statusFor(p.ID(), 0, v)
	if got.FoundVersion != "1.0" {
		t.Errorf("FoundVersion = %q, want %q (must survive the StatusChecking write)", got.FoundVersion, "1.0")
	}
	if got.Status != StatusOutdated {
		t.Errorf("Status = %q, want %q", got.Status, StatusOutdated)
	}
}

// fakeProgressProvider hands its progress callback to onProgress instead of
// calling it directly, so a test can invoke it and inspect app state
// in-flight, before Download returns and downloadOne overwrites the status
// with its final result.
type fakeProgressProvider struct {
	fakeCheckProvider
	onProgress func(provider.ProgressFunc)
}

func (f fakeProgressProvider) Download(_ context.Context, _ provider.Variant, _ provider.CheckResult, _ string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	if f.onProgress != nil {
		f.onProgress(progress)
	}
	return provider.DownloadResult{}, nil
}

// TestDownloadOnePreservesFoundVersionDuringProgress is a regression test
// for a real report: a user watched the app re-download an already-current
// 8GB Windows 11 ISO while showing "Downloading… / Not found" the entire
// time, even though the correct version was already sitting on disk. One
// cause (the other was a CompareVersions bug covered in the scrape
// package): downloadOne's progress callback built a brand-new VariantStatus
// from scratch on every tick without carrying FoundVersion forward, so it
// silently blanked to "" the instant the first progress update landed —
// for the whole rest of the download, regardless of what was known before
// it started.
func TestDownloadOnePreservesFoundVersionDuringProgress(t *testing.T) {
	a := NewApp()
	v := provider.Variant{}
	var midDownloadFoundVersion string
	p := fakeProgressProvider{
		fakeCheckProvider: fakeCheckProvider{version: "25H2"},
		onProgress: func(progress provider.ProgressFunc) {
			progress(50, 100)
			midDownloadFoundVersion = a.statusFor("fake", 0, v).FoundVersion
		},
	}

	a.setFoundVersion(p.ID(), 0, v, "25H2")

	a.downloadOne(p, 0, v, provider.CheckResult{Version: "25H2"})

	if midDownloadFoundVersion != "25H2" {
		t.Errorf("FoundVersion mid-download = %q, want %q (must survive progress updates)", midDownloadFoundVersion, "25H2")
	}
}

// TestDownloadOneUsesFoundDirectoryWhenKnown covers a real user question:
// when a provider already has a file found on disk (possibly in a
// pre-existing, manually organized folder, not this app's own per-provider
// layout), a newer download should land next to it instead of scattering
// a second copy into <dest>/<Provider>/.
func TestDownloadOneUsesFoundDirectoryWhenKnown(t *testing.T) {
	existingDir := filepath.Join(t.TempDir(), "Clonezilla")

	var gotDestDir string
	p := fakeDownloadProvider{
		fakeCheckProvider: fakeCheckProvider{version: "2.0"},
		onDownload:        func(destDir string) { gotDestDir = destDir },
	}

	a := NewApp()
	a.foundDir = map[string]map[int]string{p.ID(): {0: existingDir}}

	a.downloadOne(p, 0, provider.Variant{}, provider.CheckResult{Version: "2.0"})

	if gotDestDir != existingDir {
		t.Errorf("destDir = %q, want %q (the directory the existing file was found in)", gotDestDir, existingDir)
	}
}

// TestDownloadOneFallsBackToProviderFolderWhenNothingFound covers the
// other half: a provider never seen on disk still gets the app's own
// default per-provider layout, one folder per OS directly under the
// destination — not grouped under a Category subfolder.
func TestDownloadOneFallsBackToProviderFolderWhenNothingFound(t *testing.T) {
	var gotDestDir string
	p := fakeDownloadProvider{
		fakeCheckProvider: fakeCheckProvider{version: "2.0"},
		onDownload:        func(destDir string) { gotDestDir = destDir },
	}

	a := NewApp()
	a.cfg.General.DownloadDir = t.TempDir()

	a.downloadOne(p, 0, provider.Variant{}, provider.CheckResult{Version: "2.0"})

	want := filepath.Join(a.cfg.General.DownloadDir, p.Name())
	if gotDestDir != want {
		t.Errorf("destDir = %q, want %q", gotDestDir, want)
	}
}

// TestValidateOneSuccessKeepsFile confirms a file whose hash matches the
// provider's published checksum is left alone, with its found status
// restored (not left on the transient "validating" state).
func TestValidateOneSuccessKeepsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fake.iso")
	content := []byte("the real iso bytes")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := fmt.Sprintf("%x", sha256.Sum256(content))

	p := fakeCheckProvider{version: "1.0", checksum: &download.Checksum{Algo: download.SHA256, Hex: sum}}
	a := NewApp()

	a.validateOne(p, 0, provider.Variant{}, "1.0", "1.0", path)

	got := a.statusFor(p.ID(), 0, provider.Variant{})
	if got.Status != StatusUpToDate {
		t.Errorf("Status = %q, want %q", got.Status, StatusUpToDate)
	}
	if got.FoundVersion != "1.0" {
		t.Errorf("FoundVersion = %q, want %q (kept)", got.FoundVersion, "1.0")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file should still exist after a successful validation: %v", err)
	}
}

// TestValidateOneSuccessWithPlaceholderVersionShowsUpToDate is a regression
// test for a real report: hirensbootcdpe (whose filename never embeds a
// real version) reports a fixed placeholder as its found version so the app
// can at least recognize the file exists — but that placeholder string
// never equals a real "latest" version, and a naive numeric comparison
// treats every non-numeric segment as 0, so it read as permanently
// "outdated" even immediately after a successful checksum validation. A
// validated file with an unparseable found-version placeholder should read
// as up to date, not stay stuck on "outdated" forever.
func TestValidateOneSuccessWithPlaceholderVersionShowsUpToDate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fake.iso")
	content := []byte("the real iso bytes")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := fmt.Sprintf("%x", sha256.Sum256(content))

	p := fakeCheckProvider{version: "8.10", checksum: &download.Checksum{Algo: download.SHA256, Hex: sum}}
	a := NewApp()

	a.validateOne(p, 0, provider.Variant{}, "8.10", "unknown", path)

	got := a.statusFor(p.ID(), 0, provider.Variant{})
	if got.Status != StatusUpToDate {
		t.Errorf("Status = %q, want %q (an unparseable found-version placeholder shouldn't read as older)", got.Status, StatusUpToDate)
	}
}

// TestValidateOneSuccessWithGenuinelyOlderVersionStaysOutdated confirms the
// fix above didn't remove the version check entirely: a real, older found
// version must still show as outdated after a successful validation — the
// checksum only confirms that specific (stale) version isn't corrupted, not
// that it's current.
func TestValidateOneSuccessWithGenuinelyOlderVersionStaysOutdated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fake.iso")
	content := []byte("the real iso bytes")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := fmt.Sprintf("%x", sha256.Sum256(content))

	p := fakeCheckProvider{version: "26.04", checksum: &download.Checksum{Algo: download.SHA256, Hex: sum}}
	a := NewApp()

	a.validateOne(p, 0, provider.Variant{}, "26.04", "24.04", path)

	got := a.statusFor(p.ID(), 0, provider.Variant{})
	if got.Status != StatusOutdated {
		t.Errorf("Status = %q, want %q (a genuinely older version stays outdated even once validated)", got.Status, StatusOutdated)
	}
}

// TestValidateOneMismatchDeletesFileAndReEnablesDownload is a regression
// test for the actual feature request: a corrupted local file should have
// its Download button re-enabled, which the UI keys off Status being
// "outdated" or "not-found" — and scanFound would just re-detect a
// still-present corrupted file on the next Rescan and put FoundVersion
// right back, so the file itself has to go, not just the in-memory status.
func TestValidateOneMismatchDeletesFileAndReEnablesDownload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fake.iso")
	if err := os.WriteFile(path, []byte("corrupted bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	wantSum := fmt.Sprintf("%x", sha256.Sum256([]byte("the real iso bytes")))

	p := fakeCheckProvider{version: "1.0", checksum: &download.Checksum{Algo: download.SHA256, Hex: wantSum}}
	a := NewApp()

	a.validateOne(p, 0, provider.Variant{}, "1.0", "1.0", path)

	got := a.statusFor(p.ID(), 0, provider.Variant{})
	if got.Status != StatusNotFound {
		t.Errorf("Status = %q, want %q (so Download re-enables)", got.Status, StatusNotFound)
	}
	if got.FoundVersion != "" {
		t.Errorf("FoundVersion = %q, want cleared", got.FoundVersion)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("corrupted file should have been deleted, stat error = %v", err)
	}
}

// TestValidateOneNoChecksumPublishedLeavesFileAlone confirms a source with
// no published checksum (Checksum returns nil, nil) reports an informative,
// non-alarming status instead of either deleting the file, silently
// claiming success, or looking like a genuine error (nothing actually went
// wrong — this source just doesn't publish one).
func TestValidateOneNoChecksumPublishedLeavesFileAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fake.iso")
	if err := os.WriteFile(path, []byte("bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := fakeCheckProvider{version: "1.0", checksum: nil}
	a := NewApp()

	a.validateOne(p, 0, provider.Variant{}, "1.0", "1.0", path)

	got := a.statusFor(p.ID(), 0, provider.Variant{})
	if got.Status != StatusNotTested {
		t.Errorf("Status = %q, want %q", got.Status, StatusNotTested)
	}
	if got.Error == "" {
		t.Error("Error message is empty, want an explanation that no checksum is published")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file should still exist: %v", err)
	}
}

// TestValidateOneErrorsWhenNothingFound confirms the exported ValidateOne
// refuses to run when scanFound hasn't found anything on disk for this
// variant yet, without making any network call (a real registered
// provider's Checksum is never reached since this returns early).
func TestValidateOneErrorsWhenNothingFound(t *testing.T) {
	a := NewApp()
	if err := a.ValidateOne("ubuntu", 0); err == nil {
		t.Error("ValidateOne() with nothing found on disk = nil error, want an error")
	}
}

// TestScanFoundPopulatesFoundDir confirms scanFound() (the real, disk-based
// path, not a hand-constructed a.foundDir) records the directory of a
// matched file, mirroring the nesting scenario from
// TestScanForFoundVersionsFindsFilesRegardlessOfNesting.
func TestScanFoundPopulatesFoundDir(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "_ISO", "Ubuntu")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	isoPath := filepath.Join(nested, "ubuntu-24.04-desktop-amd64.iso")
	if err := os.WriteFile(isoPath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := NewApp()
	a.cfg.General.DownloadDir = root

	a.scanFound()

	dir, ok := a.foundDirFor("ubuntu", 0)
	if !ok {
		t.Fatal("foundDirFor(\"ubuntu\", 0) not found")
	}
	if dir != nested {
		t.Errorf("foundDirFor(\"ubuntu\", 0) = %q, want %q", dir, nested)
	}
}

func TestSetDisabledAddsAndRemoves(t *testing.T) {
	got := setDisabled(nil, 2, true)
	if len(got) != 1 || got[0] != 2 {
		t.Errorf("setDisabled(nil, 2, true) = %v, want [2]", got)
	}

	// Adding an already-disabled index must not duplicate it.
	got = setDisabled(got, 2, true)
	if len(got) != 1 {
		t.Errorf("setDisabled with an already-present index = %v, want no duplicate", got)
	}

	got = setDisabled([]int{1, 2, 3}, 2, false)
	if len(got) != 2 || containsInt(got, 2) {
		t.Errorf("setDisabled([1,2,3], 2, false) = %v, want [1,3]", got)
	}

	// Removing an index that was never disabled is a no-op.
	got = setDisabled([]int{1, 3}, 2, false)
	if len(got) != 2 {
		t.Errorf("setDisabled removing an absent index = %v, want unchanged [1,3]", got)
	}
}

// TestVisibleVariantIndexesRespectsDisabledVariants covers the new
// per-variant checkbox model directly (not through SetVariantEnabled, which
// persists to the real config file — not something a unit test should do).
func TestVisibleVariantIndexesRespectsDisabledVariants(t *testing.T) {
	ubuntu, ok := provider.Get("ubuntu")
	if !ok {
		t.Fatal("ubuntu not registered")
	}

	a := NewApp()
	a.cfg.ISO = map[string]config.ISOConfig{
		"ubuntu": {DisabledVariants: []int{1}},
	}

	idxs := a.visibleVariantIndexes(ubuntu)
	if len(idxs) != 1 || idxs[0] != 0 {
		t.Errorf("visibleVariantIndexes = %v, want [0] (variant 1 unchecked)", idxs)
	}
}

// TestListProvidersOmitsProviderWithNoVisibleVariants: unchecking every
// variant of a provider should hide it entirely, not show an empty section.
func TestListProvidersOmitsProviderWithNoVisibleVariants(t *testing.T) {
	ubuntu, ok := provider.Get("ubuntu")
	if !ok {
		t.Fatal("ubuntu not registered")
	}

	a := NewApp()
	allIdxs := make([]int, len(ubuntu.Variants()))
	for i := range allIdxs {
		allIdxs[i] = i
	}
	a.cfg.ISO = map[string]config.ISOConfig{
		"ubuntu": {DisabledVariants: allIdxs},
	}

	for _, p := range a.ListProviders() {
		if p.ID == "ubuntu" {
			t.Fatal("ListProviders() included ubuntu even though every variant is unchecked")
		}
	}
}

// TestListProviderVariantsReflectsEnabledState confirms the settings
// panel's flat variant list reports each variant's actual checked state.
func TestListProviderVariantsReflectsEnabledState(t *testing.T) {
	a := NewApp()
	a.cfg.ISO = map[string]config.ISOConfig{
		"ubuntu": {DisabledVariants: []int{1}},
	}

	var sawEnabled, sawDisabled bool
	for _, vs := range a.ListProviderVariants() {
		if vs.ProviderID != "ubuntu" {
			continue
		}
		if vs.VariantIndex == 0 {
			sawEnabled = vs.Enabled
		}
		if vs.VariantIndex == 1 {
			sawDisabled = vs.Enabled
		}
	}
	if !sawEnabled {
		t.Error("ListProviderVariants() reported ubuntu variant 0 as disabled, want enabled")
	}
	if sawDisabled {
		t.Error("ListProviderVariants() reported ubuntu variant 1 as enabled, want disabled")
	}
}
