// Package provider defines the contract every ISO source (native or,
// later, script-backed custom ISO) must implement, and a registry that
// native providers self-register into via init().
package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/arnaudcharles/iso-auto-downloader/internal/download"
)

// Category groups providers for the UI's section layout.
type Category string

const (
	CategoryDiagnostic     Category = "diagnostic"
	CategoryDiskUtility    Category = "disk-utility"
	CategoryLinux          Category = "linux"
	CategoryWindows        Category = "windows"
	CategoryBSD            Category = "bsd"
	CategoryVirtualization Category = "virtualization"
)

var categoryLabels = map[Category]string{
	CategoryDiagnostic:     "Diagnostic Tools",
	CategoryDiskUtility:    "Disk Utilities",
	CategoryLinux:          "Linux",
	CategoryWindows:        "Windows",
	CategoryBSD:            "BSD",
	CategoryVirtualization: "Virtualization",
}

// Label is the human-readable form used in the UI and as a download
// subfolder name (e.g. destDir/<Label>/<Provider.Name()>/...).
func (c Category) Label() string {
	if label, ok := categoryLabels[c]; ok {
		return label
	}
	return string(c)
}

// Variant identifies one arch/edition/lang combination a provider supports.
// Providers only list the dimensions that are actually meaningful to them —
// e.g. an ISO with a single architecture leaves Arch empty.
type Variant struct {
	Arch    string
	Edition string
	Lang    string
}

func (v Variant) String() string {
	s := ""
	for _, part := range []string{v.Edition, v.Arch, v.Lang} {
		if part == "" {
			continue
		}
		if s != "" {
			s += " / "
		}
		s += part
	}
	if s == "" {
		return "default"
	}
	return s
}

// CheckResult is the outcome of resolving the latest available version for
// a variant. It intentionally carries nothing beyond the version string:
// Download must be able to re-derive the URL from Version alone, matching
// the contract custom script-backed providers (Step 2) will also have to
// satisfy (DOWNLOAD scripts only ever receive ISOAD_VERSION from CHECK).
type CheckResult struct {
	Version string
}

// DownloadResult describes the file that ended up on disk.
type DownloadResult struct {
	Path      string
	Filename  string
	SizeBytes int64
}

// ProgressFunc is invoked as bytes arrive; total may be 0 if the server
// didn't report a Content-Length.
type ProgressFunc func(downloaded, total int64)

// Provider is implemented once per ISO (or ISO family). Native providers
// register themselves in this package's registry from an init() function.
type Provider interface {
	ID() string
	Name() string
	Category() Category
	Variants() []Variant

	Check(ctx context.Context, v Variant) (CheckResult, error)
	Download(ctx context.Context, v Variant, res CheckResult, destDir string, progress ProgressFunc) (DownloadResult, error)

	// Checksum returns the expected checksum for v at the given version, or
	// (nil, nil) if this source doesn't publish one for its files (e.g.
	// Ultimate Boot CD, Tails). Download uses this internally to verify a
	// fresh transfer; it's also exposed standalone so an already-downloaded
	// file can be re-verified without downloading it again.
	Checksum(ctx context.Context, v Variant, version string) (*download.Checksum, error)

	// LocalVersion reports whether filename (a basename, no path) looks
	// like a previously-downloaded ISO for v, and if so, the version
	// encoded in it. Used to detect ISOs that already exist on disk —
	// wherever they are under the configured download folder, not just
	// the exact subfolder this provider itself would have used — so
	// pre-existing or manually-organized collections are recognized too.
	LocalVersion(filename string, v Variant) (version string, ok bool)
}

var registry = map[string]Provider{}

// Register adds a provider to the global registry. Called from provider
// packages' init() functions; panics on duplicate IDs since that can only
// happen from a programming error, never from user input.
func Register(p Provider) {
	if _, exists := registry[p.ID()]; exists {
		panic(fmt.Sprintf("provider: duplicate id %q", p.ID()))
	}
	registry[p.ID()] = p
}

// All returns every registered provider, sorted by ID for stable UI ordering.
func All() []Provider {
	out := make([]Provider, 0, len(registry))
	for _, p := range registry {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// Get looks up a single provider by ID.
func Get(id string) (Provider, bool) {
	p, ok := registry[id]
	return p, ok
}
