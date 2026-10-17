package overview

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

func serve(t *testing.T, h http.Handler, method, path, origin string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func testInputs() (Inputs, error) {
	return Inputs{
		Cfg:      &config.Config{Targets: []string{"claude"}},
		Bundle:   handcraftedBundle(),
		Snapshot: authority.Snapshot{Revision: "0123456789ab", Project: authority.Project{Name: "demo"}},
		Findings: []Finding{{Source: SourceDrift, Target: "claude", Path: "CLAUDE.md", Message: "stale"}},
	}, nil
}

func TestNewHandler_ServesTheOverviewAndTheFeedWithLocalCORS(t *testing.T) {
	h := NewHandler(testInputs)

	rec := serve(t, h, http.MethodGet, OverviewPath, "http://localhost:5173")
	if rec.Code != http.StatusOK {
		t.Fatalf("overview status = %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("content type = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("allow origin = %q", got)
	}
	var view View
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	if view.SchemaVersion != SchemaVersion || len(view.Sections) != 6 || len(view.Feed) != 1 || view.Snapshot.Revision != "0123456789ab" {
		t.Errorf("overview = %+v", view)
	}

	rec = serve(t, h, http.MethodGet, FeedPath, "http://127.0.0.1:8080")
	if rec.Code != http.StatusOK {
		t.Fatalf("feed status = %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:8080" {
		t.Errorf("allow origin = %q", got)
	}
	var feed FeedView
	if err := json.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("decode feed: %v", err)
	}
	if feed.SchemaVersion != SchemaVersion || len(feed.Feed) != 1 || feed.Feed[0].Revision != "0123456789ab" {
		t.Errorf("feed = %+v", feed)
	}
}

func TestNewHandler_GivesOtherOriginsNoCORSHeader(t *testing.T) {
	h := NewHandler(testInputs)
	for _, origin := range []string{"http://example.com", "https://localhost:5173", "http://localhost.example.com", "http://127.0.0.2:80", "null", ""} {
		rec := serve(t, h, http.MethodGet, FeedPath, origin)
		if rec.Code != http.StatusOK {
			t.Errorf("%q status = %d", origin, rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("%q allow origin = %q, want none", origin, got)
		}
	}
	if got := serve(t, h, http.MethodGet, FeedPath, "http://localhost").Header().Get("Access-Control-Allow-Origin"); got != "http://localhost" {
		t.Errorf("portless localhost allow origin = %q", got)
	}
}

func TestNewHandler_AnswersPreflightWithoutLoading(t *testing.T) {
	loads := 0
	h := NewHandler(func() (Inputs, error) {
		loads++
		return testInputs()
	})
	rec := serve(t, h, http.MethodOptions, OverviewPath, "http://localhost:3000")
	if rec.Code != http.StatusNoContent || loads != 0 {
		t.Errorf("preflight status = %d, loads = %d", rec.Code, loads)
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" || rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Errorf("preflight headers = %v", rec.Header())
	}
}

func errorOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body, err)
	}
	return body["error"]
}

func TestNewHandler_ReportsFailuresAsJSONErrors(t *testing.T) {
	failing := NewHandler(func() (Inputs, error) { return Inputs{}, errors.New("agnostic-ai.yaml: version missing") })
	rec := serve(t, failing, http.MethodGet, OverviewPath, "")
	if rec.Code != http.StatusInternalServerError || errorOf(t, rec) != "agnostic-ai.yaml: version missing" {
		t.Errorf("load failure = %d %s", rec.Code, rec.Body)
	}

	h := NewHandler(testInputs)
	rec = serve(t, h, http.MethodPost, FeedPath, "")
	if rec.Code != http.StatusMethodNotAllowed || errorOf(t, rec) == "" || rec.Header().Get("Allow") == "" {
		t.Errorf("post = %d %s", rec.Code, rec.Body)
	}
	rec = serve(t, h, http.MethodGet, "/api/views/unknown", "")
	if rec.Code != http.StatusNotFound || errorOf(t, rec) == "" {
		t.Errorf("unknown path = %d %s", rec.Code, rec.Body)
	}
}
