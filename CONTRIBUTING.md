# Contributing

## Setup

Requires Go, Node.js, and the [Wails CLI](https://wails.io/docs/gettingstarted/installation) (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`).

```sh
wails dev          # live development with hot reload
go test ./...       # run provider/download/config unit tests
go vet ./...
```

## Adding a native ISO provider

Providers live in `internal/provider/<name>/` and implement the
`provider.Provider` interface (`internal/provider/provider.go`): `ID`,
`Name`, `Category`, `Variants`, `Check`, `Download`, and `LocalVersion`. See
`internal/provider/ubuntu`, `internal/provider/debian`, and
`internal/provider/memtest86plus` for reference implementations, and
[docs/architecture.md](docs/architecture.md) for the design this contract
follows.

Each provider should:

- Self-register via `provider.Register(...)` in an `init()` function.
- Keep any base URL as a package-level `var` (not `const`) so tests can
  point it at an `httptest.Server`.
- Implement `LocalVersion(filename, variant)` by adapting the same regex
  already used in `Check`/`Download` to match a bare filename instead of an
  href — this is what lets the app detect ISOs already on disk.
- Ship a `_test.go` with an `httptest`-mocked check, download, and a couple
  of `LocalVersion` cases (a match and a near-miss), per the existing
  providers.

When implementing a new provider, find the upstream project's official
download page and mirror layout, work out its check/download URLs and
regexes, then implement them natively in Go.

## Custom ISOs and community sharing

The custom-ISO scripting system (Step 2 of the project) isn't implemented
yet. Once it lands, this section will cover the review process for
community-submitted custom ISO definitions, since importing one means
trusting the scripts/URLs it contains.
