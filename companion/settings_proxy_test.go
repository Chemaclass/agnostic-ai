package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestSettingsProxy_PassesTheTagAndTheWrites(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case serverSettingsViewPath:
			w.Header().Set("ETag", `"v1"`)
			if r.Header.Get("If-None-Match") == `"v1"` {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"schemaVersion": "settings/v1"})
		case serverSettingsPath:
			if r.Header.Get(secretHeader) == "" {
				t.Error("a settings write went without the companion secret")
			}
			writeJSON(w, http.StatusOK, map[string]any{"changed": []string{}})
		case serverSettingsSyncPath:
			time.Sleep(150 * time.Millisecond)
			writeJSON(w, http.StatusOK, map[string]string{"id": "x"})
		case serverListPath:
			w.Header().Set("ETag", `"list"`)
			writeJSON(w, http.StatusOK, map[string]any{"needsYou": 0})
		default:
			t.Errorf("unexpected upstream path %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	server := newServer()
	server.base, server.health.Version = upstream.URL, "1.2.3"
	// A sync outlasts a call: it gets the send wait.
	server.callWait, server.sendWait = 100*time.Millisecond, 5*time.Second
	in := newInbox(server, "1.2.3", func() {})
	routes := (&shell{}).routes(in, fstest.MapFS{})

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("ETag") != `"v1"` {
		t.Fatalf("settings = %d %q %s", rec.Code, rec.Header().Get("ETag"), rec.Body)
	}
	held := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	held.Header.Set("If-None-Match", `"v1"`)
	rec = httptest.NewRecorder()
	routes.ServeHTTP(rec, held)
	if rec.Code != http.StatusNotModified {
		t.Errorf("settings with the tag = %d", rec.Code)
	}
	for _, path := range []string{"/api/settings", "/api/settings/sync"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s = %d %s", path, rec.Code, rec.Body)
		}
	}
}

func TestSettingsProxy_RefusesOtherPages(t *testing.T) {
	in := newInbox(newServer(), "1.2.3", func() {})
	routes := (&shell{}).routes(in, fstest.MapFS{})
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/settings", nil),
		httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(`{}`)),
		httptest.NewRequest(http.MethodPost, "/api/settings/sync", strings.NewReader(`{}`)),
	} {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://example.com")
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s from another page = %d", req.Method, req.URL.Path, rec.Code)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("a write without JSON = %d", rec.Code)
	}
}
