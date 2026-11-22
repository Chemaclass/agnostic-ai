package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
)

// docsURL is the project's website; a help link names a page under /docs/.
const docsURL = "https://agnostic-ai.org"

var docsPath = regexp.MustCompile(`^/docs/[a-z0-9/-]*/(#[a-z0-9-]+)?$`)

// openDocsPage answers a help link: POST /api/docs {path} opens that docs
// page in the default browser. Only a page of the docs site opens.
func (s *shell) openDocsPage(w http.ResponseWriter, r *http.Request) {
	var asked struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes)).Decode(&asked); err != nil || !docsPath.MatchString(asked.Path) {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "send {path} of a page under /docs/"})
		return
	}
	page := docsURL + asked.Path
	if err := s.openPage(page); err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: fmt.Sprintf("open %s: %v", page, err)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
