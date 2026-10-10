package lsp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
)

// DiagnosticSeverity mirrors the LSP DiagnosticSeverity enum.
type DiagnosticSeverity int

const (
	SeverityError   DiagnosticSeverity = 1
	SeverityWarning DiagnosticSeverity = 2
)

// Position is a zero-based line/character offset.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is a half-open [start, end) span within a document.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Diagnostic is one issue reported for a document.
type Diagnostic struct {
	Range    Range              `json:"range"`
	Severity DiagnosticSeverity `json:"severity,omitempty"`
	Code     string             `json:"code,omitempty"`
	Source   string             `json:"source,omitempty"`
	Message  string             `json:"message"`
}

// ServerCapabilities declares what the server supports.
type ServerCapabilities struct {
	TextDocumentSync int `json:"textDocumentSync"`
}

// ServerInfo identifies the server in the initialize response.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Linter is the callback the server calls to produce diagnostics for a
// project rooted at root. An error means the result is incomplete.
type Linter func(root string) (map[string][]Diagnostic, error)

// Server runs the LSP main loop reading from r and writing to w.
type Server struct {
	r          *Reader
	w          *Writer
	linter     Linter
	root       string
	published  map[string]bool
	clientURIs map[string]string
}

// New returns a Server that reads from r, writes to w, and delegates
// diagnostics to linter.
func New(r io.Reader, w io.Writer, linter Linter) *Server {
	return &Server{r: NewReader(r), w: NewWriter(w), linter: linter, published: map[string]bool{}, clientURIs: map[string]string{}}
}

// Run reads messages until the stream closes or exit is received.
func (s *Server) Run() error {
	for {
		msg, err := s.r.Read()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("lsp read: %w", err)
		}
		if msg.Method == "exit" {
			return nil
		}
		s.dispatch(msg)
	}
}

func (s *Server) dispatch(msg *Message) {
	switch msg.Method {
	case "initialize":
		s.handleInitialize(msg)
	case "initialized":
		// no-op notification
	case "textDocument/didOpen":
		s.handleDidOpen(msg)
	case "textDocument/didChange":
		s.handleDidChange(msg)
	case "textDocument/didSave":
		s.handleDidSave(msg)
	case "textDocument/didClose":
		s.handleDidClose(msg)
	case "shutdown":
		s.reply(msg.ID, struct{}{})
	default:
		if msg.ID != nil {
			s.replyError(msg.ID, errCodeMethodNotFound,
				fmt.Sprintf("method not supported: %s", msg.Method))
		}
	}
}

func (s *Server) handleInitialize(msg *Message) {
	var params struct {
		RootURI string `json:"rootUri"`
	}
	if err := json.Unmarshal(msg.Params, &params); err == nil && params.RootURI != "" {
		s.root = uriToPath(params.RootURI)
	}
	if s.root == "" {
		s.root = "."
	}
	s.reply(msg.ID, map[string]any{
		"capabilities": ServerCapabilities{
			TextDocumentSync: 1, // full sync
		},
		"serverInfo": ServerInfo{
			Name:    "agnostic-ai",
			Version: "lsp",
		},
	})
}

func (s *Server) handleDidOpen(msg *Message) {
	var params struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return
	}
	s.publishDiagnostics(params.TextDocument.URI)
}

func (s *Server) handleDidChange(msg *Message) {
	var params struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return
	}
	s.publishDiagnostics(params.TextDocument.URI)
}

func (s *Server) handleDidSave(msg *Message) {
	var params struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return
	}
	s.publishDiagnostics(params.TextDocument.URI)
}

func (s *Server) handleDidClose(msg *Message) {
	var params struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return
	}
	delete(s.clientURIs, pathToURI(uriToPath(params.TextDocument.URI)))
	delete(s.published, params.TextDocument.URI)
	// Clear diagnostics for the closed file.
	s.notify("textDocument/publishDiagnostics", map[string]any{
		"uri":         params.TextDocument.URI,
		"diagnostics": []Diagnostic{},
	})
}

// publishDiagnostics runs the linter and sends results for all affected files.
func (s *Server) publishDiagnostics(triggerURI string) {
	if s.linter == nil {
		return
	}
	triggerIdentity := pathToURI(uriToPath(triggerURI))
	for uri := range s.published {
		if uri != triggerURI && pathToURI(uriToPath(uri)) == triggerIdentity {
			delete(s.published, uri)
			s.published[triggerURI] = true
		}
	}
	s.clientURIs[triggerIdentity] = triggerURI
	byPath, err := s.linter(s.root)
	byURI := map[string][]Diagnostic{}
	for path, diagnostics := range byPath {
		if err != nil && len(diagnostics) == 0 {
			continue
		}
		uri := pathToURI(path)
		if clientURI, ok := s.clientURIs[uri]; ok {
			uri = clientURI
		}
		byURI[uri] = diagnostics
	}
	if err == nil {
		for uri := range s.published {
			if _, ok := byURI[uri]; !ok {
				byURI[uri] = []Diagnostic{}
			}
		}
		if _, ok := byURI[triggerURI]; !ok {
			byURI[triggerURI] = []Diagnostic{}
		}
	} else if len(byURI) == 0 {
		s.notify("window/showMessage", map[string]any{
			"type":    1,
			"message": fmt.Sprintf("agnostic-ai could not analyze this workspace: %v. Fix the reported problem and save a project file to retry.", err),
		})
	}
	uris := make([]string, 0, len(byURI))
	for uri := range byURI {
		uris = append(uris, uri)
	}
	sort.Strings(uris)
	for _, uri := range uris {
		diagnostics := byURI[uri]
		if diagnostics == nil {
			diagnostics = []Diagnostic{}
		}
		s.notify("textDocument/publishDiagnostics", map[string]any{
			"uri":         uri,
			"diagnostics": diagnostics,
		})
		if len(diagnostics) == 0 {
			delete(s.published, uri)
		} else {
			s.published[uri] = true
		}
	}
}

func (s *Server) reply(id json.RawMessage, result any) {
	_ = s.w.Send(map[string]any{
		"jsonrpc": "2.0",
		"id":      json.RawMessage(id),
		"result":  result,
	})
}

func (s *Server) replyError(id json.RawMessage, code int, message string) {
	_ = s.w.Send(map[string]any{
		"jsonrpc": "2.0",
		"id":      json.RawMessage(id),
		"error":   ResponseError{Code: code, Message: message},
	})
}

func (s *Server) notify(method string, params any) {
	_ = s.w.Send(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
}

// uriToPath converts a file:// URI to an OS path.
func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return uri
	}
	p := u.Path
	if u.Host != "" && u.Host != "localhost" {
		p = "//" + u.Host + p
	}
	// On Windows, /C:/... → C:\...
	if len(p) > 2 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
		p = strings.ReplaceAll(p, "/", string(filepath.Separator))
	}
	return filepath.FromSlash(p)
}

// pathToURI converts an OS path to a file:// URI.
func pathToURI(path string) string {
	slashPath := filepath.ToSlash(path)
	if strings.HasPrefix(slashPath, "//") {
		host, rest, _ := strings.Cut(strings.TrimPrefix(slashPath, "//"), "/")
		return (&url.URL{Scheme: "file", Host: host, Path: "/" + rest}).String()
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.ToSlash(abs)
	if !strings.HasPrefix(abs, "/") {
		abs = "/" + abs
	}
	return (&url.URL{Scheme: "file", Path: abs}).String()
}
