package overview

import (
	"encoding/json"
	"net/http"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/preview"
)

// OverviewPath is the path NewHandler serves.
const OverviewPath = "/api/views/overview"

// NewHandler serves the overview at OverviewPath as JSON. It calls load on every request, so each response
// reflects the project as it is now. A failure answers with a JSON
// object holding "error". Requests share the local API origin boundary.
func NewHandler(load func() (Inputs, error)) http.Handler {
	return authority.CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != OverviewPath {
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
			writeJSON(w, http.StatusInternalServerError, errorBody{Error: preview.RedactText(err.Error())})
			return
		}
		writeJSON(w, http.StatusOK, Build(in))
	}))
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
