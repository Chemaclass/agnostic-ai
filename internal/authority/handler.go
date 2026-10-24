package authority

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Loader reads the configuration and specs. The handler calls it on
// every request, so an edited spec shows on the next refresh.
type Loader func() (*config.Config, spec.Bundle, error)

// HandlerOptions configures NewHandler.
type HandlerOptions struct {
	// Root is the project root, as Options.Root.
	Root string
	// UIDir, when set, serves the built client from this directory for
	// paths outside /api. Client routes fall back to index.html; missing
	// assets return 404.
	UIDir string
	// UIFiles serves an embedded client when UIDir is unset.
	UIFiles fs.FS
	// ConfigFiles lists the config files read, relative to Root.
	ConfigFiles func() []string
	// Installed compares the generated files of one target with disk.
	// Optional; without it every view says installed is not checked.
	Installed func(target string, cfg *config.Config, b spec.Bundle) (InstalledState, error)
	// Now stamps each view; nil means the current time.
	Now func() time.Time
	// Global serves the user-level configuration, as Options.Global.
	Global         bool
	AllowedOrigins []string
}

// Endpoint paths.
const (
	pathView   = "/api/views/authority"
	pathImpact = "/api/views/authority/impact"
	pathSchema = "/api/views/authority/schema"
	pathHealth = "/api/health"
)

// maxProposalBytes bounds a proposal body.
const maxProposalBytes = 1 << 20

type handler struct {
	load Loader
	opts HandlerOptions
	// mu serializes requests: loading specs and comparing generated
	// files touch process-wide state.
	mu sync.Mutex
}

// endpoint documents one route for the schema endpoint.
type endpoint struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

var endpoints = []endpoint{
	{Method: http.MethodGet, Path: pathView + "?agentId=<id>&target=<tool>", Description: "The Authority view for one subject on one coding tool."},
	{Method: http.MethodPost, Path: pathImpact, Description: "The impact of a proposed permission change, computed on a copy of the specs; nothing is written."},
	{Method: http.MethodGet, Path: pathSchema, Description: "The schema version and these endpoints."},
	{Method: http.MethodGet, Path: pathHealth, Description: "Whether the configuration loads, with its current revision."},
}

// NewHandler serves the Authority view as JSON, and the built client
// when HandlerOptions.UIDir or UIFiles is set.
func NewHandler(load Loader, opts HandlerOptions) http.Handler {
	h := &handler{load: load, opts: opts}
	mux := http.NewServeMux()
	mux.HandleFunc(pathView, h.view)
	mux.HandleFunc(pathImpact, h.impact)
	mux.HandleFunc(pathSchema, h.schema)
	mux.HandleFunc(pathHealth, h.health)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, fmt.Errorf("no endpoint at %s", r.URL.Path))
	})
	mux.HandleFunc("/", h.ui)
	return CORS(mux, opts.AllowedOrigins...)
}

func (h *handler) view(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet) {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	cfg, b, err := h.load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	q := r.URL.Query()
	v, err := Build(cfg, b, h.options(cfg, b, q.Get("agentId"), q.Get("target")))
	if err != nil {
		writeError(w, statusOf(err), err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *handler) impact(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodPost) {
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, errors.New("use Content-Type application/json"))
		return
	}
	var p Proposal
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxProposalBytes))
	if err := dec.Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid proposal: %w", err))
		return
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		writeError(w, http.StatusBadRequest, errors.New("proposal must contain one JSON object"))
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	cfg, b, err := h.load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	res, err := Impact(cfg, b, h.options(cfg, b, p.AgentID, p.Target), p)
	if err != nil {
		writeError(w, statusOf(err), err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *handler) schema(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, struct {
		SchemaVersion string     `json:"schemaVersion"`
		Endpoints     []endpoint `json:"endpoints"`
	}{SchemaVersion: SchemaVersion, Endpoints: endpoints})
}

func (h *handler) health(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet) {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_, b, err := h.load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		OK       bool   `json:"ok"`
		Revision string `json:"revision"`
	}{OK: true, Revision: Revision(b, Options{Root: h.opts.Root, ConfigFiles: h.configFiles()})})
}

func (h *handler) ui(w http.ResponseWriter, r *http.Request) {
	files := h.opts.UIFiles
	if h.opts.UIDir != "" {
		files = os.DirFS(h.opts.UIDir)
	}
	if files == nil {
		writeError(w, http.StatusNotFound, errors.New("no client is served here; the API lives under /api"))
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, errors.New("use GET or HEAD"))
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	if info, err := fs.Stat(files, name); err == nil && !info.IsDir() {
		http.FileServer(http.FS(files)).ServeHTTP(w, r)
		return
	}
	if path.Ext(name) != "" || name == "assets" || strings.HasPrefix(name, "assets/") {
		http.NotFound(w, r)
		return
	}
	index, err := fs.ReadFile(files, "index.html")
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("client index.html is missing"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
}

// options builds the view options for one request.
func (h *handler) options(cfg *config.Config, b spec.Bundle, agentID, target string) Options {
	opts := Options{
		Root:        h.opts.Root,
		AgentID:     agentID,
		Target:      target,
		FileMatches: !h.opts.Global,
		ConfigFiles: h.configFiles(),
		Global:      h.opts.Global,
	}
	if h.opts.Now != nil {
		opts.Now = h.opts.Now()
	}
	if h.opts.Installed == nil {
		return opts
	}
	resolved, err := resolveTarget(cfg, target)
	if err != nil {
		return opts
	}
	if state, err := h.opts.Installed(resolved, cfg, b); err == nil {
		opts.Installed = map[string]InstalledState{resolved: state}
	}
	return opts
}

func (h *handler) configFiles() []string {
	if h.opts.ConfigFiles == nil {
		return nil
	}
	return h.opts.ConfigFiles()
}

// statusOf maps an error to its HTTP status: an unknown agent or target
// is not found, anything else a server failure.
func statusOf(err error) int {
	var notFound *NotFoundError
	if errors.As(err, &notFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

func allowMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method %s is not allowed here; use %s", r.Method, method))
	return false
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: preview.RedactText(err.Error())})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
