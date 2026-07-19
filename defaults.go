package main

import (
	"github.com/arnaudcharles/iso-auto-downloader/internal/config"
	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
)

// legacyVariantRestriction describes, for a provider's first-ever seeding,
// which of its variants should start out disabled. It only feeds
// defaultISOConfigFor — once a config exists, everything is driven by the
// user's own per-variant checkboxes in Settings (see config.ISOConfig).
type legacyVariantRestriction struct {
	enabled bool     // if false, every variant of this provider starts disabled
	arch    []string // if non-empty, only these archs start enabled
	edition []string // if non-empty, only these editions start enabled
	lang    []string // if non-empty, only these langs start enabled
}

// legacyRestrictions implements the curated default ISO list from the
// project conversation: everything previously implemented but not on that
// list starts disabled (still available in Settings, one checkbox click
// away); a few providers start restricted to a subset of their variants.
// Providers not listed here start with every variant enabled.
var legacyRestrictions = map[string]legacyVariantRestriction{
	"fedora":        {enabled: false},
	"kalilinux":     {enabled: false},
	"memtest86plus": {enabled: false},

	"manjaro":    {enabled: true, edition: []string{"kde", "xfce"}},
	"debian":     {enabled: true, arch: []string{"amd64"}},
	"rockylinux": {enabled: true, edition: []string{"dvd", "boot", "minimal"}},
}

// defaultISOConfigFor computes the initial config.ISOConfig for a provider
// the app has never seen before (new install, or a provider newly
// registered since the config was last saved) — every variant enabled,
// except where legacyRestrictions says otherwise.
func defaultISOConfigFor(p provider.Provider) config.ISOConfig {
	restriction, ok := legacyRestrictions[p.ID()]
	if !ok {
		return config.ISOConfig{}
	}

	var disabled []int
	for i, v := range p.Variants() {
		if !restriction.enabled || !legacyVariantMatches(restriction, v) {
			disabled = append(disabled, i)
		}
	}
	return config.ISOConfig{DisabledVariants: disabled}
}

// legacyVariantMatches reports whether v satisfies r's Arch/Edition/Lang
// restriction. An empty list on a dimension means no restriction on it.
func legacyVariantMatches(r legacyVariantRestriction, v provider.Variant) bool {
	if len(r.arch) > 0 && !containsString(r.arch, v.Arch) {
		return false
	}
	if len(r.edition) > 0 && !containsString(r.edition, v.Edition) {
		return false
	}
	if len(r.lang) > 0 && !containsString(r.lang, v.Lang) {
		return false
	}
	return true
}

func containsString(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
