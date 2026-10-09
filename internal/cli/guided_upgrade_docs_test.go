package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchReleaseDocs_ShowsEverySkippedReleaseUpgradeSection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases":
			_, _ = fmt.Fprint(w, `[{"tag_name":"v0.82.0"},{"tag_name":"v0.81.0"},{"tag_name":"v0.80.0"}]`)
		case "/tree":
			_, _ = fmt.Fprint(w, `{"tree":[{"path":"docs/site/content/updates/2026-10-06-v0.80.0.md"},{"path":"docs/site/content/updates/2026-10-07-v0.81.0.md"},{"path":"docs/site/content/updates/2026-10-09-v0.82.0.md"}]}`)
		case "/docs/site/content/updates/2026-10-07-v0.81.0.md":
			_, _ = fmt.Fprint(w, "+++\ntitle=\"test\"\n+++\n## Upgrading from v0.80.0\nRename invalid Claude agent names by hand.\nRun agnostic-ai migrate --only secrets.\n## What changed\nOther content\n")
		case "/docs/site/content/updates/2026-10-09-v0.82.0.md":
			_, _ = fmt.Fprint(w, "## Upgrading from v0.81.0\nKeep your requires range if you use one.\n## Features\nFeatures\n")
		default:
			t.Errorf("unexpected fetch %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	guidance, missing := fetchReleaseDocsFrom(ctx, server.Client(), server.URL+"/tree", server.URL+"/releases", server.URL+"/", "0.80.0", "0.82.0")
	if missing != "" {
		t.Errorf("missing %s", missing)
	}
	for _, want := range []string{"Rename invalid Claude agent names by hand", "migrate --only secrets", "Keep your requires range", "2026-10-07-v0.81.0/", "2026-10-09-v0.82.0/"} {
		if !strings.Contains(guidance, want) {
			t.Errorf("missing %s in %s", want, guidance)
		}
	}
	for _, no := range []string{"Other content", "Features", "Release 0.80.0"} {
		if strings.Contains(guidance, no) {
			t.Errorf("unrelated content %s", guidance)
		}
	}
}

func TestFetchReleaseDocs_ReportsMissingGuidanceBeforePrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases" {
			_, _ = fmt.Fprint(w, `[{"tag_name":"v0.82.0"},{"tag_name":"v0.81.0"}]`)
			return
		}
		if r.URL.Path == "/tree" {
			_, _ = fmt.Fprint(w, `{"tree":[{"path":"docs/site/content/updates/2026-10-09-v0.82.0.md"}]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	guidance, missing := fetchReleaseDocsFrom(context.Background(), server.Client(), server.URL+"/tree", server.URL+"/releases", server.URL+"/", "0.81.0", "0.82.0")
	if !strings.Contains(missing, "0.82.0") || !strings.Contains(guidance, "Docs unavailable") {
		t.Errorf("guidance %s; missing %s", guidance, missing)
	}
}
