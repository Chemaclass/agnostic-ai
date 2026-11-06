package cli

import (
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const sourceViewPath = "/api/views/source"

// sourceBodyLimit caps the body one source view returns, and
// sourceFrontmatterLimit the leading block read before it, which is
// parsed whole so its values are redacted.
const (
	sourceBodyLimit        = 256 << 10
	sourceFrontmatterLimit = 64 << 10
)

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
	Withheld    bool          `json:"withheld,omitempty"`
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
// rows may show, with the folders those files must resolve into: each
// layer's source folder of a kind the rows list, and the folder of each
// instructions file.
func sourceFiles(global bool) (map[string]string, []string, error) {
	var in overview.Inputs
	var layers []spec.Layer
	if global {
		scope, err := loadCheckScope(true)
		if err != nil {
			return nil, nil, err
		}
		in = overview.Inputs{Cfg: &config.Config{Targets: scope.targets}, Bundle: scope.bundle, Instructions: globalInstructionDocuments(scope.source)}
		if layers, err = globalLayers(scope.source, nil); err != nil {
			return nil, nil, err
		}
	} else {
		cfg, b, err := loadProject(".")
		if err != nil {
			return nil, nil, err
		}
		in = overview.Inputs{Cfg: cfg, Bundle: b, Instructions: projectInstructionDocuments()}
		layers = append(resolvePacksLayers("."), resolveProjectLayer(".", cfg))
		if l, ok := resolveProjectUserLayer("."); ok {
			layers = append(layers, l)
		}
	}
	var roots []string
	for _, l := range layers {
		dirs := sourceDirsByKind(l.Sources)
		for _, kind := range sourceKinds {
			if dirs[kind] != "" {
				roots = append(roots, config.ResolveSourcePath(l.Root, dirs[kind]))
			}
		}
	}
	for _, doc := range in.Instructions {
		roots = append(roots, filepath.Dir(doc.AbsPath))
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

// sourceKinds are the spec kinds whose files the rows can show.
var sourceKinds = []spec.Kind{spec.KindRule, spec.KindAgent, spec.KindSkill, spec.KindCommand, spec.KindReview, spec.KindIgnore}

// readSource reads path when it resolves to a regular file inside one
// of roots, which have their symlinks resolved. The open stays inside
// that root, so a link swapped in after the check cannot lead out.
func readSource(path string, roots []string) (sourceView, bool) {
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return sourceView{}, false
	}
	f, ok := openInsideRoots(target, roots)
	if !ok {
		return sourceView{}, false
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return sourceView{}, false
	}
	limit := sourceFrontmatterLimit + sourceBodyLimit
	data, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil {
		return sourceView{}, false
	}
	view := sourceView{Bytes: info.Size(), Frontmatter: []sourceField{}}
	if len(data) > limit {
		view.Truncated, view.Withheld = true, true
		return view, true
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	shown := preview.Display(path, text)
	if shown.Withheld {
		view.Withheld = true
		return view, true
	}
	original := text
	if shown.Hidden {
		text = shown.Text
	}
	keys, meta, body := splitSource(text)
	// Frontmatter values show redacted, but a body the preview had to
	// redact is withheld whole: the same value may repeat in plain text.
	if _, _, raw := splitSource(original); raw != body {
		view.Withheld = true
		return view, true
	}
	if len(body) > sourceBodyLimit {
		body, view.Truncated = string(validUTF8Prefix([]byte(body[:sourceBodyLimit]))), true
	}
	head := strings.TrimSuffix(text, body)
	if sensitiveSource(path, body) || sensitiveSource(path, head) {
		view.Withheld = true
		return view, true
	}
	view.Body = body
	for _, key := range keys {
		view.Frontmatter = appendSourceFields(view.Frontmatter, preview.RedactText(key), meta[key])
	}
	return view, true
}

// splitSource is text's frontmatter keys, values and body. A leading
// --- block never reaches the body, whatever it parses to.
func splitSource(text string) ([]string, map[string]any, string) {
	keys, meta, body, err := spec.SplitFrontmatter([]byte(text))
	if err != nil {
		keys = nil
	}
	if leadingBlock(text) {
		body = afterLeadingBlock(text)
	}
	return keys, meta, body
}

// leadingBlock reports whether text opens with a --- line. That block
// never reaches the body, whatever it parses to, so a value the parser
// treats as body text (a YAML list, a block that does not parse, one
// that never closes) cannot show unredacted.
func leadingBlock(text string) bool {
	first, _, _ := strings.Cut(text, "\n")
	return strings.TrimRight(first, " \t") == "---"
}

// afterLeadingBlock is text without its leading block, or nothing when
// that block never closes.
func afterLeadingBlock(text string) string {
	_, rest, _ := strings.Cut(text, "\n")
	for rest != "" {
		line, next, _ := strings.Cut(rest, "\n")
		if strings.TrimRight(line, " \t") == "---" {
			return next
		}
		rest = next
	}
	return ""
}

func openInsideRoots(target string, roots []string) (*os.File, bool) {
	for _, root := range roots {
		rel, inside := resolvedInside(root, target)
		if !inside {
			continue
		}
		f, err := os.OpenInRoot(root, rel)
		return f, err == nil
	}
	return nil, false
}

// validUTF8Prefix drops a rune the cut split in two.
func validUTF8Prefix(data []byte) []byte {
	for i := 0; i < utf8.UTFMax && len(data) > 0 && !utf8.Valid(data); i++ {
		data = data[:len(data)-1]
	}
	return data
}

// appendSourceFields adds one field per leaf of v: a map walks into
// dotted keys in sorted order, a list of plain values shows its items
// joined by commas, a list holding maps or lists walks each item by
// index, and each value is shown and redacted as the overview shows it.
// A value of any other shape is hidden, so an unexpected YAML type never
// reaches the page unredacted.
func appendSourceFields(fields []sourceField, key string, v any) []sourceField {
	switch t := v.(type) {
	case map[string]any:
		for _, k := range slices.Sorted(maps.Keys(t)) {
			fields = appendSourceFields(fields, key+"."+preview.RedactText(k), t[k])
		}
		return fields
	case map[any]any:
		named := make(map[string]any, len(t))
		for k, item := range t {
			named[fmt.Sprint(k)] = item
		}
		return appendSourceFields(fields, key, named)
	case []any:
		if slices.ContainsFunc(t, isNested) {
			for i, item := range t {
				fields = appendSourceFields(fields, key+"."+strconv.Itoa(i), item)
			}
			return fields
		}
		items := make([]string, 0, len(t))
		for _, item := range t {
			items = append(items, shownSourceValue(key, item))
		}
		return append(fields, sourceField{Key: key, Value: strings.Join(items, ", ")})
	}
	return append(fields, sourceField{Key: key, Value: shownSourceValue(key, v)})
}

func shownSourceValue(key string, v any) string {
	switch v.(type) {
	case nil, string, bool, int, int64, uint64, float64:
		return overview.ShownText(key, v)
	}
	return sourceHidden
}

const sourceHidden = "<redacted>"

func isNested(v any) bool {
	switch v.(type) {
	case map[string]any, map[any]any, []any:
		return true
	}
	return false
}

var (
	credentialAssignment = regexp.MustCompile(`(?i)(?:pass(?:word|wd|phrase)?|secret|token|api[_-]?key|access[ _-]?key|private[_-]?key|credentials?|auth(?:orization)?|bearer|cookie|session|client[ _-]?secret)[A-Za-z0-9_.-]*["'*_` + "`" + `]*\s*[:=]`)
	credentialWord       = regexp.MustCompile(`(?i)\b(?:pass(?:word|wd|phrase)s?|secrets?|credentials?|api[ _-]?keys?|private[ _-]?keys?|client[ _-]?secrets?)\b`)
	credentialLabel      = regexp.MustCompile(`(?im)(?:<\s*/?\s*[*_~` + "`" + `]*(?:tokens?|auth(?:orization)?|bearer|cookies?|sessions?(?:[ _-]?ids?)?|access[ _-]?keys?)[\s/>]|(?:\|\s*|^\s*)[*_~` + "`" + `]*(?:tokens?|auth(?:orization)?|bearer|cookies?|sessions?(?:[ _-]?ids?)?|access[ _-]?keys?)[*_~` + "`" + `]*(?:\s*\||[^A-Za-z|\n]*\|))`)
	privateKeyBlock      = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY`)
)

// sensitiveSource reports whether the body shows any sign of a credential:
// the shared preview hid or withheld part of it, a credential-named key
// is assigned, a strong credential word (password, secret, API key)
// appears anywhere, a weaker one (token, auth, cookie) labels a table
// cell or an XML tag, a private key block starts, or a recognizable
// token or credential appears. The whole body is then withheld rather than
// redacted piece by piece, so an unusual layout cannot slip a value
// through; the file still opens in the editor. A credential written as
// free prose is not detectable.
func sensitiveSource(path, body string) bool {
	shown := preview.Display(path, body)
	return shown.Hidden || shown.Withheld || credentialAssignment.MatchString(body) || credentialWord.MatchString(body) ||
		credentialLabel.MatchString(body) || privateKeyBlock.MatchString(body) ||
		preview.TokenIn(body) || preview.CredentialDetected(body)
}
