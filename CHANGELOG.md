# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.1.2] - 2026-09-04

### Added

- Omarchy (Linux) as a native provider, with its brand mark in the ISO list. It starts unchecked —
  the image is a single ~6 GB file — so enable it in Settings to see it on the main screen. Omarchy
  publishes neither a directory listing nor a GitHub release asset for its ISO, so Check reads the
  omarchy.org homepage — the only place the current build is stated — and downloads verify against
  the `omarchy-<version>.iso.sha256` file served next to the image.

### Changed

- Releases now publish to Homebrew, Scoop and Chocolatey from a single `v*` tag. Chocolatey was
  previously packaged by hand, and its files had drifted — the nuspec said 0.1.1 while
  VERIFICATION.txt still pointed at the v0.1.0 archive; all three channels now render from
  templates with the version taken from the tag.
- The GitHub Release is published directly instead of as a draft. Every package manager bakes a
  `releases/download/<tag>/...` URL into its manifest, and a draft's asset URLs return 404 for
  everyone but the repo owner, so `brew install --cask` was broken between a tag and its manual
  publication. Pull requests now run the full release build in exchange.

## [0.1.1] - 2026-07-20

### Fixed

- A Check that hit a network blip (real report: Debian failed on every architecture at once with
  "TLS handshake timeout") showed a permanent-looking error instead of recovering — Check had no
  retry at all, unlike downloads, which already retried this exact class of transient failure.
  `scrape.FetchString`/`scrape.Resolve` (what every provider's Check goes through) now retry up to
  3 times with a short backoff before giving up.

## [0.1.0] - 2026-07-18

### Added

- Initial commit.
