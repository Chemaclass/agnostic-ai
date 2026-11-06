package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/codebuddy"
	"github.com/chemaclass/agnostic-ai/internal/atomicfile"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// editSpecMovePath copies or moves one shared spec between the project
// and the global setup.
const editSpecMovePath = "/api/edit/spec/move"

// movableKinds are the kinds both a project and the global setup read.
// The global setup reads no commands, reviews, environments, or ignore
// files, and settings merge by key instead of moving whole.
var (
	removeMoveStaging = os.RemoveAll
	renameMoveUnit    = os.Rename
)

var movableKinds = []spec.Kind{spec.KindRule, spec.KindAgent, spec.KindSkill, spec.KindHook, spec.KindMCP}

type specMoveRequest struct {
	Action    string `json:"action"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	From      string `json:"from"`
	To        string `json:"to"`
	Mode      string `json:"mode"`
	Overwrite bool   `json:"overwrite"`
	Expected  string `json:"expected,omitempty"`
}

type specMovePreview struct {
	Source       string   `json:"source"`
	Destination  string   `json:"destination"`
	Exists       bool     `json:"exists"`
	Files        []string `json:"files"`
	SyncCommands []string `json:"syncCommands"`
	Expected     string   `json:"expected"`
	OK           bool     `json:"ok,omitempty"`
	Warning      string   `json:"warning,omitempty"`
}

// specMovePlan is what apply writes: unit is the spec file, or a folder
// skill's folder, on each side.
type specMovePlan struct {
	preview specMovePreview
	srcDir  string
	// stage is where a skill folder is built and the replaced one kept
	// until the swap ends: the destination root, which no loader scans.
	stage    string
	src      string
	dst      string
	folder   bool
	files    []specMoveFile
	existing bool
}

type specMoveFile struct {
	rel  string
	data []byte
	mode fs.FileMode
}

// specMoveError carries the status a refusal answers with.
type specMoveError struct {
	status int
	reply  any
}

func (e specMoveError) Error() string { return fmt.Sprint(e.reply) }

func moveRefusal(status int, format string, args ...any) specMoveError {
	return specMoveError{status: status, reply: editError{Error: fmt.Sprintf(format, args...)}}
}

type specMoveHandler struct {
	root   string
	global bool
	// peer is the other level, fixed when the server starts; nil when a
	// global server knows no project.
	peer *switchPeer
}

func newSpecMoveHandler(root string, global bool, peer *switchPeer) http.Handler {
	return &specMoveHandler{root: root, global: global, peer: peer}
}

func (h *specMoveHandler) level() string { return levelOf(h.global) }

func levelOf(global bool) string {
	if global {
		return "global"
	}
	return "project"
}

func (h *specMoveHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if status, msg := editRequestGuard(r); status != 0 {
		writeEditJSON(w, status, editError{Error: msg})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "read request: " + err.Error()})
		return
	}
	var req specMoveRequest
	if json.Unmarshal(body, &req) != nil || (req.Action != "preview" && req.Action != "apply") || req.Name == "" {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "send action preview or apply, kind, name, from, to, mode, and overwrite"})
		return
	}
	plan, err := h.plan(req)
	if err != nil {
		writeSpecMoveError(w, err)
		return
	}
	if req.Action == "preview" {
		writeEditJSON(w, http.StatusOK, plan.preview)
		return
	}
	if req.Expected == "" || req.Expected != plan.preview.Expected {
		writeEditJSON(w, http.StatusConflict, editError{Error: "The spec or its destination changed; review a fresh preview before applying"})
		return
	}
	if plan.existing && !req.Overwrite {
		writeEditJSON(w, http.StatusConflict, editError{Error: fmt.Sprintf("%s already exists; tick overwrite to replace it", plan.preview.Destination)})
		return
	}
	warning, err := plan.write()
	if err != nil {
		writeEditJSON(w, http.StatusUnprocessableEntity, editError{Error: err.Error()})
		return
	}
	plan.preview.Warning = warning
	if req.Mode == "move" {
		if err := plan.removeSource(); err != nil {
			writeEditJSON(w, http.StatusUnprocessableEntity, editError{Error: fmt.Sprintf("wrote %s but could not remove %s: %v", plan.preview.Destination, plan.preview.Source, err)})
			return
		}
	}
	plan.preview.OK = true
	writeEditJSON(w, http.StatusOK, plan.preview)
}

func writeSpecMoveError(w http.ResponseWriter, err error) {
	var refusal specMoveError
	if errors.As(err, &refusal) {
		writeEditJSON(w, refusal.status, refusal.reply)
		return
	}
	writeEditJSON(w, http.StatusUnprocessableEntity, editError{Error: err.Error()})
}

func (h *specMoveHandler) plan(req specMoveRequest) (specMovePlan, error) {
	kind := spec.Kind(req.Kind)
	if !slices.Contains(movableKinds, kind) {
		return specMovePlan{}, moveRefusal(http.StatusBadRequest, "kind must be rule, agent, skill, hook, or mcp; the global setup does not read %s specs", req.Kind)
	}
	if req.From != h.level() {
		return specMovePlan{}, moveRefusal(http.StatusBadRequest, "from must be %s, the level this server reads", h.level())
	}
	if req.To != levelOf(!h.global) {
		return specMovePlan{}, moveRefusal(http.StatusBadRequest, "to must be %s", levelOf(!h.global))
	}
	if req.Mode != "copy" && req.Mode != "move" {
		return specMovePlan{}, moveRefusal(http.StatusBadRequest, "mode must be copy or move")
	}
	if h.peer == nil {
		return specMovePlan{}, moveRefusal(http.StatusConflict, "this server knows no project; start agnostic-ai ui --global inside a project")
	}
	if !h.global {
		missing, err := globalHomeMissing(h.peer.root)
		if err != nil {
			return specMovePlan{}, err
		}
		if missing {
			return specMovePlan{}, specMoveError{status: http.StatusConflict, reply: switchMissingReply{Error: "the global setup does not exist yet; create it first", Missing: true, Root: h.peer.root}}
		}
	}
	// A project server loads the working folder, so its spec paths are
	// relative to it.
	base := h.root
	if !h.global {
		base = "."
	}
	root, err := filepath.Abs(base)
	if err != nil {
		return specMovePlan{}, err
	}
	scope, hooks, err := loadCheckScopeAllHooks(h.global)
	if err != nil {
		return specMovePlan{}, err
	}
	entries := slices.Concat(scope.bundle.Agents, scope.bundle.Skills, scope.bundle.Rules, hooks, scope.bundle.MCPs)
	i := slices.IndexFunc(entries, func(e spec.Entry) bool { return e.Kind == kind && e.Name == req.Name })
	if i < 0 {
		return specMovePlan{}, moveRefusal(http.StatusNotFound, "no %s named %s in the %s; reload to review it", kind, req.Name, h.level())
	}
	entry := entries[i]
	if !filepath.IsAbs(entry.Path) {
		entry.Path = filepath.Join(root, entry.Path)
	}
	switch {
	case entry.Layer == layerNameBuiltin:
		return specMovePlan{}, moveRefusal(http.StatusUnprocessableEntity, "%s is a built-in; switch it on in the other level instead", req.Name)
	case strings.HasPrefix(entry.Layer, layerNamePackPrefix):
		return specMovePlan{}, moveRefusal(http.StatusUnprocessableEntity, "%s comes from %s; copy it from the pack instead", req.Name, entry.Layer)
	case entry.Layer != h.level():
		return specMovePlan{}, moveRefusal(http.StatusUnprocessableEntity, "%s is a local spec; local specs stay where they are", req.Name)
	}
	if kind == spec.KindRule && !h.global {
		conditional, err := globalRuleRefused(entry, h.peer.root)
		if err != nil {
			return specMovePlan{}, err
		}
		if conditional {
			return specMovePlan{}, moveRefusal(http.StatusUnprocessableEntity, "%s has a scope, paths, or target condition; global rules must apply to every file and tool", req.Name)
		}
	}
	var srcDir string
	if h.global {
		srcDir = filepath.Join(root, sourceFor(globalSources, kind))
	} else {
		srcDir = config.ResolveSourcePath(root, sourceFor(scope.cfg.Sources, kind))
	}
	dstRoot, dstSources, err := h.destination()
	if err != nil {
		return specMovePlan{}, err
	}
	dstDir := config.ResolveSourcePath(dstRoot, sourceFor(dstSources, kind))

	plan := specMovePlan{srcDir: srcDir, src: entry.Path, stage: dstRoot}
	if kind == spec.KindSkill && filepath.Base(entry.Path) == "SKILL.md" {
		plan.folder, plan.src = true, filepath.Dir(entry.Path)
	}
	rel, err := filepath.Rel(srcDir, plan.src)
	if err != nil || !filepath.IsLocal(rel) || rel == "." {
		return specMovePlan{}, moveRefusal(http.StatusUnprocessableEntity, "%s is outside %s or is the folder itself; move it by hand", entry.Path, srcDir)
	}
	plan.dst = filepath.Join(dstDir, rel)
	overlap, err := overlappingMoveUnits(plan.src, plan.dst)
	if err != nil {
		return specMovePlan{}, err
	}
	if overlap {
		return specMovePlan{}, moveRefusal(http.StatusConflict, "%s and its destination hold the same files; one setup lies inside the other", entry.Path)
	}
	inside, err := insideRealRoot(dstRoot, plan.dst)
	if err != nil {
		return specMovePlan{}, err
	}
	if !inside {
		return specMovePlan{}, moveRefusal(http.StatusUnprocessableEntity, "%s leads outside %s through a link; move it by hand", plan.dst, dstRoot)
	}
	if kind == spec.KindSkill {
		if err := refuseSkillLayoutChange(plan.dst, dstDir, plan.folder); err != nil {
			return specMovePlan{}, err
		}
	}
	if err := h.refuseOtherDestinationSpec(dstRoot, dstSources, kind, req.Name, plan.dst, plan.folder); err != nil {
		return specMovePlan{}, err
	}
	if plan.files, err = readMoveUnit(plan.src, plan.folder); err != nil {
		return specMovePlan{}, err
	}
	current, err := readMoveUnitIfPresent(plan.dst, plan.folder)
	if err != nil {
		return specMovePlan{}, err
	}
	plan.existing = current != nil

	home, _ := globalUserHome()
	display := func(level, path string) string {
		if level == "global" {
			return nativeHomePath(home, path)
		}
		return overviewDisplay(root, path)
	}
	destination := plan.dst
	if plan.folder {
		destination = filepath.Join(plan.dst, "SKILL.md")
	}
	plan.preview = specMovePreview{
		Source:       display(h.level(), entry.Path),
		Destination:  display(req.To, destination),
		Exists:       plan.existing,
		Files:        make([]string, 0, len(plan.files)),
		SyncCommands: h.syncCommands(req.Mode),
	}
	for _, f := range plan.files {
		plan.preview.Files = append(plan.preview.Files, f.rel)
	}
	// One preview covers both buttons and the overwrite box: the revision
	// names the spec and the two sides, not the choice.
	identity := req
	identity.Action, identity.Expected, identity.Mode, identity.Overwrite = "", "", "", false
	shown := plan.preview
	shown.SyncCommands = nil
	sum := sha256.New()
	_ = json.NewEncoder(sum).Encode(struct {
		Request specMoveRequest
		Preview specMovePreview
		Src     string
		Dst     string
	}{identity, shown, plan.src, plan.dst})
	for _, unit := range [][]specMoveFile{plan.files, current} {
		for _, f := range unit {
			_, _ = fmt.Fprintf(sum, "%s\x00%o\x00%d\x00", f.rel, f.mode, len(f.data))
			_, _ = sum.Write(f.data)
		}
		_, _ = sum.Write([]byte{0})
	}
	plan.preview.Expected = hex.EncodeToString(sum.Sum(nil))
	return plan, nil
}

// destination is the peer's root and its spec folders.
func (h *specMoveHandler) destination() (string, config.Sources, error) {
	if !h.global {
		return h.peer.root, globalSources, nil
	}
	cfg, _, err := loadValidConfig(h.peer.root)
	if err != nil {
		return "", config.Sources{}, err
	}
	return h.peer.root, cfg.Sources, nil
}

// refuseOtherDestinationSpec stops a copy that would leave two specs of one
// name in the destination: one at another path would still win or clash.
func (h *specMoveHandler) refuseOtherDestinationSpec(root string, sources config.Sources, kind spec.Kind, name, dst string, folder bool) error {
	b, err := spec.LoadLayered([]spec.Layer{{Name: levelOf(!h.global), Root: root, Sources: sources}})
	if err != nil {
		return err
	}
	for _, e := range b.All() {
		if e.Kind != kind || !strings.EqualFold(e.Name, name) {
			continue
		}
		at := e.Path
		if folder && filepath.Base(at) == "SKILL.md" {
			at = filepath.Dir(at)
		}
		if at != dst {
			return moveRefusal(http.StatusConflict, "a %s named %s already exists at %s", kind, name, e.Path)
		}
	}
	return nil
}

func (h *specMoveHandler) syncCommands(mode string) []string {
	project := "agnostic-ai sync"
	if h.global {
		project = "cd " + shellQuote(h.peer.root) + " && agnostic-ai sync"
	}
	global := "agnostic-ai sync --global"
	if mode == "copy" {
		if h.global {
			return []string{project}
		}
		return []string{global}
	}
	return []string{project, global}
}

func sourceFor(s config.Sources, kind spec.Kind) string {
	switch kind {
	case spec.KindAgent:
		return s.Agents
	case spec.KindSkill:
		return s.Skills
	case spec.KindRule:
		return s.Rules
	case spec.KindHook:
		return s.Hooks
	case spec.KindMCP:
		return s.MCPs
	}
	return ""
}

func readMoveUnitIfPresent(path string, folder bool) ([]specMoveFile, error) {
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	files, err := readMoveUnit(path, folder)
	if err != nil {
		return nil, err
	}
	if files == nil {
		files = []specMoveFile{}
	}
	return files, nil
}

// readMoveUnit reads the spec file, or every file of a skill folder. A
// link or other special file stops the move, so nothing outside the
// folder is copied or deleted.
func readMoveUnit(path string, folder bool) ([]specMoveFile, error) {
	read := func(full, rel string) (specMoveFile, error) {
		info, err := os.Lstat(full)
		if err != nil {
			return specMoveFile{}, err
		}
		if !info.Mode().IsRegular() {
			return specMoveFile{}, moveRefusal(http.StatusUnprocessableEntity, "%s is not a regular file; move it by hand", full)
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return specMoveFile{}, err
		}
		return specMoveFile{rel: filepath.ToSlash(rel), data: data, mode: info.Mode().Perm()}, nil
	}
	if !folder {
		f, err := read(path, filepath.Base(path))
		if err != nil {
			return nil, err
		}
		return []specMoveFile{f}, nil
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, moveRefusal(http.StatusUnprocessableEntity, "%s is not a folder; move it by hand", path)
	}
	var files []specMoveFile
	err = filepath.WalkDir(path, func(full string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(path, full)
		if err != nil {
			return err
		}
		f, err := read(full, rel)
		files = append(files, f)
		return err
	})
	return files, err
}

// write replaces the destination. A file goes through atomicfile; a skill
// folder is built in the stage folder and renamed into place, so a
// failure leaves the old folder whole. The reply is a warning when the
// swap worked but the stage folder could not be removed.
func (p specMovePlan) write() (string, error) {
	if !p.folder {
		f := p.files[0]
		if err := os.MkdirAll(filepath.Dir(p.dst), 0o755); err != nil {
			return "", err
		}
		return "", atomicfile.Write(p.dst, f.data, f.mode)
	}
	if err := os.MkdirAll(filepath.Dir(p.dst), 0o755); err != nil {
		return "", err
	}
	work, err := os.MkdirTemp(p.stage, spec.MoveStagingPrefix+"*")
	if err != nil {
		return "", err
	}
	if err := p.swapFolder(work); err != nil {
		var lost rollbackError
		if !errors.As(err, &lost) {
			_ = removeMoveStaging(work)
		}
		return "", err
	}
	if err := removeMoveStaging(work); err != nil {
		return fmt.Sprintf("wrote %s but could not remove %s: %v; delete it by hand", p.preview.Destination, work, err), nil
	}
	return "", nil
}

func (p specMovePlan) swapFolder(work string) error {
	staged, old := filepath.Join(work, "new"), filepath.Join(work, "old")
	for _, f := range p.files {
		full := filepath.Join(staged, filepath.FromSlash(f.rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := atomicfile.Write(full, f.data, f.mode); err != nil {
			return err
		}
	}
	if !p.existing {
		return renameMoveUnit(staged, p.dst)
	}
	if err := renameMoveUnit(p.dst, old); err != nil {
		return err
	}
	if err := renameMoveUnit(staged, p.dst); err != nil {
		if undo := renameMoveUnit(old, p.dst); undo != nil {
			return rollbackError{fmt.Errorf("replace %s: %w; restoring it failed too (%v): the old skill is in %s and the new one in %s", p.dst, err, undo, old, staged)}
		}
		return err
	}
	return nil
}

// rollbackError is a failed swap whose old folder could not go back, so
// the stage folder must stay.
type rollbackError struct{ error }

// globalRuleRefused reports whether sync --global would refuse rule for
// the targets the global setup at source selects.
func globalRuleRefused(rule spec.Entry, source string) (bool, error) {
	targets, err := loadGlobalTargets(source, io.Discard)
	if err != nil {
		return false, err
	}
	if len(targets) == 0 {
		targets = globalTargetNames()
	}
	if slices.Contains(targets, "codebuddy") && !(codebuddy.Adapter{}).AlwaysOnRule(rule) {
		return true, nil
	}
	return globalRuleConditional(rule, targets), nil
}

// overlappingMoveUnits reports whether src and dst, once links are
// followed, are one file or folder or one lies inside the other, as when
// one setup's folder links to, or sits inside, the other's.
func overlappingMoveUnits(src, dst string) (bool, error) {
	a, err := realPathOf(src)
	if err != nil {
		return false, err
	}
	b, err := realPathOf(dst)
	if err != nil {
		return false, err
	}
	within := func(parent, child string) bool {
		rel, err := filepath.Rel(parent, child)
		return err == nil && filepath.IsLocal(rel)
	}
	return within(a, b) || within(b, a), nil
}

// realPathOf follows the links of path's nearest existing folder and
// keeps the rest of path as written.
func realPathOf(path string) (string, error) {
	existing, rest := path, ""
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return path, nil
		}
		rest = filepath.Join(filepath.Base(existing), rest)
		existing = parent
	}
	real, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", moveRefusal(http.StatusUnprocessableEntity, "%s: %v; fix or remove the link", existing, err)
	}
	return filepath.Join(real, rest), nil
}

// insideRealRoot reports whether path, once links in its nearest existing
// folder are followed, stays inside root.
func insideRealRoot(root, path string) (bool, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false, err
	}
	dir := filepath.Dir(path)
	for {
		_, err := os.Lstat(dir)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false, nil
		}
		dir = parent
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false, moveRefusal(http.StatusUnprocessableEntity, "%s: %v; fix or remove the link", dir, err)
	}
	rel, err := filepath.Rel(realRoot, real)
	return err == nil && filepath.IsLocal(rel), nil
}

// refuseSkillLayoutChange stops a skill landing where the loader would
// read it differently: under a folder skill, whose files are its assets,
// or over a folder of flat skills instead of a folder skill.
func refuseSkillLayoutChange(dst, dstDir string, folder bool) error {
	for dir := filepath.Dir(dst); ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err == nil {
			return moveRefusal(http.StatusConflict, "%s holds a skill, which would read this one as its file; move it by hand", dir)
		}
		if dir == dstDir || dir == filepath.Dir(dir) {
			break
		}
	}
	if !folder {
		return nil
	}
	if info, err := os.Lstat(dst); err == nil && info.IsDir() {
		if _, err := os.Stat(filepath.Join(dst, "SKILL.md")); err != nil {
			return moveRefusal(http.StatusConflict, "%s is a folder of other skills, not a skill; move it by hand", dst)
		}
	}
	return nil
}

// removeSource deletes the source after a move: the spec file, or the
// skill folder, which plan checked sits inside the kind's folder.
func (p specMovePlan) removeSource() error {
	if !p.folder {
		return os.Remove(p.src)
	}
	rel, err := filepath.Rel(p.srcDir, p.src)
	if err != nil || !filepath.IsLocal(rel) || rel == "." {
		return fmt.Errorf("%s is not a skill folder inside %s", p.src, p.srcDir)
	}
	return os.RemoveAll(p.src)
}
