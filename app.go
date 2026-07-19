package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/arnaudcharles/iso-auto-downloader/internal/config"
	"github.com/arnaudcharles/iso-auto-downloader/internal/download"
	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
	"github.com/arnaudcharles/iso-auto-downloader/internal/scrape"

	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/alpinelinux"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/clonezilla"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/debian"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/fedora"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/hirensbootcdpe"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/kalilinux"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/linuxmint"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/manjaro"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/medicat"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/memtest86plus"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/opensuse"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/parrotos"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/proxmox"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/rescuezilla"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/rockylinux"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/systemrescue"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/tails"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/truenas"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/ubuntu"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/ultimatebootcd"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/windows11"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/windowsserver"
	_ "github.com/arnaudcharles/iso-auto-downloader/internal/provider/xcpng"
)

// Status is the lifecycle state of one provider variant, as shown by the badge in the UI.
type Status string

const (
	StatusUnknown     Status = "unknown"
	StatusChecking    Status = "checking"
	StatusUpToDate    Status = "up-to-date"
	StatusOutdated    Status = "outdated"
	StatusNotFound    Status = "not-found"
	StatusDownloading Status = "downloading"
	StatusValidating  Status = "validating"
	StatusNotTested   Status = "not-tested"
	StatusError       Status = "error"
)

// VariantStatus is what the frontend renders per ISO variant, and what's
// pushed over the "provider:status" event as checks/downloads progress.
type VariantStatus struct {
	ProviderID    string  `json:"providerId"`
	VariantIndex  int     `json:"variantIndex"`
	Label         string  `json:"label"`
	LatestVersion string  `json:"latestVersion"`
	FoundVersion  string  `json:"foundVersion"`
	Status        Status  `json:"status"`
	Error         string  `json:"error,omitempty"`
	Progress      float64 `json:"progress"`
}

// ProviderInfo is a provider and the current status of each of its variants.
type ProviderInfo struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Category string          `json:"category"`
	Variants []VariantStatus `json:"variants"`
}

// VariantSetting is one row in the settings panel's ISO list — every
// registered provider's every variant, regardless of enabled state.
type VariantSetting struct {
	ProviderID   string `json:"providerId"`
	ProviderName string `json:"providerName"`
	Category     string `json:"category"`
	VariantIndex int    `json:"variantIndex"`
	Label        string `json:"label"`
	Enabled      bool   `json:"enabled"`
}

// App is the Wails-bound backend.
// maxConcurrentChecks and maxConcurrentDownloads cap how many variants run
// at once from CheckAll/DownloadAll (and stack with individual Check/
// Download clicks fired in quick succession, since they share the same
// semaphores). Real report: clicking Download All fired one goroutine per
// visible variant simultaneously — 40+ at once across every provider —
// which was enough to overwhelm the local DNS resolver (sporadic "no such
// host" on otherwise-fine domains) and the local network stack (TCP resets,
// corrupted transfers) well before any single mirror was actually at fault.
// Downloads get a tighter cap since they hold a connection open far longer
// and consume real bandwidth; checks are cheap, short-lived HTTP requests.
const (
	maxConcurrentChecks      = 6
	maxConcurrentDownloads   = 3
	maxConcurrentValidations = 3
)

type App struct {
	ctx context.Context
	cfg config.Config

	mu        sync.Mutex
	state     map[string]map[int]*VariantStatus
	foundDir  map[string]map[int]string
	foundPath map[string]map[int]string

	checkSem    chan struct{}
	downloadSem chan struct{}
	validateSem chan struct{}
}

func NewApp() *App {
	return &App{
		state:       make(map[string]map[int]*VariantStatus),
		foundDir:    make(map[string]map[int]string),
		foundPath:   make(map[string]map[int]string),
		checkSem:    make(chan struct{}, maxConcurrentChecks),
		downloadSem: make(chan struct{}, maxConcurrentDownloads),
		validateSem: make(chan struct{}, maxConcurrentValidations),
	}
}

// goCheck, goDownload, and goValidate run fn in a new goroutine, gated by
// the relevant semaphore so at most maxConcurrentChecks/
// maxConcurrentDownloads/maxConcurrentValidations run at once — the acquire
// happens inside the spawned goroutine, so the caller (an All loop, or a
// single click) never blocks; excess work just queues until a slot frees up.
func (a *App) goCheck(fn func()) {
	go func() {
		a.checkSem <- struct{}{}
		defer func() { <-a.checkSem }()
		fn()
	}()
}

func (a *App) goDownload(fn func()) {
	go func() {
		a.downloadSem <- struct{}{}
		defer func() { <-a.downloadSem }()
		fn()
	}()
}

func (a *App) goValidate(fn func()) {
	go func() {
		a.validateSem <- struct{}{}
		defer func() { <-a.validateSem }()
		fn()
	}()
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	a.cfg = config.Default()
	if path, err := config.DefaultPath(); err == nil {
		if cfg, err := config.Load(path); err == nil {
			a.cfg = cfg
		}
	}

	// Any provider not yet in the config (new install, or newly ported
	// since the config was last saved) defaults to enabled/visible.
	added := false
	if a.cfg.ISO == nil {
		a.cfg.ISO = map[string]config.ISOConfig{}
	}
	for _, p := range provider.All() {
		if _, ok := a.cfg.ISO[p.ID()]; !ok {
			a.cfg.ISO[p.ID()] = defaultISOConfigFor(p)
			added = true
		}
	}
	if added {
		if path, err := config.DefaultPath(); err == nil {
			_ = config.Save(path, a.cfg)
		}
	}

	a.scanFound()
}

// foundFile is a matched on-disk file's version, the directory it was found
// in, and its exact filename (so a later Validate knows exactly which file
// to hash without having to re-derive the name from the provider).
type foundFile struct {
	Version  string
	Dir      string
	Filename string
}

// scanForFoundVersions recursively walks root and returns, for every
// registered provider variant that has a matching file anywhere under it,
// the highest version found and which directory it's in. It deliberately
// doesn't care which exact subfolder a file is in — not just the
// per-provider path this app itself downloads into — so pre-existing
// or manually-organized ISOs are detected too. Pure (no Wails dependency),
// so it's directly testable.
func scanForFoundVersions(root string) map[string]map[int]foundFile {
	result := map[string]map[int]foundFile{}
	if root == "" {
		return result
	}

	type entry struct{ name, dir string }
	var files []entry
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		files = append(files, entry{name: d.Name(), dir: filepath.Dir(path)})
		return nil
	})

	for _, p := range provider.All() {
		for i, v := range p.Variants() {
			var best foundFile
			for _, f := range files {
				if ver, ok := p.LocalVersion(f.name, v); ok {
					if best.Version == "" || scrape.CompareVersions(ver, best.Version) > 0 {
						best = foundFile{Version: ver, Dir: f.dir, Filename: f.name}
					}
				}
			}
			if best.Version != "" {
				if result[p.ID()] == nil {
					result[p.ID()] = map[int]foundFile{}
				}
				result[p.ID()][i] = best
			}
		}
	}
	return result
}

// scanFound updates every provider variant's FoundVersion from disk, and
// remembers which directory each found file lives in so a future download
// of a newer version can land next to it instead of the default
// per-provider folder (see downloadOne). Idempotent: variants with no
// matching file get FoundVersion (and remembered directory) cleared, so
// switching to an empty/different folder is reflected correctly.
func (a *App) scanFound() {
	found := scanForFoundVersions(a.cfg.General.DownloadDir)

	dirs := make(map[string]map[int]string, len(found))
	paths := make(map[string]map[int]string, len(found))
	for pid, byIdx := range found {
		dirs[pid] = make(map[int]string, len(byIdx))
		paths[pid] = make(map[int]string, len(byIdx))
		for i, f := range byIdx {
			dirs[pid][i] = f.Dir
			paths[pid][i] = filepath.Join(f.Dir, f.Filename)
		}
	}
	a.mu.Lock()
	a.foundDir = dirs
	a.foundPath = paths
	a.mu.Unlock()

	for _, p := range provider.All() {
		for i, v := range p.Variants() {
			a.setFoundVersion(p.ID(), i, v, found[p.ID()][i].Version)
		}
	}
}

// foundDirFor returns the directory a variant's on-disk file was last found
// in, if any.
func (a *App) foundDirFor(providerID string, variantIndex int) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	dir, ok := a.foundDir[providerID][variantIndex]
	return dir, ok
}

// foundPathFor returns the full path of a variant's on-disk file as last
// found, if any — used by Validate to know exactly which file to hash.
func (a *App) foundPathFor(providerID string, variantIndex int) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	path, ok := a.foundPath[providerID][variantIndex]
	return path, ok
}

// RescanFound re-scans the download folder on demand (e.g. after the user
// adds files outside the app, or wants to double check).
func (a *App) RescanFound() {
	a.scanFound()
}

func (a *App) setFoundVersion(providerID string, variantIndex int, v provider.Variant, found string) {
	current := a.statusFor(providerID, variantIndex, v)
	next := current
	next.FoundVersion = found
	if current.LatestVersion != "" {
		switch found {
		case "":
			next.Status = StatusNotFound
		case current.LatestVersion:
			next.Status = StatusUpToDate
		default:
			next.Status = StatusOutdated
		}
	}
	if next == current {
		return
	}
	a.setStatus(next)
}

// DownloadDir returns the folder ISOs are saved to.
func (a *App) DownloadDir() string {
	return a.cfg.General.DownloadDir
}

// Theme returns the persisted theme preference: "light", "dark", or "auto".
func (a *App) Theme() string {
	if a.cfg.General.Theme == "" {
		return "auto"
	}
	return a.cfg.General.Theme
}

// SetTheme persists the user's theme choice.
func (a *App) SetTheme(theme string) error {
	a.cfg.General.Theme = theme
	path, err := config.DefaultPath()
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	if err := config.Save(path, a.cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// ChooseDownloadDir opens a native folder picker defaulting to the current
// download folder. On cancel it returns the unchanged current path. On a
// new selection it persists the choice and returns the new path.
func (a *App) ChooseDownloadDir() (string, error) {
	// runtime.OpenDirectoryDialog refuses to open at all if DefaultDirectory
	// doesn't exist yet (e.g. the default ~/Downloads/ISOs on first run) —
	// make sure it's there so the picker actually appears.
	if err := os.MkdirAll(a.cfg.General.DownloadDir, 0o755); err != nil {
		return "", fmt.Errorf("create default download dir: %w", err)
	}

	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "Choose the ISO destination folder",
		DefaultDirectory: a.cfg.General.DownloadDir,
	})
	if err != nil {
		return "", fmt.Errorf("choose download dir: %w", err)
	}
	if dir == "" {
		return a.cfg.General.DownloadDir, nil
	}

	a.cfg.General.DownloadDir = dir
	if path, err := config.DefaultPath(); err == nil {
		if err := config.Save(path, a.cfg); err != nil {
			return "", fmt.Errorf("save config: %w", err)
		}
	}
	a.scanFound()
	return dir, nil
}

// SetVariantEnabled shows/hides a single ISO variant on the main screen and
// persists the choice.
func (a *App) SetVariantEnabled(providerID string, variantIndex int, enabled bool) error {
	if _, _, err := a.lookup(providerID, variantIndex); err != nil {
		return err
	}

	iso := a.cfg.ISO[providerID]
	iso.DisabledVariants = setDisabled(iso.DisabledVariants, variantIndex, !enabled)
	a.cfg.ISO[providerID] = iso

	path, err := config.DefaultPath()
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	if err := config.Save(path, a.cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// SetAllVariantsEnabled checks or unchecks every ISO variant across every
// registered provider at once (the settings panel's "Check all"/"Uncheck
// all" button) and persists the choice.
func (a *App) SetAllVariantsEnabled(enabled bool) error {
	for _, p := range provider.All() {
		variants := p.Variants()
		iso := a.cfg.ISO[p.ID()]
		if enabled {
			iso.DisabledVariants = nil
		} else {
			all := make([]int, len(variants))
			for i := range variants {
				all[i] = i
			}
			iso.DisabledVariants = all
		}
		a.cfg.ISO[p.ID()] = iso
	}

	path, err := config.DefaultPath()
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	if err := config.Save(path, a.cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// setDisabled adds or removes variantIndex from disabled, without
// duplicating an entry that's already there.
func setDisabled(disabled []int, variantIndex int, wantDisabled bool) []int {
	pos := -1
	for i, d := range disabled {
		if d == variantIndex {
			pos = i
			break
		}
	}
	if wantDisabled {
		if pos >= 0 {
			return disabled
		}
		return append(disabled, variantIndex)
	}
	if pos < 0 {
		return disabled
	}
	return append(disabled[:pos], disabled[pos+1:]...)
}

// ListProviderVariants returns every registered provider's every variant
// (regardless of enabled state) for the settings panel's checkbox list.
func (a *App) ListProviderVariants() []VariantSetting {
	out := make([]VariantSetting, 0, len(provider.All()))
	for _, p := range provider.All() {
		variants := p.Variants()
		enabledIdx := a.visibleVariantIndexes(p)
		enabled := make(map[int]bool, len(enabledIdx))
		for _, i := range enabledIdx {
			enabled[i] = true
		}
		for i, v := range variants {
			out = append(out, VariantSetting{
				ProviderID:   p.ID(),
				ProviderName: p.Name(),
				Category:     p.Category().Label(),
				VariantIndex: i,
				Label:        v.String(),
				Enabled:      enabled[i],
			})
		}
	}
	return out
}

// ListProviders returns every provider that has at least one visible
// variant, with each visible variant's last-known status (StatusUnknown
// until a check has run). Variants the user has unchecked in Settings (see
// SetVariantEnabled) are omitted, and a provider with none left visible is
// omitted entirely.
func (a *App) ListProviders() []ProviderInfo {
	out := make([]ProviderInfo, 0, len(provider.All()))
	for _, p := range provider.All() {
		variants := p.Variants()
		idxs := a.visibleVariantIndexes(p)
		if len(idxs) == 0 {
			continue
		}
		vs := make([]VariantStatus, 0, len(idxs))
		for _, i := range idxs {
			vs = append(vs, a.statusFor(p.ID(), i, variants[i]))
		}
		out = append(out, ProviderInfo{
			ID:       p.ID(),
			Name:     p.Name(),
			Category: string(p.Category()),
			Variants: vs,
		})
	}
	return out
}

// visibleVariantIndexes returns the indexes into p.Variants() the user
// hasn't unchecked in Settings.
func (a *App) visibleVariantIndexes(p provider.Provider) []int {
	iso := a.cfg.ISO[p.ID()]
	variants := p.Variants()
	idxs := make([]int, 0, len(variants))
	for i := range variants {
		if !containsInt(iso.DisabledVariants, i) {
			idxs = append(idxs, i)
		}
	}
	return idxs
}

func containsInt(list []int, n int) bool {
	for _, item := range list {
		if item == n {
			return true
		}
	}
	return false
}

func (a *App) statusFor(providerID string, variantIndex int, v provider.Variant) VariantStatus {
	a.mu.Lock()
	defer a.mu.Unlock()

	if byIdx, ok := a.state[providerID]; ok {
		if s, ok := byIdx[variantIndex]; ok {
			return *s
		}
	}
	return VariantStatus{ProviderID: providerID, VariantIndex: variantIndex, Label: v.String(), Status: StatusUnknown}
}

func (a *App) setStatus(s VariantStatus) {
	a.mu.Lock()
	if a.state[s.ProviderID] == nil {
		a.state[s.ProviderID] = make(map[int]*VariantStatus)
	}
	a.state[s.ProviderID][s.VariantIndex] = &s
	a.mu.Unlock()

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "provider:status", s)
	}
}

// markBusy immediately (synchronously, before goCheck/goDownload/goValidate
// queue the real work behind their semaphore) flips a variant's status to
// the in-progress state the UI is about to show, and returns it. Without
// this, a Check/Download/Validate that has to wait its turn behind
// maxConcurrentChecks/Downloads/Validations leaves the previous status —
// and an enabled button — in place for however long it sits queued, which
// reads as the button silently doing nothing when clicked. Preserves every
// other field (FoundVersion in particular) so it's still safe to build the
// real work's status updates from statusFor() after this.
func (a *App) markBusy(providerID string, variantIndex int, v provider.Variant, status Status) VariantStatus {
	next := a.statusFor(providerID, variantIndex, v)
	next.Status = status
	a.setStatus(next)
	return next
}

// CheckAll kicks off a concurrent version check for every visible ISO
// variant. Results stream back via the "provider:status" event, not a
// return value, so the UI never blocks on a single slow mirror. The disk is
// re-scanned first (not just relying on the startup/Rescan snapshot) so a
// Found version discovered or changed since the app launched is never stale
// by the time a Check reports "not found" or "outdated".
func (a *App) CheckAll() {
	a.scanFound()
	for _, p := range provider.All() {
		variants := p.Variants()
		for _, i := range a.visibleVariantIndexes(p) {
			v := variants[i]
			a.markBusy(p.ID(), i, v, StatusChecking)
			a.goCheck(func() { a.checkOne(p, i, v) })
		}
	}
}

// CheckOne kicks off a version check for a single provider variant. The
// disk is re-scanned first, same reasoning as CheckAll.
func (a *App) CheckOne(providerID string, variantIndex int) error {
	p, v, err := a.lookup(providerID, variantIndex)
	if err != nil {
		return err
	}
	a.scanFound()
	a.markBusy(p.ID(), variantIndex, v, StatusChecking)
	a.goCheck(func() { a.checkOne(p, variantIndex, v) })
	return nil
}

func (a *App) checkOne(p provider.Provider, variantIndex int, v provider.Variant) {
	label := v.String()
	// markBusy (in CheckAll/CheckOne, before this ever got queued) already
	// flipped Status to "checking" while preserving FoundVersion, so it's
	// safe to just read it back here.
	found := a.statusFor(p.ID(), variantIndex, v).FoundVersion

	res, err := p.Check(a.ctx, v)
	if err != nil {
		a.setStatus(VariantStatus{ProviderID: p.ID(), VariantIndex: variantIndex, Label: label, FoundVersion: found, Status: StatusError, Error: err.Error()})
		return
	}

	status := StatusNotFound
	if found != "" {
		if found == res.Version {
			status = StatusUpToDate
		} else {
			status = StatusOutdated
		}
	}

	a.setStatus(VariantStatus{
		ProviderID:    p.ID(),
		VariantIndex:  variantIndex,
		Label:         label,
		LatestVersion: res.Version,
		FoundVersion:  found,
		Status:        status,
	})
}

// DownloadOne downloads the latest version found by a previous check.
// Progress streams via the "provider:status" event.
func (a *App) DownloadOne(providerID string, variantIndex int) error {
	p, v, err := a.lookup(providerID, variantIndex)
	if err != nil {
		return err
	}

	latest := a.statusFor(p.ID(), variantIndex, v).LatestVersion
	if latest == "" {
		return fmt.Errorf("run a check before downloading")
	}

	a.markBusy(p.ID(), variantIndex, v, StatusDownloading)
	a.goDownload(func() { a.downloadOne(p, variantIndex, v, provider.CheckResult{Version: latest}) })
	return nil
}

// DownloadAll downloads every visible variant that a previous Check found
// to be outdated or not-found. Variants that haven't been checked yet (no
// known LatestVersion) are skipped rather than implicitly checked first —
// run Check All beforehand to cover everything.
func (a *App) DownloadAll() {
	for _, p := range provider.All() {
		variants := p.Variants()
		for _, i := range a.visibleVariantIndexes(p) {
			v := variants[i]
			status := a.statusFor(p.ID(), i, v)
			if status.LatestVersion == "" {
				continue
			}
			if status.Status != StatusOutdated && status.Status != StatusNotFound {
				continue
			}
			a.markBusy(p.ID(), i, v, StatusDownloading)
			a.goDownload(func() { a.downloadOne(p, i, v, provider.CheckResult{Version: status.LatestVersion}) })
		}
	}
}

func (a *App) downloadOne(p provider.Provider, variantIndex int, v provider.Variant, res provider.CheckResult) {
	label := v.String()
	// markBusy (in DownloadOne/DownloadAll, before this ever got queued)
	// already flipped Status to "downloading".

	// Carry forward whatever version was already found on disk (if any) so
	// progress updates don't blank the "Found: ..." line to "Not found"
	// for the entire duration of a download that's just replacing an
	// existing file with a newer one — or, as in a re-download of a
	// version that was already fully present, misleadingly imply nothing
	// was there to begin with.
	previousFound := a.statusFor(p.ID(), variantIndex, v).FoundVersion

	progress := func(downloaded, total int64) {
		var pct float64
		if total > 0 {
			pct = float64(downloaded) / float64(total)
		}
		a.setStatus(VariantStatus{ProviderID: p.ID(), VariantIndex: variantIndex, Label: label, LatestVersion: res.Version, FoundVersion: previousFound, Status: StatusDownloading, Progress: pct})
	}

	// If a file for this variant was already found on disk (anywhere under
	// the download folder, not necessarily in the per-provider layout
	// below), save the new version alongside it instead of scattering it
	// into a second location — respects a pre-existing organization scheme.
	destDir := filepath.Join(a.cfg.General.DownloadDir, p.Name())
	if dir, ok := a.foundDirFor(p.ID(), variantIndex); ok && dir != "" {
		destDir = dir
	}
	if _, err := p.Download(a.ctx, v, res, destDir, progress); err != nil {
		a.setStatus(VariantStatus{ProviderID: p.ID(), VariantIndex: variantIndex, Label: label, LatestVersion: res.Version, Status: StatusError, Error: err.Error()})
		return
	}

	a.setStatus(VariantStatus{
		ProviderID:    p.ID(),
		VariantIndex:  variantIndex,
		Label:         label,
		LatestVersion: res.Version,
		FoundVersion:  res.Version,
		Status:        StatusUpToDate,
		Progress:      1,
	})
}

// ValidateOne re-verifies the file already found on disk for a variant
// against its provider's published checksum, without downloading anything.
// It always checks the single most-recently-found version for this variant
// (whatever scanFound last identified), never every historical file that
// might exist under the destination. Errors if nothing has been found yet.
func (a *App) ValidateOne(providerID string, variantIndex int) error {
	p, v, err := a.lookup(providerID, variantIndex)
	if err != nil {
		return err
	}

	status := a.statusFor(p.ID(), variantIndex, v)
	path, ok := a.foundPathFor(p.ID(), variantIndex)
	if status.FoundVersion == "" || !ok || path == "" {
		return fmt.Errorf("nothing downloaded yet to validate")
	}

	a.markBusy(p.ID(), variantIndex, v, StatusValidating)
	a.goValidate(func() { a.validateOne(p, variantIndex, v, status.LatestVersion, status.FoundVersion, path) })
	return nil
}

// ValidateAll re-verifies every visible variant that currently has a file
// found on disk.
func (a *App) ValidateAll() {
	for _, p := range provider.All() {
		variants := p.Variants()
		for _, i := range a.visibleVariantIndexes(p) {
			v := variants[i]
			status := a.statusFor(p.ID(), i, v)
			if status.FoundVersion == "" {
				continue
			}
			path, ok := a.foundPathFor(p.ID(), i)
			if !ok || path == "" {
				continue
			}
			a.markBusy(p.ID(), i, v, StatusValidating)
			a.goValidate(func() { a.validateOne(p, i, v, status.LatestVersion, status.FoundVersion, path) })
		}
	}
}

// validateOne re-hashes the file at path and compares it against the
// provider's published checksum for foundVersion (the version actually on
// disk, which isn't necessarily latest — this validates what's really
// there). A confirmed mismatch means the local file is corrupted: it's
// deleted, same as a fresh Download already does to a bad transfer, so a
// later Rescan doesn't keep reporting the bad file as "found" — the variant
// drops to "not found" so Download lights back up to fetch a clean copy.
// If the provider doesn't publish a checksum at all, or the checksum
// couldn't be fetched, nothing is deleted — there's no basis to conclude
// the file is bad, just that it couldn't be confirmed good.
func (a *App) validateOne(p provider.Provider, variantIndex int, v provider.Variant, latest, foundVersion, path string) {
	label := v.String()
	// ValidateOne/ValidateAll already called markBusy before this ever got
	// queued; setStatus it again anyway since, unlike checkOne/downloadOne,
	// validateOne is also called directly in tests without going through
	// either entry point.
	a.setStatus(VariantStatus{ProviderID: p.ID(), VariantIndex: variantIndex, Label: label, LatestVersion: latest, FoundVersion: foundVersion, Status: StatusValidating})

	fail := func(msg string) {
		a.setStatus(VariantStatus{ProviderID: p.ID(), VariantIndex: variantIndex, Label: label, LatestVersion: latest, FoundVersion: foundVersion, Status: StatusError, Error: msg})
	}

	checksum, err := p.Checksum(a.ctx, v, foundVersion)
	if err != nil {
		fail(fmt.Sprintf("validate: %v", err))
		return
	}
	if checksum == nil {
		a.setStatus(VariantStatus{ProviderID: p.ID(), VariantIndex: variantIndex, Label: label, LatestVersion: latest, FoundVersion: foundVersion, Status: StatusNotTested, Error: "no checksum published for this source — cannot validate"})
		return
	}

	got, err := download.HashFile(path, checksum.Algo)
	if err != nil {
		fail(fmt.Sprintf("validate: %v", err))
		return
	}

	if !strings.EqualFold(got, checksum.Hex) {
		_ = os.Remove(path)
		a.setStatus(VariantStatus{ProviderID: p.ID(), VariantIndex: variantIndex, Label: label, LatestVersion: latest, Status: StatusNotFound, Error: "local file was corrupted (checksum mismatch) and has been deleted — download again"})
		return
	}

	// A version comparison only means anything if foundVersion actually
	// looks like one — providers whose filename can't encode a real version
	// (e.g. hirensbootcdpe) report a fixed placeholder instead, and
	// comparing that numerically would make it read as "always older" by
	// accident (every non-numeric segment parses as 0) even right after a
	// checksum confirms the file is genuinely fine.
	restoredStatus := StatusUpToDate
	if latest != "" && scrape.LooksLikeVersion(foundVersion) && scrape.CompareVersions(foundVersion, latest) < 0 {
		restoredStatus = StatusOutdated
	}
	a.setStatus(VariantStatus{ProviderID: p.ID(), VariantIndex: variantIndex, Label: label, LatestVersion: latest, FoundVersion: foundVersion, Status: restoredStatus})
}

func (a *App) lookup(providerID string, variantIndex int) (provider.Provider, provider.Variant, error) {
	p, ok := provider.Get(providerID)
	if !ok {
		return nil, provider.Variant{}, fmt.Errorf("unknown provider %q", providerID)
	}
	variants := p.Variants()
	if variantIndex < 0 || variantIndex >= len(variants) {
		return nil, provider.Variant{}, fmt.Errorf("unknown variant index %d for provider %q", variantIndex, providerID)
	}
	return p, variants[variantIndex], nil
}
