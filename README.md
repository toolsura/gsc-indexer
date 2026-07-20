
Index / re-index URLs via the Google Search Console **URL Inspection API**.
Inspecting a URL asks Google to crawl / re-fetch it — the practical way to
push a blog page into, or refresh it in, the index when you hold at least
"site full user" rights on the property.

> The Google Indexing API v3 is intentionally NOT used: it requires the
> service account to be an **Owner** and is scoped to JobPosting /
> BroadcastEvent pages, so it does not apply to general blog URLs.

## Install

Install the latest release with `go install` (Go 1.25+):

```
go install -v github.com/toolsura/gsc-indexer@latest
```

This puts the `gsc-indexer` binary in `$GOBIN` (default `~/go/bin`) — make sure
that directory is on your `PATH`. No release tag yet? Install the tip of `main`:

```
go install -v github.com/toolsura/gsc-indexer@main
```

## Build

```
go build -o gsc-indexer .
```

## Usage

```
# single URL
./gsc-indexer -creds sa.json "https://www.toolsura.com/post/"

# batch file (one URL per line, `#` comments ignored)
./gsc-indexer -creds sa.json -batch urls.txt

# sitemap: fetched + expanded into every <loc> (indexes followed recursively)
./gsc-indexer -creds sa.json "https://www.toolsura.com/sitemap.xml" -report ./report

# preview the URL set before a long run (no API calls, no credentials)
./gsc-indexer -creds sa.json "https://www.toolsura.com/sitemap.xml" -dry-run
```

Run `gsc-indexer -h` for these examples plus the full flag list.

## Documentation

Detailed guides and walkthroughs for gsc-indexer are published on the
project blog:

- 📖 **Docs & tutorials:** https://www.toolsura.com/blog/gsc-indexer-start-here/

The posts cover each workflow end to end — single-URL and batch indexing,
sitemap expansion, tracking indexed-vs-not over time with `-report`/`-diff`,
and the Google Search Console service-account (OAuth) setup.

## GitHub Action

gsc-indexer is also published as a **GitHub Action**, so you can re-index URLs
straight from CI on every push to your blog. It is listed on the GitHub
Marketplace — website: https://www.toolsura.com.

### Usage

```yaml
# .github/workflows/index.yml
name: Index on Search Console
on:
  push:
    branches: [main]
jobs:
  index:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: toolsura/gsc-indexer@v1
        with:
          service_account_json: ${{ secrets.GSC_SERVICE_ACCOUNT_JSON }}
          sitemap: "https://www.toolsura.com/sitemap.xml"
          delay: "10s"
          quiet: "true"
          report: "./report"
```

### Inputs

| Input | Required | Default | Maps to |
|-------|----------|---------|---------|
| `service_account_json` | yes | — | `-creds` (written to a `chmod 600` temp file) |
| `api_key` | no | — | `-apikey` |
| `site` | no | `https://www.toolsura.com/` | `-site` |
| `urls` | no | — | positional args (one URL per line) |
| `sitemap` | no | — | positional `.xml` URL (auto-expanded) |
| `delay` | no | `1s` | `-delay` |
| `quiet` | no | `false` | `-q` |
| `dry_run` | no | `false` | `-dry-run` |
| `json` | no | `false` | `-json` |
| `color` | no | `never` | `-color` |
| `report` | no | — | `-report` |
| `diff` | no | — | `-diff` |

> The Action writes your `service_account_json` secret to a `600`-permission
> temp file that is removed on exit; the secret is never echoed to the log.

## Flags

- `-creds`  service-account JSON (or env `GSC_CREDENTIALS`) — **required** for GSC
- `-apikey` Google API key (or env `GSC_API_KEY`) — accepted, but GSC's URL
  Inspection API requires OAuth, so a key yields **401**; use `-creds`
- `-site`   GSC property (default `https://www.toolsura.com/`)
- `-batch`  file of URLs (alternative to a positional URL)
- `-delay`  pause between requests (default `1s`, also applied between
  sitemap-expanded URLs); raise it (e.g. `10s`) for large batches so Google
  does not throttle the run
- `-json`   emit results as JSON (human-readable progress is suppressed)
- `-q`      quiet: show only a live progress line + final summary + errors
  (per-URL detail suppressed — use for long sitemap runs)
- `-color`  color output: `auto` (default, on for interactive terminals only),
  `always`, or `never`. Color is decorative only — the INDEXED / NOT INDEXED /
  ERROR text always carries the meaning, so piped/redirected output stays clean
- `-dry-run` expand sitemaps / read the batch, list every URL that *would* be
  inspected, then exit. **No API calls, no credentials required**
- `-report` write `indexed.txt` / `not-indexed.txt` / `summary.json` into a dir
- `-diff`   compare against a saved `summary.json` and report what changed
  (newly indexed / dropped / unchanged)

## URL sources

All sources are composable and **deduped**, then run together:

- **positional args** — one or more URLs
- **`-batch` file** — one URL per line; `#` comments and blank lines ignored
- **stdin pipe** — `cat urls.txt | ./gsc-indexer -creds sa.json`
- **sitemap URLs** — any arg / batch line ending in `.xml` is fetched and
  expanded into its `<loc>` entries; nested sitemap indexes are followed
  recursively

```
./gsc-indexer -creds sa.json "https://www.toolsura.com/post/"
cat urls.txt | ./gsc-indexer -creds sa.json
./gsc-indexer -creds sa.json -batch urls.txt
./gsc-indexer -creds sa.json "https://www.toolsura.com/sitemap.xml" -report ./report
```

## Track indexed vs not over time

Run with `-report` to snapshot the current state:

```
./gsc-indexer -creds sa.json -batch urls.txt -report ./report
```

Later, re-run and pass the saved `summary.json` to `-diff`:

```
./gsc-indexer -creds sa.json -batch urls.txt -diff ./report/summary.json
```

This prints `▲ newly indexed` / `▼ dropped` lines plus before→after counts.
Combine both (`-report ./report -diff ./report/summary.json`) to update and
compare in one pass.

## Preview before a long run

A sitemap can resolve to 100+ URLs, and at a polite `-delay` (e.g. `10s`) that
is a 15–20 minute run. Two flags keep it in control:

- `-dry-run` lists the full expanded URL set **without calling the API** — so
  you can confirm the count and contents first.
- `-q` collapses per-URL detail into a single live progress line with a
  remaining-time estimate, then prints the summary:

```
./gsc-indexer -creds sa.json "https://www.toolsura.com/sitemap.xml" -dry-run
./gsc-indexer -creds sa.json "https://www.toolsura.com/sitemap.xml" -delay 10s -q -report ./report
```

## Output

For each URL: indexed/not, coverage state, fetch state, robots state, and last
crawl time. Inspecting the URL already submits it for re-crawl — that
confirmation is printed **once**, after the run, not per URL.

With `-report`, results are split into `indexed.txt` / `not-indexed.txt` and a
`summary.json` (used by `-diff`). With `-json`, results are emitted as a single
JSON array at the end; progress lines are suppressed so the output stays valid
machine-readable data.

Exit code is non-zero if any URL fails (network / HTTP / parse) so the tool is
safe in CI pipelines. URLs merely "not indexed" are **not** failures.
```
# gsc-indexer
