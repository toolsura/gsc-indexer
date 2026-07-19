# Changelog

All notable changes to `gsc-indexer` are documented in this file.

The project adheres to [Semantic Versioning](https://semver.org/).

## [0.1.0] - 2026-07-19

Initial public release of `gsc-indexer` — a CLI to request (re-)crawling of
URLs through the Google Search Console **URL Inspection API**.

### Added

- **Flexible URL sources**, all deduped and composable:
  - positional URLs
  - batch files (`-batch`, `#` comments and blanks ignored)
  - stdin pipes
  - sitemaps (`*.xml`, expanded recursively into every `<loc>`)
- **`-dry-run`** — expand and list the full URL set without calling the API
  (no credentials required), so long runs can be previewed first.
- **`-report` / `-diff`** — snapshot indexed-vs-not-indexed state and compare
  it across runs to track progress over time.
- **`-delay` / `-q`** — polite pacing and a single live progress line for large
  sitemap runs, avoiding Google rate-limiting.
- **`-json`** — machine-readable output for pipelines, with a non-zero exit
  code on real failures (network / HTTP / parse) so it is safe in CI.
- Per-URL results report coverage state, fetch state, robots state, and last
  crawl time.

### Requirements

- Go **1.25+** (built and tested with the current Go toolchain).
- A Google Search Console **service account** with **Full** access to the
  property. The URL Inspection API requires OAuth — an API key returns `401`.
  The Indexing API is intentionally not used (it is scoped to JobPosting /
  BroadcastEvent pages).

### Install

```sh
go install -v github.com/toolsura/gsc-indexer@latest
```

Ensure `$GOBIN` (default `~/go/bin`) is on your `PATH`.
