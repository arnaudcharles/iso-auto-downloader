# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- Omarchy (Linux) as a native provider, with its brand mark in the ISO list. It starts unchecked —
  the image is a single ~6 GB file — so enable it in Settings to see it on the main screen. Omarchy
  publishes neither a directory listing nor a GitHub release asset for its ISO, so Check reads the
  omarchy.org homepage — the only place the current build is stated — and downloads verify against
  the `omarchy-<version>.iso.sha256` file served next to the image.

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
