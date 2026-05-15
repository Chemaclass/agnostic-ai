package lsp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
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
// project rooted at root. Returns a map of absolute file path → diagnostics.
type Linter func(root string) map[string][]Diagnostic

// Server runs the LSP main loop reading from r and writing to w.
type Server struct {
	r      *Reader
	w      *Writer
	linter Linter
	root   string
	exitFn func(int) // defaults to os.Exit
}

// New returns a Server that reads from r, writes to w, and delegates
// diagnostics to linter.
func New(r io.Reader, w io.Writer, linter Linter) *Server {
	return &Server{r: NewReader(r), w: NewWriter(w), linter: linter, exitFn: os.Exit}
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
	case "exit":
		s.exitFn(0)
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
	byPath := s.linter(s.root)
	// Send results for every file the linter reported, plus clear the trigger
	// file if the linter produced no findings for it.
	triggerPath := uriToPath(triggerURI)
	seen := map[string]bool{}
	for path, diags := range byPath {
		uri := pathToURI(path)
		s.notify("textDocument/publishDiagnostics", map[string]any{
			"uri":         uri,
			"diagnostics": diags,
		})
		seen[path] = true
	}
	if !seen[triggerPath] {
		s.notify("textDocument/publishDiagnostics", map[string]any{
			"uri":         triggerURI,
			"diagnostics": []Diagnostic{},
		})
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
	// On Windows, /C:/... → C:\...
	if len(p) > 2 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
		p = strings.ReplaceAll(p, "/", string(filepath.Separator))
	}
	return p
}

// pathToURI converts an OS path to a file:// URI.
func pathToURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.ToSlash(abs)
	if !strings.HasPrefix(abs, "/") {
		abs = "/" + abs
	}
	return "file://" + abs
}
