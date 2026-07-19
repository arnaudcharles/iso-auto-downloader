package main

import (
	"testing"

	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
)

// enabledVariantIndexes mirrors what visibleVariantIndexes would compute
// from a seeded config.ISOConfig, without needing a full App.
func enabledVariantIndexes(t *testing.T, providerID string) []int {
	t.Helper()
	p, ok := provider.Get(providerID)
	if !ok {
		t.Fatalf("provider %q not registered", providerID)
	}
	iso := defaultISOConfigFor(p)
	var idxs []int
	for i := range p.Variants() {
		if !containsInt(iso.DisabledVariants, i) {
			idxs = append(idxs, i)
		}
	}
	return idxs
}

func TestDefaultISOConfigForCuratedList(t *testing.T) {
	cases := []struct {
		id      string
		enabled bool
	}{
		{"fedora", false},
		{"kalilinux", false},
		{"memtest86plus", false},
		{"ubuntu", true},
		{"hirensbootcdpe", true},
		{"xcpng", true},
	}
	for _, c := range cases {
		got := len(enabledVariantIndexes(t, c.id)) > 0
		if got != c.enabled {
			t.Errorf("provider %q starts with any variant enabled = %v, want %v", c.id, got, c.enabled)
		}
	}
}

func TestDefaultISOConfigForVariantRestrictions(t *testing.T) {
	debian, ok := provider.Get("debian")
	if !ok {
		t.Fatal("debian not registered")
	}
	iso := defaultISOConfigFor(debian)
	for i, v := range debian.Variants() {
		want := v.Arch == "amd64"
		got := !containsInt(iso.DisabledVariants, i)
		if got != want {
			t.Errorf("debian variant %d (%s) enabled = %v, want %v", i, v.String(), got, want)
		}
	}

	mint, ok := provider.Get("linuxmint")
	if !ok {
		t.Fatal("linuxmint not registered")
	}
	if got := len(enabledVariantIndexes(t, "linuxmint")); got != len(mint.Variants()) {
		t.Errorf("linuxmint enabled variant count = %d, want all %d (no restriction)", got, len(mint.Variants()))
	}

	manjaro, ok := provider.Get("manjaro")
	if !ok {
		t.Fatal("manjaro not registered")
	}
	iso = defaultISOConfigFor(manjaro)
	for i, v := range manjaro.Variants() {
		want := v.Edition == "kde" || v.Edition == "xfce"
		got := !containsInt(iso.DisabledVariants, i)
		if got != want {
			t.Errorf("manjaro variant %d (%s) enabled = %v, want %v", i, v.String(), got, want)
		}
	}

	rocky, ok := provider.Get("rockylinux")
	if !ok {
		t.Fatal("rockylinux not registered")
	}
	if got := len(enabledVariantIndexes(t, "rockylinux")); got != len(rocky.Variants()) {
		t.Errorf("rockylinux enabled variant count = %d, want all %d (dvd/boot/minimal is everything it has)", got, len(rocky.Variants()))
	}
}

func TestContainsInt(t *testing.T) {
	if !containsInt([]int{1, 2, 3}, 2) {
		t.Error("expected 2 to be found in [1,2,3]")
	}
	if containsInt([]int{1, 2, 3}, 5) {
		t.Error("expected 5 to not be found in [1,2,3]")
	}
	if containsInt(nil, 0) {
		t.Error("expected nil list to contain nothing")
	}
}
