package integration

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const updatesDir = "../../docs/site/updates"

func readUpdateFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(updatesDir, name)
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

func TestTargetUpdates_ArchiveAndFeedKeepManagedEnvelopes(t *testing.T) {
	t.Parallel()
	index := readUpdateFile(t, "index.html")
	feed := readUpdateFile(t, "feed.xml")
	assertMarkerPair(t, index, "<!-- target-updates:index:start -->", "<!-- target-updates:index:end -->")
	assertMarkerPair(t, feed, "<!-- target-updates:feed:start -->", "<!-- target-updates:feed:end -->")

	var rss struct {
		Channel struct {
			Items []struct {
				Link string `xml:"link"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal([]byte(feed), &rss); err != nil {
		t.Errorf("feed.xml is not valid XML: %v", err)
	}
}

func TestTargetUpdates_EveryArticleIsDiscoverable(t *testing.T) {
	t.Parallel()
	index := readUpdateFile(t, "index.html")
	feed := readUpdateFile(t, "feed.xml")
	articles, err := filepath.Glob(filepath.Join(updatesDir, "[0-9]*.html"))
	if err != nil {
		t.Fatalf("find target update articles: %v", err)
	}
	if len(articles) == 0 {
		t.Error("no dated target update articles found")
	}
	for _, path := range articles {
		name := filepath.Base(path)
		if !strings.Contains(index, `href="`+name+`"`) {
			t.Errorf("%s is missing from the update archive", name)
		}
		if !strings.Contains(feed, "/updates/"+name) {
			t.Errorf("%s is missing from the RSS feed", name)
		}
		article := readUpdateFile(t, name)
		marker := regexp.MustCompile(`<!-- target-capability-audit:\d{4}-\d{2}-\d{2}:[a-z0-9,-]+:[a-f0-9]{12} -->`)
		if count := len(marker.FindAllString(article, -1)); count != 1 {
			t.Errorf("%s has %d audit markers, want 1", name, count)
		}
	}
}

func TestTargetUpdates_CriticalSignalsCarryDurableEvidence(t *testing.T) {
	t.Parallel()
	article := readUpdateFile(t, "2026-09-15.html")
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
			t.Errorf("2026-09-15.html is missing %q", required)
		}
	}
}
