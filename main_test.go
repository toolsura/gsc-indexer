package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockResp builds an *http.Response with the given status and body for tests.
func mockResp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestCollectURLs(t *testing.T) {
	t.Run("batch file dedupes and skips comments", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "urls.txt")
		if err := os.WriteFile(f, []byte("# comment\nhttps://a.example/\nhttps://a.example/\nhttps://b.example/\n\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := collectURLs(f, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"https://a.example/", "https://b.example/"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("got %v want %v", got, want)
		}
	})

	t.Run("positional args merged with batch", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "urls.txt")
		_ = os.WriteFile(f, []byte("https://a.example/\n"), 0o644)
		got, err := collectURLs(f, []string{"https://c.example/", "https://a.example/"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// a appears once (deduped), c once, in batch-then-args order.
		want := "https://a.example/,https://c.example/"
		if strings.Join(got, ",") != want {
			t.Fatalf("got %v want %v", got, want)
		}
	})

	t.Run("empty input returns empty slice", func(t *testing.T) {
		got, err := collectURLs("", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected empty, got %v", got)
		}
	})

	t.Run("missing batch file errors", func(t *testing.T) {
		_, err := collectURLs("/no/such/file.txt", nil)
		if err == nil {
			t.Fatal("expected error for missing batch file")
		}
	})
}

func TestValidateURLs(t *testing.T) {
	t.Run("clean list passes", func(t *testing.T) {
		if err := validateURLs([]string{"https://a.example/", "https://b.example/"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("misplaced flag is rejected with clear message", func(t *testing.T) {
		err := validateURLs([]string{"https://a.example/", "-color"})
		if err == nil {
			t.Fatal("expected error for misplaced -color flag")
		}
		if !strings.Contains(err.Error(), "flags must come before URLs") {
			t.Fatalf("error not helpful: %v", err)
		}
	})

	t.Run("any leading-dash argument is rejected", func(t *testing.T) {
		if err := validateURLs([]string{"-q"}); err == nil {
			t.Fatal("expected error for leading-dash argument")
		}
	})

	t.Run("empty list passes", func(t *testing.T) {
		if err := validateURLs(nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestExpandURLs(t *testing.T) {
	orig := sitemapFetch
	defer func() { sitemapFetch = orig }()

	t.Run("non-sitemap URLs pass through", func(t *testing.T) {
		got, err := expandURLs([]string{"https://a.example/", "https://b.example/page"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 pass-through URLs, got %v", got)
		}
	})

	t.Run("sitemap expanded into loc URLs", func(t *testing.T) {
		sitemapFetch = func(u string) (*http.Response, error) {
			if u != "https://a.example/sitemap.xml" {
				t.Fatalf("unexpected sitemap url %s", u)
			}
			body := `<urlset><url><loc>https://a.example/p1</loc></url><url><loc>https://a.example/p2</loc></url></urlset>`
			return mockResp(200, body), nil
		}
		got, err := expandURLs([]string{"https://a.example/sitemap.xml"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "https://a.example/p1,https://a.example/p2"
		if strings.Join(got, ",") != want {
			t.Fatalf("got %v want %v", got, want)
		}
	})

	t.Run("sitemap index expands child sitemaps recursively", func(t *testing.T) {
		sitemapFetch = func(u string) (*http.Response, error) {
			switch u {
			case "https://a.example/index.xml":
				return mockResp(200, `<sitemapindex><sitemap><loc>https://a.example/s1.xml</loc></sitemap></sitemapindex>`), nil
			case "https://a.example/s1.xml":
				return mockResp(200, `<urlset><url><loc>https://a.example/p1</loc></url></urlset>`), nil
			default:
				t.Fatalf("unexpected sitemap url %s", u)
				return nil, fmt.Errorf("unexpected sitemap url %s", u)
			}
		}
		got, err := expandURLs([]string{"https://a.example/index.xml"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Join(got, ",") != "https://a.example/p1" {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("sitemap fetch error propagates", func(t *testing.T) {
		sitemapFetch = func(u string) (*http.Response, error) {
			return mockResp(404, ""), nil
		}
		_, err := expandURLs([]string{"https://a.example/sitemap.xml"})
		if err == nil {
			t.Fatal("expected error on HTTP 404 sitemap")
		}
	})

	t.Run("refuses to expand beyond maxExpand", func(t *testing.T) {
		sitemapFetch = func(u string) (*http.Response, error) {
			return mockResp(200, `<urlset><url><loc>https://a.example/p</loc></url></urlset>`), nil
		}
		// craft a raw input that expands past the cap
		huge := make([]string, maxExpand+5)
		for i := range huge {
			huge[i] = "https://a.example/sitemap.xml"
		}
		_, err := expandURLs(huge)
		if err == nil {
			t.Fatal("expected maxExpand guard to trip")
		}
	})
}

func TestParseInspection(t *testing.T) {
	t.Run("verdict PASS and not excluded is indexed", func(t *testing.T) {
		raw := []byte(`{"inspectionResult":{"indexStatusResult":{"verdict":"PASS","coverageState":"Submitted and indexed","lastCrawlTime":"2026-01-01T00:00:00Z"}}}`)
		r := parseInspection("https://a.example/", raw)
		if r.Error != "" {
			t.Fatalf("unexpected error: %v", r.Error)
		}
		if !r.Indexed {
			t.Fatal("expected indexed=true")
		}
		if r.LastCrawl != "2026-01-01T00:00:00Z" {
			t.Fatalf("last crawl not parsed: %q", r.LastCrawl)
		}
	})

	t.Run("excluded coverage is not indexed", func(t *testing.T) {
		raw := []byte(`{"inspectionResult":{"indexStatusResult":{"verdict":"PASS","coverageState":"Excluded by meta tag"}}}`)
		r := parseInspection("https://a.example/", raw)
		if r.Indexed {
			t.Fatal("excluded URL must not be indexed")
		}
	})

	t.Run("non-PASS verdict is not indexed", func(t *testing.T) {
		raw := []byte(`{"inspectionResult":{"indexStatusResult":{"verdict":"NEUTRAL","coverageState":"URL is unknown to Google"}}}`)
		r := parseInspection("https://a.example/", raw)
		if r.Indexed {
			t.Fatal("non-PASS verdict must not be indexed")
		}
	})

	t.Run("malformed JSON records parse error", func(t *testing.T) {
		r := parseInspection("https://a.example/", []byte("not json"))
		if r.Error == "" {
			t.Fatal("expected parse error")
		}
	})
}

func TestBuildAndLoadSummary(t *testing.T) {
	results := []result{
		{URL: "https://a.example/", Indexed: true},
		{URL: "https://b.example/", Indexed: false},
		{URL: "https://c.example/", Error: "boom"},
	}
	s := buildSummary(results)
	if s.Total != 3 || s.Indexed != 1 || s.NotIndexed != 2 {
		t.Fatalf("summary counts wrong: %+v", s)
	}
	if len(s.IndexedURLs) != 1 || s.IndexedURLs[0] != "https://a.example/" {
		t.Fatalf("indexed urls wrong: %v", s.IndexedURLs)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "summary.json")
	b, _ := json.MarshalIndent(s, "", "  ")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadSummary(path)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if loaded.Total != s.Total || loaded.Indexed != s.Indexed {
		t.Fatalf("round-trip mismatch: %+v vs %+v", loaded, s)
	}
}

func TestPrintDiff(t *testing.T) {
	base := summary{
		IndexedURLs:    []string{"https://a.example/", "https://b.example/"},
		NotIndexedURLs: []string{"https://c.example/"},
	}
	cur := []result{
		{URL: "https://a.example/", Indexed: true}, // unchanged
		{URL: "https://b.example/"},                // dropped
		{URL: "https://d.example/", Indexed: true}, // newly indexed
	}
	var buf bytes.Buffer
	// printDiff writes to stdout; capture by redirecting temporarily.
	captureStdout(&buf, func() {
		_ = printDiff("base.json", base, cur)
	})
	out := buf.String()
	if !strings.Contains(out, "newly indexed") || !strings.Contains(out, "dropped") {
		t.Fatalf("diff output missing changes:\n%s", out)
	}
	if !strings.Contains(out, "https://d.example/") || !strings.Contains(out, "https://b.example/") {
		t.Fatalf("diff output missing URLs:\n%s", out)
	}
}

// captureStdout runs fn while capturing os.Stdout into buf, then restores it.
// (printDiff uses fmt.Printf → os.Stdout.)
func captureStdout(buf *bytes.Buffer, fn func()) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	_, _ = buf.ReadFrom(r)
}
