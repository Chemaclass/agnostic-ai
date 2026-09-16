package integration

import (
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const updatesContentDir = "../../docs/site/content/updates"

func readUpdateSource(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(updatesContentDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func assertMarkerPair(t *testing.T, body, start, end string) {
	t.Helper()
	if count := strings.Count(body, start); count != 1 {
		t.Errorf("marker %q occurs %d times, want 1", start, count)
	}
	if count := strings.Count(body, end); count != 1 {
		t.Errorf("marker %q occurs %d times, want 1", end, count)
	}
	if strings.Index(body, start) > strings.Index(body, end) {
		t.Errorf("marker %q appears after %q", start, end)
	}
}

func TestTargetUpdates_ArticlesKeepDurablePublicationMetadata(t *testing.T) {
	t.Parallel()
	articles, err := filepath.Glob(filepath.Join(updatesContentDir, "[0-9]*.md"))
	if err != nil {
		t.Fatalf("find target update articles: %v", err)
	}
	if len(articles) == 0 {
		t.Fatal("no dated target update articles found")
	}

	auditMarker := regexp.MustCompile(`audit_marker = "target-capability-audit:\d{4}-\d{2}-\d{2}:[a-z0-9,-]+:[a-f0-9]{12}"`)
	for _, path := range articles {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		article := string(data)
		for _, required := range []string{"kind = ", "rss_guid = ", "aliases = ", "archive_stats = ", "[[extra.signals]]"} {
			if !strings.Contains(article, required) {
				t.Errorf("%s is missing %q", filepath.Base(path), required)
			}
		}
		if strings.Contains(article, `kind = "audit"`) {
			if count := len(auditMarker.FindAllString(article, -1)); count != 1 {
				t.Errorf("%s has %d audit markers, want 1", filepath.Base(path), count)
			}
		} else if strings.Contains(article, `kind = "release"`) {
			for _, forbidden := range []string{"audit_marker = ", "report_digest = ", "targets_checked = ", "finding_count = ", "clean_count = "} {
				if strings.Contains(article, forbidden) {
					t.Errorf("%s release metadata contains audit-only field %q", filepath.Base(path), forbidden)
				}
			}
		} else {
			t.Errorf("%s has an unknown update kind", filepath.Base(path))
		}
	}
}

func TestTargetUpdates_CriticalSignalsCarryDurableEvidence(t *testing.T) {
	t.Parallel()
	article := readUpdateSource(t, "2026-09-15.md")
	for _, signal := range []string{
		"cap-manual-only-skill-invocation",
		"cap-agent-mcp-scoping",
		"cap-directory-scoped-skills",
		"cap-project-extension-bundles",
	} {
		assertMarkerPair(t, article,
			"<!-- target-capability:"+signal+":start -->",
			"<!-- target-capability:"+signal+":end -->")
	}
	for _, required := range []string{
		"Vendor evidence.",
		"Repository evidence.",
		"Where agnostic-ai is now",
		"What happens next",
		"observations, not shipped support",
		"blob/8b4832a9494914bbfc0a13d7b0c8ae7aed6f5eb6/",
	} {
		if !strings.Contains(article, required) {
			t.Errorf("2026-09-15.md is missing %q", required)
		}
	}
}

func TestTargetUpdates_PostAloneUpdatesSiteOutputs(t *testing.T) {
	zolaPath, err := exec.LookPath("zola")
	if err != nil {
		t.Skip("zola is not installed")
	}

	siteDir := t.TempDir()
	if err := os.CopyFS(siteDir, os.DirFS("../../docs/site")); err != nil {
		t.Fatalf("copy site fixture: %v", err)
	}

	secondPost := `+++
title = "agnostic-ai v0.59.0: A temporary release briefing"
description = "A fixture proving that one release post updates every publishing surface."
date = 2026-09-22T00:00:00+02:00
slug = "2026-09-22-v0.59.0"
aliases = ["updates/2026-09-22-v0.59.0.html"]

[extra]
kind = "release"
version = "v0.59.0"
dek = "This release article exists only inside the site build test."
rss_guid = "https://chemaclass.github.io/agnostic-ai/updates/2026-09-22-v0.59.0.html"
archive_stats = "1 shipped change · 1 upstream note"

[[extra.signals]]
status = "shipped"
targets = ["agnostic-ai"]
title = "Release metadata drives the latest edition"
summary = "No archive, feed, or template file changed."
+++

## Shipped in agnostic-ai v0.59.0

Only this Markdown file was added.

## Upstream CLI and model news

No verified upstream change met the publication bar.
`
	postPath := filepath.Join(siteDir, "content", "updates", "2026-09-22-v0.59.0.md")
	if err := os.WriteFile(postPath, []byte(secondPost), 0o600); err != nil {
		t.Fatalf("write second post fixture: %v", err)
	}

	outputDir := filepath.Join(t.TempDir(), "public")
	command := exec.Command(zolaPath, "--root", siteDir, "build", "--output-dir", outputDir)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build Zola fixture: %v\n%s", err, output)
	}

	archive := readBuiltFile(t, filepath.Join(outputDir, "updates", "index.html"))
	feed := readBuiltFile(t, filepath.Join(outputDir, "updates", "feed.xml"))
	article := readBuiltFile(t, filepath.Join(outputDir, "updates", "2026-09-22-v0.59.0", "index.html"))
	alias := readBuiltFile(t, filepath.Join(outputDir, "updates", "2026-09-22-v0.59.0.html"))
	home := readBuiltFile(t, filepath.Join(outputDir, "index.html"))

	if !strings.Contains(archive, "A temporary release briefing") || !strings.Contains(archive, "Release metadata drives the latest edition") {
		t.Error("second post did not become the archive's latest edition")
	}
	if !strings.Contains(feed, "A temporary release briefing") || !strings.Contains(feed, "https://chemaclass.github.io/agnostic-ai/updates/2026-09-22-v0.59.0.html") {
		t.Error("second post did not reach the RSS feed with its stable GUID")
	}
	if !strings.Contains(feed, "https://chemaclass.github.io/agnostic-ai/updates/2026-09-15.html") {
		t.Error("the existing RSS GUID changed during the post-only build")
	}
	if !strings.Contains(article, "Release briefing") || !strings.Contains(article, "v0.59.0 release briefing") {
		t.Error("the generated article is missing its release identity")
	}
	if strings.Contains(article, "target-capability-audit:") || strings.Contains(article, "Audit summary") {
		t.Error("the generated release article includes audit-only rendering")
	}
	if !strings.Contains(alias, "url=https://chemaclass.github.io/agnostic-ai/updates/2026-09-22-v0.59.0/") {
		t.Error("the .html compatibility alias does not redirect to the canonical article")
	}
	for name, body := range map[string]string{"home": home, "archive": archive, "article": article} {
		for _, label := range []string{"Home", "Updates", "Playground", "Docs", "GitHub"} {
			if !strings.Contains(body, ">"+label+"</a>") {
				t.Errorf("%s navigation is missing %s", name, label)
			}
		}
	}

	var rss struct {
		Channel struct {
			Items []struct {
				Title string `xml:"title"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal([]byte(feed), &rss); err != nil {
		t.Fatalf("feed.xml is not valid XML: %v", err)
	}
	if len(rss.Channel.Items) != 2 || rss.Channel.Items[0].Title != "agnostic-ai v0.59.0: A temporary release briefing" {
		t.Errorf("feed order = %+v, want the temporary post first", rss.Channel.Items)
	}
}

func TestTargetUpdates_SitemapUsesCanonicalContentRoutes(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "sitemap.xml")
	command := exec.Command("bash", "./scripts/build-site-sitemap.sh", outputPath)
	command.Dir = "../.."
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build sitemap: %v\n%s", err, output)
	}

	sitemap := readBuiltFile(t, outputPath)
	for _, route := range []string{
		"https://chemaclass.github.io/agnostic-ai/",
		"https://chemaclass.github.io/agnostic-ai/playground/",
		"https://chemaclass.github.io/agnostic-ai/updates/",
		"https://chemaclass.github.io/agnostic-ai/updates/2026-09-15/",
	} {
		if !strings.Contains(sitemap, "<loc>"+route+"</loc>") {
			t.Errorf("sitemap is missing %s", route)
		}
	}
	if strings.Contains(sitemap, "2026-09-15.html</loc>") {
		t.Error("sitemap advertises the compatibility alias instead of the canonical article")
	}

	var urlset struct {
		URLs []struct {
			Location string `xml:"loc"`
			Lastmod  string `xml:"lastmod"`
		} `xml:"url"`
	}
	if err := xml.Unmarshal([]byte(sitemap), &urlset); err != nil {
		t.Fatalf("sitemap.xml is not valid XML: %v", err)
	}
	for _, entry := range urlset.URLs {
		if entry.Lastmod == "" {
			t.Errorf("%s has no git-derived lastmod", entry.Location)
		}
	}
}

func readBuiltFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read built file %s: %v", path, err)
	}
	return string(data)
}
