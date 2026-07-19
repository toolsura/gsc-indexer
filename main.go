// gsc-indexer — index / re-index URLs via the Google Search Console
// URL Inspection API.
//
// The GSC URL Inspection API both reports a URL's index status AND asks
// Google to crawl / re-fetch it — the practical way to push a blog page
// into (or refresh it in) the index for properties where you hold at
// least "site owner / full user" rights.
//
// The Google Indexing API v3 (JobPosting/BroadcastEvent) is NOT used: it
// requires the service account to be an Owner and is scoped to job/broadcast
// structured data, so it does not apply to general blog pages.
//
// URL sources (all composable): positional args, -batch file, stdin pipe,
// and sitemap URLs (auto-detected by the .xml suffix) which are fetched and
// expanded into their <loc> entries.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
)

const inspectEndpoint = "https://searchconsole.googleapis.com/v1/urlInspection/index:inspect"

// maxExpand caps sitemap-expanded URLs so a huge sitemap can't blow up a run.
const maxExpand = 50000

type result struct {
	URL           string `json:"url"`
	Indexed       bool   `json:"indexed"`
	CoverageState string `json:"coverage_state,omitempty"`
	CrawledAs     string `json:"crawled_as,omitempty"`
	RobotsState   string `json:"robots_state,omitempty"`
	PageFetch     string `json:"page_fetch_state,omitempty"`
	Indexable     string `json:"indexing_state,omitempty"`
	LastCrawl     string `json:"last_crawl_time,omitempty"`
	ReferringURLs int    `json:"referring_urls,omitempty"`
	Error         string `json:"error,omitempty"`
}

// Flag values mirrored at package scope so output helpers (color, etc.) can
// read them without threading them through every call.
var (
	asJSON    *bool
	colorMode *string
)

func main() {
	// Prefix-free fatal messages read cleaner in a terminal or CI log.
	log.SetFlags(0)

	creds := flag.String("creds", os.Getenv("GSC_CREDENTIALS"), "path to service-account JSON (or set GSC_CREDENTIALS)")
	apiKey := flag.String("apikey", os.Getenv("GSC_API_KEY"), "Google API key (or set GSC_API_KEY)")
	site := flag.String("site", "https://www.toolsura.com/", "GSC property / site URL")
	batch := flag.String("batch", "", "file with one URL per line (alternative to positional URL)")
	delay := flag.Duration("delay", time.Second, "delay between requests (all URLs, including sitemap-expanded)")
	asJSON = flag.Bool("json", false, "output results as JSON")
	quiet := flag.Bool("q", false, "quiet: show only a progress line + final summary + errors")
	dryRun := flag.Bool("dry-run", false, "expand & list the URLs that would be inspected, then exit (no API calls)")
	colorMode = flag.String("color", "auto", "color output: auto (terminal only), always, or never")
	report := flag.String("report", "", "write indexed.txt / not-indexed.txt + summary into this dir")
	diff := flag.String("diff", "", "compare against a saved summary.json and report changes")
	flag.Usage = usage
	flag.Parse()

	raw, err := collectURLs(*batch, flag.Args())
	if err != nil {
		log.Fatalf("reading URLs: %v", err)
	}
	// Flag parsing stops at the first non-flag argument, so a flag placed after
	// a URL (e.g. `… url -color always`) is silently treated as a URL and would
	// later produce a confusing 403 from GSC. Catch it early with a clear error.
	if err := validateURLs(raw); err != nil {
		log.Fatal(err)
	}
	urls, err := expandURLs(raw)
	if err != nil {
		log.Fatalf("expanding sitemaps: %v", err)
	}
	if len(urls) == 0 {
		log.Fatal("no URLs given: pass a URL / sitemap, -batch file.txt, or pipe URLs via stdin")
	}

	// Dry-run needs no credentials — it only resolves and lists the URL set.
	if *dryRun {
		fmt.Printf("Dry-run: %d URL(s) would be inspected (no API calls made):\n", len(urls))
		for _, u := range urls {
			fmt.Println("  " + u)
		}
		return
	}

	if *creds == "" && *apiKey == "" {
		log.Fatal("missing credentials: pass -creds /path/to/sa.json, -apikey KEY, or set GSC_CREDENTIALS / GSC_API_KEY")
	}

	ctx := context.Background()
	client, err := authClient(ctx, *creds, *apiKey)
	if err != nil {
		log.Fatalf("auth: %v", err)
	}

	results := make([]result, 0, len(urls))
	var failed int
	total := len(urls)
	term := isTerminal(os.Stdout)
	for i, u := range urls {
		r := inspect(ctx, client, *site, u, *apiKey)
		results = append(results, r)
		if r.Error != "" {
			failed++
		}
		if !*asJSON {
			switch {
			case *quiet && term:
				left := time.Duration(total-(i+1)) * *delay
				fmt.Printf("\r  %d/%d  %-11s (%-7s left)  %s%s",
					i+1, total, c(stateColor(r), stateLabel(r)), fmtDuration(left), u,
					strings.Repeat(" ", 12))
			case *quiet:
				fmt.Printf("  %d/%d  %-11s  %s\n", i+1, total, c(stateColor(r), stateLabel(r)), u)
			default:
				printResult(r, i+1, total)
			}
		}
		if i < total-1 && *delay > 0 {
			time.Sleep(*delay)
		}
	}
	if !*asJSON {
		if *quiet {
			fmt.Println()
			for _, r := range results {
				if r.Error != "" {
					fmt.Printf("  %s %s — %s\n", c(ansiRed, "✗"), r.URL, r.Error)
				}
			}
		} else {
			fmt.Println("→ inspection submitted for all URLs; Google will re-crawl on next pass.")
		}
	}

	if *asJSON {
		out, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(out))
	}

	if *report != "" {
		if err := writeReport(*report, results); err != nil {
			log.Fatalf("writing report: %v", err)
		}
		var idx, not int
		for _, r := range results {
			if r.Error == "" && r.Indexed {
				idx++
			} else {
				not++
			}
		}
		fmt.Printf("\nSUMMARY: %d indexed, %d not indexed of %d URLs\n", idx, not, len(results))
		fmt.Printf("  → %s/indexed.txt\n  → %s/not-indexed.txt\n", *report, *report)
	}

	if *diff != "" {
		base, err := loadSummary(*diff)
		if err != nil {
			log.Fatalf("loading baseline %s: %v", *diff, err)
		}
		if err := printDiff(*diff, base, results); err != nil {
			log.Fatalf("diff: %v", err)
		}
	}

	// A non-zero exit if any URL failed (network/HTTP/parse) so the tool is
	// safe in CI pipelines. URLs merely "not indexed" are not failures.
	if failed > 0 {
		os.Exit(1)
	}
}

// validateURLs rejects arguments that look like misplaced flags. The flag
// package stops parsing at the first non-flag argument, so a flag written
// after a URL (e.g. `… url -color always`) is captured as a positional and
// would otherwise be sent to GSC as a URL, yielding a confusing 403.
func validateURLs(raw []string) error {
	for _, u := range raw {
		if strings.HasPrefix(u, "-") {
			return fmt.Errorf("unexpected argument %q — flags must come before URLs; move %q (and any other flags) ahead of the URL list", u, u)
		}
	}
	return nil
}

// collectURLs merges a batch file, stdin pipe (if present), and positional
// args into a deduped URL list. A sitemap URL is kept as-is here and expanded
// later by expandURLs.
func collectURLs(batch string, args []string) ([]string, error) {
	seen := make(map[string]struct{})
	var out []string
	add := func(u string) {
		u = strings.TrimSpace(u)
		if u == "" || strings.HasPrefix(u, "#") {
			return
		}
		if _, ok := seen[u]; ok {
			return
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}

	if batch != "" {
		f, err := os.Open(batch)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			add(sc.Text())
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}

	// Pipe support: only read stdin when it is not a terminal (i.e. piped).
	if fi, err := os.Stdin.Stat(); err == nil {
		if fi.Mode()&os.ModeCharDevice == 0 {
			sc := bufio.NewScanner(os.Stdin)
			for sc.Scan() {
				add(sc.Text())
			}
		}
	}

	for _, a := range args {
		add(a)
	}
	return out, nil
}

// expandURLs replaces any sitemap URL (ends in .xml) with the URLs it lists,
// traversing sitemap indexes recursively. Non-sitemap URLs pass through.
func expandURLs(raw []string) ([]string, error) {
	var out []string
	for _, u := range raw {
		if !strings.HasSuffix(strings.ToLower(u), ".xml") {
			out = append(out, u)
			continue
		}
		got, err := sitemapURLs(u)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", u, err)
		}
		out = append(out, got...)
		if len(out) > maxExpand {
			return nil, fmt.Errorf("refusing to expand beyond %d URLs", maxExpand)
		}
	}
	return out, nil
}

// sitemapClient fetches sitemaps with a bounded timeout so a stalled host
// can't hang the whole run.
var sitemapClient = &http.Client{Timeout: 30 * time.Second}

// sitemapFetch is the function used to retrieve a sitemap's body. It is a
// variable (not a direct http.Get call) so tests can substitute a fake.
var sitemapFetch = func(smURL string) (*http.Response, error) {
	return sitemapClient.Get(smURL)
}

// sitemapURLs fetches a sitemap (or sitemap index) and returns its <loc> URLs,
// following nested sitemap indexes recursively.
func sitemapURLs(smURL string) ([]string, error) {
	resp, err := sitemapFetch(smURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var urls []string
	dec := xml.NewDecoder(resp.Body)
	var inLoc bool
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			inLoc = t.Name.Local == "loc"
		case xml.CharData:
			if inLoc {
				u := strings.TrimSpace(string(t))
				if u != "" {
					urls = append(urls, u)
				}
				inLoc = false
			}
		}
	}

	// A sitemap index lists child sitemaps (also .xml) — expand each.
	var expanded []string
	for _, u := range urls {
		if strings.HasSuffix(strings.ToLower(u), ".xml") {
			child, err := sitemapURLs(u)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", u, err)
			}
			expanded = append(expanded, child...)
		} else {
			expanded = append(expanded, u)
		}
	}
	return expanded, nil
}

// authClient returns an *http.Client. With an API key it is a plain client
// (key sent as a query param per request); otherwise it builds a
// service-account OAuth client. GSC requires OAuth, so prefer -creds.
// Both clients get a request timeout so a stalled/throttled call fails fast
// instead of hanging the whole run.
func authClient(ctx context.Context, creds, apiKey string) (*http.Client, error) {
	if apiKey != "" {
		return &http.Client{Timeout: 30 * time.Second}, nil
	}
	b, err := os.ReadFile(creds)
	if err != nil {
		return nil, err
	}
	cfg, err := google.JWTConfigFromJSON(b, "https://www.googleapis.com/auth/webmasters.readonly")
	if err != nil {
		return nil, err
	}
	// The returned client bounds every inspect call (30s). Token refreshes use
	// the library's own client; add a transport with timeouts if refreshes are
	// observed to stall under heavy throttling.
	client := cfg.Client(ctx)
	client.Timeout = 30 * time.Second
	return client, nil
}

func inspect(ctx context.Context, client *http.Client, site, u, apiKey string) result {
	body, _ := json.Marshal(map[string]string{
		"inspectionUrl": u,
		"siteUrl":       site,
	})
	endpoint := inspectEndpoint
	if apiKey != "" {
		endpoint += "?key=" + url.QueryEscape(apiKey)
	}

	// Retry on rate-limit / transient server errors. Each attempt gets a fresh
	// request (a drained body can't be reused) and a hard per-attempt deadline
	// so a stalled call can never hang the whole run — it fails, backs off,
	// and retries. (GSC throttles sustained inspect volume.)
	const maxAttempts = 4
	var raw []byte
	var status int
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, strings.NewReader(string(body)))
		if err != nil {
			cancel()
			return result{URL: u, Error: err.Error()}
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			cancel()
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * 700 * time.Millisecond)
			continue
		}
		raw, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()
		status = resp.StatusCode
		lastErr = nil
		if status == http.StatusTooManyRequests || status >= 500 {
			time.Sleep(time.Duration(attempt+1) * 700 * time.Millisecond)
			continue
		}
		break
	}
	if lastErr != nil {
		return result{URL: u, Error: lastErr.Error()}
	}
	resp := &http.Response{StatusCode: status}

	if resp.StatusCode != http.StatusOK {
		return result{URL: u, Error: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))}
	}
	return parseInspection(u, raw)
}

// parseInspection pulls the fields we care about out of the inspection result.
func parseInspection(u string, raw []byte) result {
	var full struct {
		InspectionResult struct {
			IndexStatusResult struct {
				CoverageState  string   `json:"coverageState"`
				CrawledAs      string   `json:"crawledAs"`
				RobotsState    string   `json:"robotsState"`
				PageFetchState string   `json:"pageFetchState"`
				IndexingState  string   `json:"indexingState"`
				LastCrawlTime  string   `json:"lastCrawlTime"`
				ReferringURLs  []string `json:"referringUrls"`
				Verdict        string   `json:"verdict"`
			} `json:"indexStatusResult"`
		} `json:"inspectionResult"`
	}
	r := result{URL: u}
	if err := json.Unmarshal(raw, &full); err != nil {
		r.Error = "parse: " + err.Error()
		return r
	}
	ir := full.InspectionResult.IndexStatusResult
	r.CoverageState = ir.CoverageState
	r.CrawledAs = ir.CrawledAs
	r.RobotsState = ir.RobotsState
	r.PageFetch = ir.PageFetchState
	r.Indexable = ir.IndexingState
	r.LastCrawl = ir.LastCrawlTime
	r.ReferringURLs = len(ir.ReferringURLs)
	// A URL is "indexed" when Google has it in the index (verdict PASS and a
	// non-excluded coverage state).
	r.Indexed = ir.Verdict == "PASS" && !strings.Contains(strings.ToLower(ir.CoverageState), "excluded")
	return r
}

func printResult(r result, idx, total int) {
	pos := fmt.Sprintf("[%d/%d] ", idx, total)
	if r.Error != "" {
		fmt.Printf("%s%s %s\n    %s %s\n", pos, c(ansiRed, "✗"), r.URL, c(ansiRed, "error:"), r.Error)
		return
	}
	state, stateCode := "NOT INDEXED", ansiYellow
	if r.Indexed {
		state, stateCode = "INDEXED", ansiGreen
	}
	fmt.Printf("%s• %s  [%s]\n", pos, r.URL, c(stateCode, state))
	fmt.Printf("    coverage: %s | fetch: %s | robots: %s | last crawl: %s\n",
		r.CoverageState, r.PageFetch, r.RobotsState, r.LastCrawl)
}

// stateLabel is the one-word status used by the quiet progress line.
func stateLabel(r result) string {
	switch {
	case r.Error != "":
		return "ERROR"
	case r.Indexed:
		return "INDEXED"
	default:
		return "NOT INDEXED"
	}
}

// fmtDuration renders a remaining-time estimate compactly (e.g. "12m30s").
func fmtDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	s := int(d.Seconds())
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	return fmt.Sprintf("%dm%02ds", s/60, s%60)
}

// isTerminal reports whether f is an interactive character device (vs a pipe
// or file), so progress output can avoid carriage-return tricks when logged.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// usage replaces the default flag.Usage with examples + flag list so the tool
// is discoverable without leaving the terminal.
func usage() {
	fmt.Fprintf(os.Stderr, "gsc-indexer — index/re-index URLs via the Google Search Console URL Inspection API.\n\n")
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  gsc-indexer -creds sa.json \"https://www.toolsura.com/post/\"\n")
	fmt.Fprintf(os.Stderr, "  gsc-indexer -creds sa.json -batch urls.txt -report ./out\n")
	fmt.Fprintf(os.Stderr, "  gsc-indexer -creds sa.json \"https://www.toolsura.com/sitemap.xml\" -dry-run\n\n")
	fmt.Fprintf(os.Stderr, "Flags:\n")
	flag.PrintDefaults()
}

// ANSI color codes. Color is purely decorative — the textual state (INDEXED /
// NOT INDEXED / ERROR) always carries the meaning, so output stays readable
// when color is off or stripped by a pipe.
const (
	ansiReset  = "\033[0m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
)

// colorActive reports whether colored output should be emitted: never for
// JSON (it would corrupt the data), forced by -color=always/never, and on by
// default only when stdout is an interactive terminal.
func colorActive() bool {
	// Guard against nil so output helpers are callable before flag.Parse
	// (e.g. direct unit tests) — default to no color in that case.
	if asJSON != nil && *asJSON {
		return false
	}
	switch {
	case colorMode != nil && *colorMode == "always":
		return true
	case colorMode != nil && *colorMode == "never":
		return false
	default:
		return isTerminal(os.Stdout)
	}
}

// c wraps s in the given ANSI code when color is active.
func c(code, s string) string {
	if !colorActive() {
		return s
	}
	return code + s + ansiReset
}

// stateColor picks the color for a result's status label.
func stateColor(r result) string {
	switch {
	case r.Error != "":
		return ansiRed
	case r.Indexed:
		return ansiGreen
	default:
		return ansiYellow
	}
}

// writeReport splits results into indexed.txt / not-indexed.txt and writes a
// summary.json so you can see at a glance what Google has vs. hasn't.
func writeReport(dir string, results []result) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	idx, err := os.Create(filepath.Join(dir, "indexed.txt"))
	if err != nil {
		return err
	}
	defer idx.Close()
	nf, err := os.Create(filepath.Join(dir, "not-indexed.txt"))
	if err != nil {
		return err
	}
	defer nf.Close()

	for _, r := range results {
		if r.Error != "" || !r.Indexed {
			fmt.Fprintln(nf, r.URL)
			continue
		}
		fmt.Fprintln(idx, r.URL)
	}

	summary := buildSummary(results)
	b, _ := json.MarshalIndent(summary, "", "  ")
	return os.WriteFile(filepath.Join(dir, "summary.json"), b, 0o644)
}

// summary is the on-disk shape written to summary.json (and read by -diff).
type summary struct {
	Total          int      `json:"total"`
	Indexed        int      `json:"indexed"`
	NotIndexed     int      `json:"not_indexed"`
	IndexedURLs    []string `json:"indexed_urls"`
	NotIndexedURLs []string `json:"not_indexed_urls"`
}

func buildSummary(results []result) summary {
	s := summary{Total: len(results)}
	for _, r := range results {
		if r.Error == "" && r.Indexed {
			s.Indexed++
			s.IndexedURLs = append(s.IndexedURLs, r.URL)
		} else {
			s.NotIndexed++
			s.NotIndexedURLs = append(s.NotIndexedURLs, r.URL)
		}
	}
	return s
}

func loadSummary(path string) (summary, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return summary{}, err
	}
	var s summary
	if err := json.Unmarshal(b, &s); err != nil {
		return summary{}, err
	}
	return s, nil
}

// printDiff compares the current run against a previously saved summary and
// reports only what moved: newly indexed, newly dropped, plus unchanged counts.
func printDiff(basePath string, base summary, results []result) error {
	cur := buildSummary(results)
	wasIdx := toSet(base.IndexedURLs)
	nowIdx := toSet(cur.IndexedURLs)

	var gained, dropped, same int
	for u := range nowIdx {
		if _, ok := wasIdx[u]; ok {
			same++
		} else {
			gained++
			fmt.Printf("  %s newly indexed:    %s\n", c(ansiGreen, "▲"), u)
		}
	}
	for u := range wasIdx {
		if _, ok := nowIdx[u]; !ok {
			dropped++
			fmt.Printf("  %s dropped (was indexed, now not): %s\n", c(ansiRed, "▼"), u)
		}
	}

	fmt.Printf("\nDIFF vs %s\n", basePath)
	fmt.Printf("  indexed:   %d → %d  (%+d)\n", base.Indexed, cur.Indexed, cur.Indexed-base.Indexed)
	fmt.Printf("  not index: %d → %d  (%+d)\n", base.NotIndexed, cur.NotIndexed, cur.NotIndexed-base.NotIndexed)
	fmt.Printf("  newly indexed: %d | dropped: %d | unchanged: %d\n", gained, dropped, same)
	if gained == 0 && dropped == 0 {
		fmt.Printf("  → no change\n")
	}
	return nil
}

func toSet(urls []string) map[string]struct{} {
	m := make(map[string]struct{}, len(urls))
	for _, u := range urls {
		m[u] = struct{}{}
	}
	return m
}
