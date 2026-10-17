package overview

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// Paths NewHandler serves.
const (
	OverviewPath = "/api/views/overview"
	FeedPath     = "/api/views/feed"
)

// NewHandler serves the overview at OverviewPath and the feed at
// FeedPath as JSON. It calls load on every request, so each response
// reflects the project as it is now. A failure answers with a JSON
// object holding "error". Pages served from http://localhost or
// http://127.0.0.1, on any port, may read the responses.
func NewHandler(load func() (Inputs, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowLocalOrigin(w.Header(), r.Header.Get("Origin"))
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
		case http.MethodOptions:
			w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			w.Header().Set("Allow", "GET, HEAD, OPTIONS")
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

// allowLocalOrigin lets a page from a local origin read the response.
// Any other origin gets no CORS header, so a browser keeps its pages
// from reading the views.
func allowLocalOrigin(h http.Header, origin string) {
	h.Add("Vary", "Origin")
	if localOrigin(origin) {
		h.Set("Access-Control-Allow-Origin", origin)
	}
}

// localOrigin reports whether origin is http://localhost or
// http://127.0.0.1, with or without a port.
func localOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if host := u.Hostname(); host != "localhost" && host != "127.0.0.1" {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		return err == nil && n > 0 && n <= 65535
	}
	return u.Host == u.Hostname()
}
