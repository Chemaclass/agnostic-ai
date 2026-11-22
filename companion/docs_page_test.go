package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func postDocs(s *shell, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.routes(&inbox{}, fstest.MapFS{}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/docs", strings.NewReader(body)))
	return rec
}

func TestDocsPage_OpensADocsSection(t *testing.T) {
	s, _, _, _ := menuShell(t)
	var opened []string
	s.openPage = func(url string) error { opened = append(opened, url); return nil }

	if rec := postDocs(s, `{"path":"/docs/mailbox/#hand-off-files"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("POST = %d %s", rec.Code, rec.Body)
	}
	if want := "https://agnostic-ai.org/docs/mailbox/#hand-off-files"; len(opened) != 1 || opened[0] != want {
		t.Errorf("opened %q; want %q", opened, want)
	}
}

func TestDocsPage_RefusesAPageOutsideTheDocs(t *testing.T) {
	s, _, _, _ := menuShell(t)
	s.openPage = func(url string) error { t.Errorf("opened %s", url); return nil }

	for _, body := range []string{``, `{"path":"https://example.com/docs/"}`, `{"path":"/docs/../x/"}`, `{"path":"//example.com/docs/"}`, `{"path":"/blog/"}`} {
		if rec := postDocs(s, body); rec.Code != http.StatusBadRequest {
			t.Errorf("POST %s = %d; want 400", body, rec.Code)
		}
	}
}

func TestDocsPage_SaysWhenTheBrowserDidNotOpen(t *testing.T) {
	s, _, _, _ := menuShell(t)
	s.openPage = func(string) error { return errors.New("no browser") }

	if rec := postDocs(s, `{"path":"/docs/mailbox/"}`); rec.Code != http.StatusInternalServerError {
		t.Errorf("POST = %d; want 500", rec.Code)
	}
}
