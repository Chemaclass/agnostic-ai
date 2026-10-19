package overview

import (
	"encoding/json"
	"net/http"
)

// Paths NewHandler serves.
const (
	OverviewPath = "/api/views/overview"
	FeedPath     = "/api/views/feed"
)

// NewHandler serves the overview at OverviewPath and the feed at
// FeedPath as JSON. It calls load on every request, so each response
// reflects the project as it is now. A failure answers with a JSON
// object holding "error". It sets no CORS headers: serve's origin guard
// decides which pages may call it.
func NewHandler(load func() (Inputs, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var render func(Inputs) any
		switch r.URL.Path {
		case OverviewPath:
			render = func(in Inputs) any { return Build(in) }
		case FeedPath:
			render = func(in Inputs) any { return BuildFeed(in) }
		default:
			writeJSON(w, http.StatusNotFound, errorBody{Error: "no view at " + r.URL.Path})
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead:
		default:
			w.Header().Set("Allow", "GET, HEAD")
			writeJSON(w, http.StatusMethodNotAllowed, errorBody{Error: r.Method + " is not supported here; use GET"})
			return
		}
		in, err := load()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorBody{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, render(in))
	})
}

type errorBody struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		status = http.StatusInternalServerError
		data, _ = json.Marshal(errorBody{Error: "encode view: " + err.Error()})
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(append(data, '\n'))
}
