package cli

import (
	"bytes"
	"context"
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
	"time"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
	"github.com/chemaclass/agnostic-ai/internal/atomicfile"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/mailbox"
)

const (
	settingsViewPath  = "/api/views/settings"
	settingsWritePath = "/api/settings"
	settingsSyncPath  = "/api/settings/sync"
	settingsSchema    = "settings/v1"
)

// Keys POST /api/settings takes.
const (
	settingApprovalsMode      = "approvals.mode"
	settingApprovalsCompanion = "approvals.companion"
	settingMailboxApprove     = "mailbox.approveAgentSends"
)

// settingsByCompanion names who asked for a loosening write in the
// decision log. Slice 2 adds the web page.
const settingsByCompanion = approvals.ByCompanion

// settingsByWeb names a change asked from a page that view opened.
const settingsByWeb = "web"

// settingsView is the user settings both the companion menu and the web
// page show. Every read builds it from the files, so two servers on one
// disk agree.
type settingsView struct {
	SchemaVersion string            `json:"schemaVersion"`
	ConfigRoot    string            `json:"configRoot"`
	Approvals     settingsApprovals `json:"approvals"`
	Mailbox       *settingsMailbox  `json:"mailbox,omitempty"`
	Sync          settingsSync      `json:"sync"`
	// CompanionApp is the companion app's own settings; nil when the
	// configuration folder cannot be found.
	CompanionApp *settingsCompanionApp `json:"companionApp,omitempty"`
	// CanLoosen says whether this request carried proof that the person
	// asked, so the page can lock the controls that loosen.
	CanLoosen bool `json:"canLoosen"`
}

type settingsApprovals struct {
	// Mode is the approvals.mode the home config asks for.
	Mode string `json:"mode"`
	// ModeFile is the home file, relative to configRoot, whose approvals
	// block is in effect; empty when neither file has one.
	ModeFile             string            `json:"modeFile"`
	Companion            bool              `json:"companion"`
	Overrides            settingsOverrides `json:"overrides"`
	CompanionSetup       map[string]string `json:"companionSetup,omitempty"`
	CompanionUnavailable bool              `json:"companionUnavailable,omitempty"`
	// Shadowed names keys of the shared file that the local block hides.
	Shadowed []string              `json:"shadowed"`
	Tools    []approvalsToolStatus `json:"tools"`
	Problem  string                `json:"problem,omitempty"`
}

type settingsOverrides struct {
	Tools     int `json:"tools"`
	Projects  int `json:"projects"`
	AlwaysAsk int `json:"alwaysAsk"`
}

type settingsMailbox struct {
	ApproveAgentSends bool `json:"approveAgentSends"`
}

// settingsSync says whether a sync --scope user would put the asked
// approvals in effect, and why.
type settingsSync struct {
	Needed  bool     `json:"needed"`
	Reasons []string `json:"reasons"`
}

type settingsChange struct {
	Key      string          `json:"key"`
	Value    json.RawMessage `json:"value"`
	Expected json.RawMessage `json:"expected,omitempty"`
}

type settingsWriteReply struct {
	Settings settingsView `json:"settings"`
	// Changed names every file the write changed, relative to configRoot
	// when inside it.
	Changed []string `json:"changed"`
}

type settingsRefusal struct {
	Error    string        `json:"error"`
	Settings *settingsView `json:"settings,omitempty"`
}

// settingsHandler serves the user settings document, one write route
// and the sync route on a user-level server. A project server answers
// 404 for all three.
type settingsHandler struct {
	userLevel bool
	reader    *approvalsReader
	// marks is the conversation marks file; the mailbox sits beside it.
	marks func() string
	// secret is the companion's; a loosening write needs it or session.
	secret companionSecret
	// session is the key of a page this server opened, or nil.
	session *webSession
	version string
	// runSync runs agnostic-ai with args in process and returns what it
	// printed; tests replace it.
	runSync func(ctx context.Context, args ...string) ([]byte, error)
	// syncReplyWait is how long a sync may take to answer.
	syncReplyWait time.Duration
	now           func() time.Time
	// companionDir holds the companion's settings and state; tests replace it.
	companionDir func() (string, error)
}

func newSettingsHandler(userLevel bool, reader *approvalsReader, marks func() string, secret companionSecret, version string) *settingsHandler {
	h := &settingsHandler{userLevel: userLevel, reader: reader, marks: marks, secret: secret, version: version, syncReplyWait: settingsSyncReplyTimeout, now: time.Now, companionDir: companionDir}
	h.runSync = h.runInProcess
	return h
}

func (h *settingsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.userLevel {
		writeEditJSON(w, http.StatusNotFound, editError{Error: "User settings are served by the user-level server only: run agnostic-ai view --scope user."})
		return
	}
	switch r.URL.Path {
	case settingsViewPath:
		h.view(w, r)
	case settingsWritePath:
		h.write(w, r)
	case settingsSyncPath:
		h.sync(w, r)
	default:
		writeEditJSON(w, http.StatusNotFound, editError{Error: "no endpoint at " + r.URL.Path})
	}
}

// proven reports whether r carries the companion's secret or the key of
// a page this server opened.
func (h *settingsHandler) proven(r *http.Request) bool {
	return h.secret.allows(r) || h.session.allows(r)
}

// canProve reports whether this server holds a secret or a session key
// that a request could prove itself with.
func (h *settingsHandler) canProve() bool {
	return h.secret != "" || h.session != nil
}

// askedBy names who asked for a proven change, for the decision log.
func (h *settingsHandler) askedBy(r *http.Request) string {
	if h.secret.allows(r) {
		return settingsByCompanion
	}
	return settingsByWeb
}

func (h *settingsHandler) view(w http.ResponseWriter, r *http.Request) {
	if !allowGetOnly(w, r) {
		return
	}
	doc := h.document(r)
	tag, err := settingsTag(doc)
	if err != nil {
		writeEditJSON(w, http.StatusInternalServerError, editError{Error: err.Error()})
		return
	}
	w.Header().Set("ETag", tag)
	if r.Header.Get("If-None-Match") == tag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeEditJSON(w, http.StatusOK, doc)
}

func settingsTag(doc settingsView) (string, error) {
	data, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("encode settings: %w", err)
	}
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:16]) + `"`, nil
}

func (h *settingsHandler) mailbox() (mailbox.Store, bool) {
	if h.marks == nil {
		return mailbox.Store{}, false
	}
	return mailboxStoreAt(h.marks())
}

// document reads every setting from the files.
func (h *settingsHandler) document(r *http.Request) settingsView {
	doc := settingsView{
		SchemaVersion: settingsSchema,
		Approvals:     settingsApprovals{Mode: config.ApprovalsAsk, Shadowed: []string{}, Tools: []approvalsToolStatus{}},
		Sync:          settingsSync{Reasons: []string{}},
		CanLoosen:     h.proven(r),
	}
	if store, ok := h.mailbox(); ok {
		doc.Mailbox = &settingsMailbox{ApproveAgentSends: store.Settings().ApproveAgentSends}
	}
	if dir, err := h.companionDir(); err == nil {
		doc.CompanionApp = companionApp(dir)
	}
	source, err := h.reader.source()
	if err != nil {
		doc.Approvals.Problem = err.Error()
		return doc
	}
	doc.ConfigRoot = source
	if file, shadowed, err := approvalsBlockLayout(source); err != nil {
		doc.Approvals.Problem = err.Error()
	} else {
		doc.Approvals.ModeFile, doc.Approvals.Shadowed = file, shadowed
	}
	home, err := config.LoadHomeApprovals(source)
	if err != nil {
		doc.Approvals.Problem = err.Error()
		return doc
	}
	doc.Approvals.Mode = cmpMode(home.Mode)
	doc.Approvals.Companion = home.Companion
	doc.Approvals.Overrides = settingsOverrides{Tools: len(home.Tools), Projects: len(home.Projects), AlwaysAsk: len(home.AlwaysAsk)}
	status, err := h.reader.status()
	if err != nil {
		doc.Approvals.Problem = err.Error()
		return doc
	}
	doc.Approvals.Tools = status.Tools
	builtins, err := loadUserBuiltins(source, io.Discard)
	if err != nil {
		doc.Approvals.Problem = err.Error()
	}
	var companionProblems map[string]string
	if home.Companion {
		companionProblems = companionApprovalProblems(status.Tools)
		doc.Approvals.CompanionUnavailable = len(companionProblems) == 0
		doc.Approvals.CompanionSetup = companionApprovalSetup(companionProblems, home)
	}
	doc.Sync = approvalsSyncNeed(home, status, slices.Contains(builtins, config.ApprovalsBuiltin), companionProblems)
	return doc
}

func cmpMode(mode string) string {
	if mode == "" {
		return config.ApprovalsAsk
	}
	return mode
}

// Reasons a tool falls back to ask that a sync --scope user fixes.
var syncFixedReasons = []string{
	"the hook is not installed",
	"the reviewer is not on in the tool's user settings",
}

func companionApprovalProblems(tools []approvalsToolStatus) map[string]string {
	problems := make(map[string]string)
	for _, tool := range tools {
		if tool.Tool == "claude" || tool.Tool == "codex" {
			problems[tool.Tool] = approvalHookProblem(tool.Tool)
		}
	}
	return problems
}

func companionApprovalSetup(problems map[string]string, home config.ApprovalsConfig) map[string]string {
	setup := make(map[string]string, len(problems))
	for tool, problem := range problems {
		switch {
		case problem == "":
			continue
		case strings.HasPrefix(problem, syncFixedReasons[0]):
			setup[tool] = "install"
		case strings.HasPrefix(problem, codexPermissionDisabled):
			setup[tool] = "disabled"
		case strings.HasPrefix(problem, codexPermissionUntrusted):
			setup[tool] = "trust"
		default:
			setup[tool] = "unavailable"
		}
	}
	if _, configured := problems["codex"]; configured {
		if mode, _ := homeToolMode(home, "codex"); mode == config.ApprovalsReview {
			setup["codex"] = "reviewer"
		}
	}
	return setup
}

// approvalsSyncNeed says whether a sync --scope user would put the asked
// approvals in effect: the hook is not installed, approvals is missing
// from builtins, or a native reviewer key is missing or left over.
func approvalsSyncNeed(home config.ApprovalsConfig, status approvalsStatusOutput, hasBuiltin bool, companionProblems map[string]string) settingsSync {
	out := settingsSync{Reasons: []string{}}
	if (home.Loosens() || len(companionProblems) > 0) && !hasBuiltin {
		out.Reasons = append(out.Reasons, "approvals is not in builtins in the home config")
	}
	review := false
	for _, tool := range status.Tools {
		review = review || tool.Mode == config.ApprovalsReview
		problem := companionProblems[tool.Tool]
		if strings.HasPrefix(problem, syncFixedReasons[0]) {
			out.Reasons = append(out.Reasons, tool.Tool+": "+problem)
		}
		if tool.Mode == config.ApprovalsAsk || tool.Effective != config.ApprovalsAsk || tool.Reason == problem {
			continue
		}
		if slices.ContainsFunc(syncFixedReasons, func(reason string) bool { return strings.HasPrefix(tool.Reason, reason) }) {
			out.Reasons = append(out.Reasons, tool.Tool+": "+tool.Reason)
		}
	}
	if status.reviewerWritten && !review {
		out.Reasons = append(out.Reasons, "a tool's user settings still turn on the reviewer that review wrote")
	}
	out.Needed = len(out.Reasons) > 0
	return out
}

// approvalsBlockLayout names the home file whose approvals block is in
// effect, relative to source, and the keys of the shared block that a
// local block hides.
func approvalsBlockLayout(source string) (string, []string, error) {
	snap, err := readHomeSnapshot(source)
	if err != nil {
		return "", nil, err
	}
	return snap.approvalsLayout()
}

// approvalsLayout is approvalsBlockLayout on the snapshot.
func (s homeSnapshot) approvalsLayout() (string, []string, error) {
	var docs [2]map[string]yaml.Node
	for i, path := range s.paths {
		if err := yaml.Unmarshal(s.data[i], &docs[i]); err != nil {
			return "", nil, errs.Coded(errs.CodeConfigDecode, "parse %s: %w", path, err)
		}
	}
	shared, local := docs[0], docs[1]
	sharedBlock, inShared := shared["approvals"]
	localBlock, inLocal := local["approvals"]
	shadowed := []string{}
	switch {
	case inLocal:
		if inShared {
			for _, key := range []string{"mode", "tools", "projects", "companion"} {
				if mappingKey(&sharedBlock, key) && (key != "companion" || mappingKey(&localBlock, key)) {
					shadowed = append(shadowed, "approvals."+key)
				}
			}
		}
		return filepath.ToSlash(filepath.Join("local", config.ConfigFileName)), shadowed, nil
	case inShared:
		return config.ConfigFileName, shadowed, nil
	}
	return "", shadowed, nil
}

func mappingKey(node *yaml.Node, key string) bool {
	if node.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return true
		}
	}
	return false
}

// approvalsWriteFile is the home file an approvals write other than Ask
// me lands in: the one whose block is in effect, so per-tool and
// per-folder modes beside the changed key stay, else the local file.
func (s homeSnapshot) approvalsWriteFile() (string, error) {
	file, _, err := s.approvalsLayout()
	if err != nil {
		return "", err
	}
	if file == config.ConfigFileName {
		return s.paths[0], nil
	}
	return s.paths[1], nil
}

// errBrokenBlockNeedsProof refuses a write to an approvals block that
// does not load from a caller without the companion's secret.
var errBrokenBlockNeedsProof = errors.New("the approvals block in the home config does not load")

// errSettingNeedsProof refuses a write that would turn on automatic
// approvals for a caller without the companion's secret.
var errSettingNeedsProof = errors.New("this change would turn on automatic approvals; make it from the companion or a page opened with agnostic-ai view --scope user")

// writeApprovalsSetting sets one approvals key in the home config in
// source and returns the files it changed; the caller holds the approvals
// lock, and snap is the home config as read under it. Ask me writes what
// the Ask me chip writes. A value that needs the hook adds approvals to
// builtins when it is missing; when that fails, the approvals change is
// undone. Without proven, a block that does not load is left alone, and
// a change that would leave the policy looser than before is refused
// before it is written.
func writeApprovalsSetting(source string, snap homeSnapshot, key string, value any, proven bool) ([]string, error) {
	if key == "mode" && value == config.ApprovalsAsk {
		path, err := writeApprovalsAsk(source, snap, proven)
		if err != nil {
			return nil, err
		}
		return []string{path}, nil
	}
	path, err := snap.approvalsWriteFile()
	if err != nil {
		return nil, err
	}
	data := snap.of(path)
	before, loadErr := config.LoadHomeApprovalsFiles(source, snap.files())
	if loadErr != nil && !proven {
		// A block that does not load leaves every prompt to the person, and
		// a repair could turn on what it holds before any check runs.
		return nil, errBrokenBlockNeedsProof
	}
	undo := restoreTo(path, data, data == nil)
	ours, err := approvalsSetYAML(data, key, value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	// The policy the edit gives is checked before anything is written.
	home, err := config.LoadHomeApprovalsFiles(source, snap.with(path, ours))
	switch {
	case err != nil && loadErr == nil:
		return nil, fmt.Errorf("the change would break the approvals block, so it was not made: %w", err)
	case err == nil && !approvalsHolds(home, key, value):
		return nil, fmt.Errorf("%s: the change would not take effect, so it was not made; set approvals.%s by hand", path, key)
	case err == nil && !proven && approvalsLoosened(before, home):
		return nil, errSettingNeedsProof
	}
	if err := editApprovalsFile(snap, path, ours); err != nil {
		return nil, err
	}
	restore := func() error { return undo(ours) }
	changed := []string{path}
	if !needsApprovalsHook(key, value) {
		return changed, nil
	}
	builtins, err := loadUserBuiltins(source, io.Discard)
	if err == nil && slices.Contains(builtins, config.ApprovalsBuiltin) {
		return changed, nil
	}
	file := ""
	if err == nil {
		file, err = addApprovalsBuiltin(source)
	}
	if err != nil {
		return nil, errors.Join(fmt.Errorf("could not add approvals to builtins, so approvals.%s was not changed: %w", key, err), restore())
	}
	if !slices.Contains(changed, file) {
		changed = append(changed, file)
	}
	return changed, nil
}

// approvalsLoosened reports whether after approves a request that before
// left to the person: a looser mode anywhere, or the companion buttons.
func approvalsLoosened(before, after config.ApprovalsConfig) bool {
	if after.Companion && !before.Companion {
		return true
	}
	rank := func(mode string) int { return slices.Index(config.ApprovalModes, cmpMode(mode)) }
	if rank(after.Mode) > rank(before.Mode) {
		return true
	}
	for tool, mode := range after.Tools {
		if rank(mode) > rank(before.Tools[tool]) {
			return true
		}
	}
	for project, mode := range after.Projects {
		if rank(mode) > rank(before.Projects[project]) {
			return true
		}
	}
	return false
}

// approvalsHolds reports whether the loaded approvals carry key: value.
func approvalsHolds(home config.ApprovalsConfig, key string, value any) bool {
	switch key {
	case "mode":
		return cmpMode(home.Mode) == value
	case "companion":
		return home.Companion == value
	}
	return false
}

// restoreTo returns a func that puts path back to before (its bytes, or
// no file when missing) once an edit wrote ours. It leaves alone a file
// another program changed since.
func restoreTo(path string, before []byte, missing bool) func(ours []byte) error {
	return func(ours []byte) error {
		now, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(now, ours) {
			return fmt.Errorf("%s changed while the change was undone, so it was left as it is; check the approvals block", path)
		}
		return putBack(path, before, missing)
	}
}

func putBack(path string, before []byte, missing bool) error {
	if missing {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		return nil
	}
	return atomicfile.Write(path, before, 0o644)
}

// needsApprovalsHook reports whether approvals.key: value only works
// with the approvals hook installed.
func needsApprovalsHook(key string, value any) bool {
	switch key {
	case "mode":
		return value != config.ApprovalsAsk
	case "companion":
		return value == true
	}
	return false
}

// addApprovalsBuiltin turns the approvals built-in on in the personal
// home config, through the same edit the web page's built-in switch
// makes. The caller holds the approvals lock.
func addApprovalsBuiltin(source string) (string, error) {
	edits := &editHandler{root: source, userLevel: true, homeLocked: true}
	file, edit, err := edits.builtinsEdit(audiencePersonal, builtinRequest{name: config.ApprovalsBuiltin, enabled: true})
	if err != nil {
		return "", err
	}
	prepare := func() (func(bool) error, error) { return edits.preparePersonalOutOfGit(file) }
	problems, err := edits.applyValidatedWithPrepare(file, edit, prepare)
	if err != nil && len(problems) > 0 {
		err = fmt.Errorf("%w: %s", err, strings.Join(problems, "; "))
	}
	return file, err
}

// settingValue is a decoded change: the value as written to its file,
// and whether it makes the machine less safe.
type settingValue struct {
	value   any
	loosens bool
}

func decodeSettingValue(key string, raw json.RawMessage) (settingValue, error) {
	switch key {
	case settingApprovalsMode:
		var mode string
		if err := json.Unmarshal(raw, &mode); err != nil || !slices.Contains(config.ApprovalModes, mode) {
			return settingValue{}, errors.New("approvals.mode takes ask, review or yolo")
		}
		return settingValue{value: mode, loosens: mode != config.ApprovalsAsk}, nil
	case settingApprovalsCompanion, settingMailboxApprove:
		on, err := decodeSwitch(key, raw)
		return settingValue{value: on, loosens: on == (key == settingApprovalsCompanion)}, err
	case settingCompanionAnimate, settingCompanionDimWhenIdle, settingCompanionSmall:
		on, err := decodeSwitch(key, raw)
		return settingValue{value: on}, err
	case settingCompanionCheckForUpdates:
		// Off leaves a known fix uninstalled, so it needs proof like a loosening write.
		on, err := decodeSwitch(key, raw)
		return settingValue{value: on, loosens: !on}, err
	}
	return settingValue{}, fmt.Errorf("key must be %s, %s, %s or a companion.* switch", settingApprovalsMode, settingApprovalsCompanion, settingMailboxApprove)
}

// decodeSwitch reads true or false; null, which decodes as false, is refused.
func decodeSwitch(key string, raw json.RawMessage) (bool, error) {
	var on *bool
	if err := json.Unmarshal(raw, &on); err != nil || on == nil {
		return false, fmt.Errorf("%s takes true or false", key)
	}
	return *on, nil
}

// currentSetting is key's value in doc, as the page shows it.
// approvalsSetting is the value of an approvals key as the snapshot
// gives it, the same value the settings page shows: Ask me and the
// buttons off when the block does not load.
func (s homeSnapshot) approvalsSetting(source, key string) (any, bool) {
	home, err := config.LoadHomeApprovalsFiles(source, s.files())
	if err != nil {
		home = config.ApprovalsConfig{}
	}
	switch key {
	case settingApprovalsMode:
		return cmpMode(home.Mode), true
	case settingApprovalsCompanion:
		return home.Companion, true
	}
	return nil, false
}

func currentSetting(doc settingsView, key string) (any, bool) {
	switch key {
	case settingApprovalsMode:
		return doc.Approvals.Mode, true
	case settingApprovalsCompanion:
		return doc.Approvals.Companion, true
	case settingMailboxApprove:
		if doc.Mailbox == nil {
			return nil, false
		}
		return doc.Mailbox.ApproveAgentSends, true
	case settingCompanionAnimate, settingCompanionDimWhenIdle, settingCompanionSmall, settingCompanionCheckForUpdates:
		if doc.CompanionApp == nil {
			return nil, false
		}
		return *doc.CompanionApp.switchFor(key), true
	}
	return nil, false
}

func (h *settingsHandler) write(w http.ResponseWriter, r *http.Request) {
	// Only the server's own origin, not one --allow-origin added.
	if status, message := editRequestGuard(r.WithContext(context.Background())); status != 0 {
		writeEditJSON(w, status, editError{Error: message})
		return
	}
	var change settingsChange
	if err := decodeOneJSON(w, r, 4<<10, &change); err != nil || len(change.Value) == 0 {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: `send {"key", "value"} and, optionally, "expected": the value the page showed`})
		return
	}
	value, err := decodeSettingValue(change.Key, change.Value)
	if err != nil {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: err.Error()})
		return
	}
	if value.loosens && !h.proven(r) {
		message := "This change makes approvals less strict. To change this here, run agnostic-ai view --scope user again, or use the companion. You can also edit the home config."
		if change.Key == settingCompanionCheckForUpdates {
			message = "Turning off update checks needs a page opened with agnostic-ai view --scope user, or the companion's menu."
		}
		writeEditJSON(w, http.StatusForbidden, editError{Error: message})
		return
	}
	var snap *homeSnapshot
	if strings.HasPrefix(change.Key, "approvals.") {
		// The check against expected and the write read one snapshot,
		// under the lock every in-app home config writer takes.
		source, err := h.reader.source()
		if err != nil {
			writeEditJSON(w, http.StatusInternalServerError, editError{Error: err.Error()})
			return
		}
		release, err := lockApprovalsWrites(source)
		if err != nil {
			writeEditJSON(w, http.StatusConflict, settingsRefusal{Error: err.Error(), Settings: new(h.document(r))})
			return
		}
		defer release()
		read, err := readHomeSnapshot(source)
		if err != nil {
			writeEditJSON(w, http.StatusConflict, settingsRefusal{Error: err.Error(), Settings: new(h.document(r))})
			return
		}
		snap = &read
	}
	if _, ok := companionSwitchFields[change.Key]; ok {
		dir, err := h.companionDir()
		if err != nil {
			writeEditJSON(w, http.StatusNotFound, editError{Error: err.Error()})
			return
		}
		release, err := lockCompanionSettings(filepath.Join(dir, companionSettingsFile))
		if err != nil {
			writeEditJSON(w, http.StatusConflict, settingsRefusal{Error: err.Error(), Settings: new(h.document(r))})
			return
		}
		defer release()
	}
	doc := h.document(r)
	current, ok := currentSetting(doc, change.Key)
	if snap != nil {
		current, ok = snap.approvalsSetting(doc.ConfigRoot, change.Key)
	}
	if !ok {
		writeEditJSON(w, http.StatusNotFound, editError{Error: "This setting is not available on this server."})
		return
	}
	if len(change.Expected) > 0 && !sameJSONValue(change.Expected, current) {
		writeEditJSON(w, http.StatusConflict, settingsRefusal{Error: "This setting changed since the page read it. Check the current value.", Settings: &doc})
		return
	}
	changed, err := h.apply(r.Context(), snap, change.Key, value.value, h.proven(r))
	if errors.Is(err, errBrokenBlockNeedsProof) {
		writeEditJSON(w, http.StatusForbidden, settingsRefusal{Error: "The approvals block in the home config does not load. Choose Ask me, fix the home config, or make the change from the companion or a page opened with agnostic-ai view --scope user.", Settings: new(h.document(r))})
		return
	}
	if errors.Is(err, errSettingNeedsProof) {
		writeEditJSON(w, http.StatusForbidden, settingsRefusal{Error: "This change would turn on automatic approvals. To change this here, run agnostic-ai view --scope user again, or use the companion.", Settings: new(h.document(r))})
		return
	}
	if err != nil {
		writeEditJSON(w, http.StatusConflict, settingsRefusal{Error: err.Error(), Settings: new(h.document(r))})
		return
	}
	if value.loosens {
		h.logChange(doc.ConfigRoot, h.askedBy(r), "set", change.Key+": "+fmt.Sprint(value.value))
	}
	after := h.document(r)
	for i, path := range changed {
		changed[i] = overviewDisplay(after.ConfigRoot, path)
	}
	writeEditJSON(w, http.StatusOK, settingsWriteReply{Settings: after, Changed: changed})
}

func sameJSONValue(raw json.RawMessage, value any) bool {
	var got any
	if json.Unmarshal(raw, &got) != nil {
		return false
	}
	return got == value
}

// apply writes one decoded change and returns the files it changed.
func (h *settingsHandler) apply(ctx context.Context, snap *homeSnapshot, key string, value any, proven bool) ([]string, error) {
	if key == settingMailboxApprove {
		store, _ := h.mailbox()
		return []string{store.SettingsPath()}, store.SaveSettings(ctx, mailbox.Settings{ApproveAgentSends: value == true})
	}
	if _, ok := companionSwitchFields[key]; ok {
		dir, err := h.companionDir()
		if err != nil {
			return nil, err
		}
		path := filepath.Join(dir, companionSettingsFile)
		return []string{path}, writeCompanionSwitch(path, key, value == true)
	}
	source, err := h.reader.source()
	if err != nil {
		return nil, err
	}
	if snap == nil {
		return nil, fmt.Errorf("%s needs the home config read under the approvals lock", key)
	}
	return writeApprovalsSetting(source, *snap, strings.TrimPrefix(key, "approvals."), value, proven)
}

// logChange adds a loosening write or a sync to the approvals decision
// log. A log that cannot be written does not undo the change.
func (h *settingsHandler) logChange(source, by, call, detail string) {
	if source == "" {
		return
	}
	entry := approvals.Entry{Time: h.now().UTC(), Tool: "settings", Call: call, Detail: detail, Mode: "-", Decision: approvals.Changed, By: by}
	if home, err := config.LoadHomeApprovals(source); err == nil {
		entry.Mode = cmpMode(home.Mode)
	}
	if err := (approvals.Log{Path: approvals.LogPath(source)}).Append(entry); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
}

// runInProcess runs agnostic-ai with args in this process, as view runs
// serve, and returns its standard output. The caller holds serializeAPI.
// The root command's pre-run sets package state, which this server keeps
// using, so it is put back after.
func (h *settingsHandler) runInProcess(ctx context.Context, args ...string) ([]byte, error) {
	prevVerbosity, prevWarnOut, prevWarned := verbosity, requiresWarnOut, requiresWarned
	defer func() { verbosity, requiresWarnOut, requiresWarned = prevVerbosity, prevWarnOut, prevWarned }()
	var out, errOut bytes.Buffer
	root := NewRootCmd(h.version)
	root.SetContext(ctx)
	root.SetIn(strings.NewReader(""))
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		if text := strings.TrimSpace(errOut.String()); text != "" {
			return out.Bytes(), fmt.Errorf("%w\n%s", err, text)
		}
		return out.Bytes(), err
	}
	return out.Bytes(), nil
}
