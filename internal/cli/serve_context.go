package cli

import (
	"net/http"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/preview"
)

const contextViewPath = "/api/views/context"

type contextView struct {
	SchemaVersion string                `json:"schemaVersion"`
	Supported     bool                  `json:"supported"`
	Reason        string                `json:"reason,omitempty"`
	Root          string                `json:"root,omitempty"`
	Targets       []string              `json:"targets"`
	Target        string                `json:"target"`
	Report        *explainContextOutput `json:"report,omitempty"`
}

// newContextHandler serves the explain --context report for one target.
// It reads the project on every request, so a refresh sees edited specs.
func newContextHandler(root string, global bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeEditJSON(w, http.StatusMethodNotAllowed, editError{Error: r.Method + " is not supported here; use GET"})
			return
		}
		view := contextView{SchemaVersion: "1", Targets: []string{}}
		if global {
			view.Reason = "Context size is reported for a project. Open a project to see it."
			writeEditJSON(w, http.StatusOK, view)
			return
		}
		cfg, b, err := loadProject(".")
		if err != nil {
			writeEditJSON(w, http.StatusInternalServerError, editError{Error: preview.RedactText(err.Error())})
			return
		}
		view.Target = r.URL.Query().Get("target")
		if view.Target == "" && len(cfg.Targets) > 0 {
			view.Target = cfg.Targets[0]
		}
		report, err := explainContext(cfg, b, view.Target, r.URL.Query().Get("file"))
		if err != nil {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: preview.RedactText(err.Error())})
			return
		}
		view.Supported, view.Report, view.Targets = true, &report, append(view.Targets, cfg.Targets...)
		if abs, err := filepath.Abs(root); err == nil {
			view.Root = abs
		}
		writeEditJSON(w, http.StatusOK, view)
	})
}
