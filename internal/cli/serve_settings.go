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

// settingsView is the user settings both the companion menu and the web
// page show. Every read builds it from the files, so two servers on one
// disk agree.
type settingsView struct {
	SchemaVersion string            `json:"schemaVersion"`
	ConfigRoot    string            `json:"configRoot"`
	Approvals     settingsApprovals `json:"approvals"`
	Mailbox       *settingsMailbox  `json:"mailbox,omitempty"`
	Sync          settingsSync      `json:"sync"`
	// CanLoosen says whether this request carried proof that the person
	// asked, so the page can lock the controls that loosen.
	CanLoosen bool `json:"canLoosen"`
}

type settingsApprovals struct {
	// Mode is the approvals.mode the home config asks for.
	Mode string `json:"mode"`
	// ModeFile is the home file, relative to configRoot, whose approvals
	// block is in effect; empty when neither file has one.
	ModeFile  string            `json:"modeFile"`
	Companion bool              `json:"companion"`
	Overrides settingsOverrides `json:"overrides"`
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
	// secret is the companion's; a loosening write needs it.
	secret  companionSecret
	version string
	// runSync runs agnostic-ai with args in process and returns what it
	// printed; tests replace it.
	runSync func(ctx context.Context, args ...string) ([]byte, error)
	now     func() time.Time
}

func newSettingsHandler(userLevel bool, reader *approvalsReader, marks func() string, secret companionSecret, version string) *settingsHandler {
	h := &settingsHandler{userLevel: userLevel, reader: reader, marks: marks, secret: secret, version: version, now: time.Now}
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

// proven reports whether r carries proof that the person asked: the
// secret of the companion that started this server.
func (h *settingsHandler) proven(r *http.Request) bool {
	return h.secret.allows(r)
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
	doc.Sync = approvalsSyncNeed(home, status, slices.Contains(builtins, config.ApprovalsBuiltin))
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

// approvalsSyncNeed says whether a sync --scope user would put the asked
// approvals in effect: the hook is not installed, approvals is missing
// from builtins, or a native reviewer key is missing or left over.
func approvalsSyncNeed(home config.ApprovalsConfig, status approvalsStatusOutput, hasBuiltin bool) settingsSync {
	out := settingsSync{Reasons: []string{}}
	if (home.Loosens() || home.Companion) && !hasBuiltin {
		out.Reasons = append(out.Reasons, "approvals is not in builtins in the home config")
	}
	review := false
	for _, tool := range status.Tools {
		review = review || tool.Mode == config.ApprovalsReview
		if tool.Mode == config.ApprovalsAsk {
			// The companion buttons need the hook even while the tool asks.
			if home.Companion && (tool.Tool == "claude" || tool.Tool == "codex") {
				if problem := approvalHookProblem(tool.Tool); strings.HasPrefix(problem, syncFixedReasons[0]) {
					out.Reasons = append(out.Reasons, tool.Tool+": "+problem)
				}
			}
			continue
		}
		if tool.Effective != config.ApprovalsAsk {
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
	paths := userConfigPaths(source)
	shared, err := readUserConfig(paths[0])
	if err != nil {
		return "", nil, err
	}
	local, err := readUserConfig(paths[1])
	if err != nil {
		return "", nil, err
	}
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
func approvalsWriteFile(source string) (string, error) {
	file, _, err := approvalsBlockLayout(source)
	if err != nil {
		return "", err
	}
	paths := userConfigPaths(source)
	if file == config.ConfigFileName {
		return paths[0], nil
	}
	return paths[1], nil
}

// writeApprovalsSetting sets one approvals key in the home config in
// source and returns the files it changed. Ask me writes what the Ask me
// chip writes. A value that needs the hook adds approvals to builtins
// when it is missing; when that fails, the approvals change is undone.
func writeApprovalsSetting(source, key string, value any) ([]string, error) {
	if key == "mode" && value == config.ApprovalsAsk {
		path := userConfigPaths(source)[1]
		return []string{path}, writeApprovalsAsk(path)
	}
	path, err := approvalsWriteFile(source)
	if err != nil {
		return nil, err
	}
	_, loadErr := config.LoadHomeApprovals(source)
	restore, err := restorePoint(path)
	if err != nil {
		return nil, err
	}
	if err := editApprovalsFile(path, func(data []byte) ([]byte, error) { return approvalsSetYAML(data, key, value) }); err != nil {
		return nil, err
	}
	home, err := config.LoadHomeApprovals(source)
	switch {
	case err != nil && loadErr == nil:
		return nil, errors.Join(fmt.Errorf("the change would break the approvals block, so it was not kept: %w", err), restore())
	case err == nil && !approvalsHolds(home, key, value):
		return nil, errors.Join(fmt.Errorf("%s: the change would not take effect, so it was not kept; set approvals.%s by hand", path, key), restore())
	}
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

// restorePoint returns a func that puts path back as it is now: its
// bytes, or no file.
func restorePoint(path string) (func() error, error) {
	before, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return func() error {
			if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			return nil
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return func() error { return atomicfile.Write(path, before, 0o644) }, nil
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
// makes.
func addApprovalsBuiltin(source string) (string, error) {
	edits := &editHandler{root: source, userLevel: true}
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
		var on bool
		if err := json.Unmarshal(raw, &on); err != nil {
			return settingValue{}, fmt.Errorf("%s takes true or false", key)
		}
		return settingValue{value: on, loosens: on == (key == settingApprovalsCompanion)}, nil
	}
	return settingValue{}, fmt.Errorf("key must be %s, %s or %s", settingApprovalsMode, settingApprovalsCompanion, settingMailboxApprove)
}

// currentSetting is key's value in doc, as the page shows it.
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
		writeEditJSON(w, http.StatusForbidden, editError{Error: "This change makes approvals less strict, so only the companion can make it here. Use the companion's settings menu, or edit the home config."})
		return
	}
	doc := h.document(r)
	current, ok := currentSetting(doc, change.Key)
	if !ok {
		writeEditJSON(w, http.StatusNotFound, editError{Error: "The mailbox is not available on this server."})
		return
	}
	if len(change.Expected) > 0 && !sameJSONValue(change.Expected, current) {
		writeEditJSON(w, http.StatusConflict, settingsRefusal{Error: "This setting changed since the page read it. Check the current value.", Settings: &doc})
		return
	}
	changed, err := h.apply(r.Context(), change.Key, value.value)
	if err != nil {
		writeEditJSON(w, http.StatusConflict, settingsRefusal{Error: err.Error(), Settings: ptr(h.document(r))})
		return
	}
	if value.loosens {
		h.logChange(doc.ConfigRoot, "set", change.Key+": "+fmt.Sprint(value.value))
	}
	after := h.document(r)
	for i, path := range changed {
		changed[i] = overviewDisplay(after.ConfigRoot, path)
	}
	writeEditJSON(w, http.StatusOK, settingsWriteReply{Settings: after, Changed: changed})
}

func ptr[T any](v T) *T { return &v }

func sameJSONValue(raw json.RawMessage, value any) bool {
	var got any
	if json.Unmarshal(raw, &got) != nil {
		return false
	}
	return got == value
}

// apply writes one decoded change and returns the files it changed.
func (h *settingsHandler) apply(ctx context.Context, key string, value any) ([]string, error) {
	if key == settingMailboxApprove {
		store, _ := h.mailbox()
		return []string{store.SettingsPath()}, store.SaveSettings(ctx, mailbox.Settings{ApproveAgentSends: value == true})
	}
	source, err := h.reader.source()
	if err != nil {
		return nil, err
	}
	return writeApprovalsSetting(source, strings.TrimPrefix(key, "approvals."), value)
}

// logChange adds a loosening write or a sync to the approvals decision
// log. A log that cannot be written does not undo the change.
func (h *settingsHandler) logChange(source, call, detail string) {
	if source == "" {
		return
	}
	entry := approvals.Entry{Time: h.now().UTC(), Tool: "settings", Call: call, Detail: detail, Mode: "-", Decision: approvals.Changed, By: settingsByCompanion}
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
