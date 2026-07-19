# 🧩 Contributing Guide

Thank you for your interest in contributing to ISO Auto Downloader. This guide explains how to set up your environment, work on the codebase, and submit changes in a clean and consistent way.

## 🚀 Set up your environment

Before you start, make sure you have the required tools installed:

- Go
- Node.js
- The Wails CLI

Install the Wails CLI with:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Then you can run the app locally in development mode:

```bash
wails dev
```

Useful checks before submitting changes:

```bash
go test ./...
go vet ./...
```

## 🧱 Project structure

The project is organized around a few key areas:

- `internal/provider/` contains the native ISO providers
- `internal/download/` handles download and verification flows
- `internal/config/` manages configuration state
- `frontend/` contains the desktop UI built with Wails/React

If you are making changes to the app behavior, it is usually helpful to inspect the relevant provider or core package first.

## ✍️ Adding a native ISO provider

Providers live in `internal/provider/<name>/` and implement the `provider.Provider` interface defined in `internal/provider/provider.go`.

Each provider should:

- self-register with `provider.Register(...)` inside an `init()` function
- keep its base URL as a package-level `var` rather than `const` so tests can point it at an `httptest.Server`
- implement `LocalVersion(filename, variant)` by adapting the same regex used in `Check` or `Download` to match a bare filename
- ship a `_test.go` file covering the check, download, and `LocalVersion` behavior

A good reference is the existing providers in `internal/provider/ubuntu`, `internal/provider/debian`, and `internal/provider/memtest86plus`.

When implementing a new provider, the workflow is usually:

1. find the official upstream download page
2. identify the latest-version detection logic
3. extract the download URL pattern and any necessary regexes
4. implement the provider natively in Go
5. add tests that cover the main success and edge cases

## 🧪 Testing

Before submitting a pull request, make sure your changes are covered and still pass the existing test suite:

```bash
go test ./...
```

If you add or change behavior, prefer updating or adding tests rather than leaving the change unverified.

## 🧹 Code style

A few simple conventions help keep the codebase readable:

- keep code clear and explicit
- prefer small, focused functions
- follow the existing Go style already used in the project
- document public functions and non-obvious behavior when needed

## 💡 Custom ISOs and community sharing

The custom ISO scripting system is not implemented yet. Once it is added, this section will cover how community-submitted custom ISO definitions should be reviewed and validated, since importing them means trusting the URLs and scripts they contain.

## 🧭 Creating a pull request

Once your branch is ready:

1. commit your changes clearly
2. push your branch to GitHub
3. open a pull request with a concise description of what changed

A good PR description should include:

- a summary of the change
- the main files or areas affected
- any testing performed

## 🔍 Review process

After opening a pull request, be ready to discuss the change and adjust it if needed. Feedback is part of the process, and thoughtful iterations help keep the project consistent and maintainable.

Thank you for contributing to ISO Auto Downloader. Your help makes the project better.
