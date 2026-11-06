package cli

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const sourceViewPath = "/api/views/source"

// sourceBodyLimit caps the bytes one source view returns.
const sourceBodyLimit = 256 << 10

type sourceField struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type sourceView struct {
	Path        string        `json:"path"`
	Kind        string        `json:"kind"`
	Frontmatter []sourceField `json:"frontmatter"`
	Body        string        `json:"body"`
	Bytes       int64         `json:"bytes"`
	Truncated   bool          `json:"truncated"`
}

var errSourceNotFound = editError{Error: "path is not a file of the loaded configuration"}

// newSourceHandler serves the content of one file a row of the page
// lists, read on every request. Any other path answers 404, as does a
// listed file whose symlinks lead outside the configuration's folders.
func newSourceHandler(global bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeEditJSON(w, http.StatusMethodNotAllowed, editError{Error: r.Method + " is not supported here; use GET"})
			return
		}
		switch requestOriginStatus(r) {
		case http.StatusMisdirectedRequest:
			writeEditJSON(w, http.StatusMisdirectedRequest, editError{Error: "file content is served on loopback only"})
			return
		case http.StatusForbidden:
			writeEditJSON(w, http.StatusForbidden, editError{Error: "file content is served to the server origin only"})
			return
		}
		requested := r.URL.Query().Get("path")
		path := filepath.FromSlash(requested)
		if requested == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
			writeEditJSON(w, http.StatusNotFound, errSourceNotFound)
			return
		}
		files, roots, err := sourceFiles(global)
		if err != nil {
			writeEditJSON(w, http.StatusInternalServerError, editError{Error: preview.RedactText(err.Error())})
			return
		}
		kind, listed := files[filepath.ToSlash(path)]
		if !listed {
			writeEditJSON(w, http.StatusNotFound, errSourceNotFound)
			return
		}
		view, ok := readSource(path, roots)
		if !ok {
			writeEditJSON(w, http.StatusNotFound, errSourceNotFound)
			return
		}
		view.Path, view.Kind = requested, kind
		writeEditJSON(w, http.StatusOK, view)
	})
}

// sourceFiles loads the served configuration and returns the files its
// rows may show, with the folders those files must resolve into.
func sourceFiles(global bool) (map[string]string, []string, error) {
	var in overview.Inputs
	var roots []string
	if global {
		scope, err := loadCheckScope(true)
		if err != nil {
			return nil, nil, err
		}
		in = overview.Inputs{Cfg: &config.Config{Targets: scope.targets}, Bundle: scope.bundle, Instructions: globalInstructionDocuments(scope.source)}
		roots = []string{scope.source}
	} else {
		cfg, b, err := loadProject(".")
		if err != nil {
			return nil, nil, err
		}
		in = overview.Inputs{Cfg: cfg, Bundle: b, Instructions: projectInstructionDocuments()}
		roots = []string{".", filepath.Dir(adapters.AgnosticEntryPointPath)}
		if source, err := globalSourceRoot(); err == nil {
			roots = append(roots, source)
		}
	}
	resolved := make([]string, 0, len(roots))
	for _, root := range roots {
		if abs, err := filepath.Abs(root); err == nil {
			if target, err := filepath.EvalSymlinks(abs); err == nil {
				resolved = append(resolved, target)
			}
		}
	}
	return overview.SourceFiles(in), resolved, nil
}

// readSource reads path when it resolves to a regular file inside one
// of roots, which have their symlinks resolved.
func readSource(path string, roots []string) (sourceView, bool) {
	target, err := filepath.EvalSymlinks(path)
	if err != nil || !insideAny(target, roots) {
		return sourceView{}, false
	}
	f, err := os.Open(target)
	if err != nil {
		return sourceView{}, false
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return sourceView{}, false
	}
	data, err := io.ReadAll(io.LimitReader(f, sourceBodyLimit+1))
	if err != nil {
		return sourceView{}, false
	}
	view := sourceView{Bytes: info.Size(), Frontmatter: []sourceField{}}
	if len(data) > sourceBodyLimit {
		data, view.Truncated = validUTF8Prefix(data[:sourceBodyLimit]), true
	}
	keys, meta, body, err := spec.SplitFrontmatter(data)
	if err != nil {
		view.Body = string(data)
		return view, true
	}
	view.Body = body
	for _, key := range keys {
		view.Frontmatter = append(view.Frontmatter, sourceField{Key: key, Value: sourceFieldValue(key, meta[key])})
	}
	return view, true
}

func insideAny(path string, roots []string) bool {
	for _, root := range roots {
		if _, inside := resolvedInside(root, path); inside {
			return true
		}
	}
	return false
}

// validUTF8Prefix drops a rune the cut split in two.
func validUTF8Prefix(data []byte) []byte {
	for i := 0; i < utf8.UTFMax && len(data) > 0 && !utf8.Valid(data); i++ {
		data = data[:len(data)-1]
	}
	return data
}

// sourceFieldValue shows a list as its items joined by commas, and any
// other value as the overview shows it.
func sourceFieldValue(key string, v any) string {
	list, ok := v.([]any)
	if !ok {
		return overview.ShownText(key, v)
	}
	items := make([]string, 0, len(list))
	for _, item := range list {
		items = append(items, overview.ShownText(key, item))
	}
	return strings.Join(items, ", ")
}
