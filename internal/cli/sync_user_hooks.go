package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/augment"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claude"
	"github.com/chemaclass/agnostic-ai/internal/adapters/cursor"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// errUserFileUnchanged reports that a merge leaves an existing file's
// content as it is, so sync must neither rewrite nor remove it.
var errUserFileUnchanged = errors.New("user file unchanged")

// mergeUserHooks returns the hooks file with the managed entries
// replaced, nil when the file should not exist, or errUserFileUnchanged
// when its parsed content would not change. A rewrite keeps the file's
// key order and indent. A recorded entry the file no longer holds counts
// as removed, with a warning, so a reset settings file is rebuilt. One
// still there with the same matcher and command but other fields was
// edited by hand, and stops the run: replacing it would run it twice.
// An unrecorded entry exactly as sync would write it satisfies the
// source and stays the user's, since nothing in it shows sync wrote it.
// One with the same matcher and command but other fields stops the run.
// An empty container goes only when created, the state's record of what
// sync made, lists it, so one the user wrote stays.
func mergeUserHooks(path, format string, target userHookTarget, entries []spec.Entry, previous, next map[string][]any, created []string, legacy bool, warn io.Writer) (userHooksMerge, error) {
	var merge userHooksMerge
	data, err := mergeUserHooksData(path, format, target, entries, previous, next, created, legacy, &merge, warn)
	merge.data = data
	return merge, err
}

// userHooksMerge is a hooks file as one sync run leaves it.
type userHooksMerge struct {
	// data is the new file, or nil when the file should not exist.
	data []byte
	// read is the file as the merge read it.
	read *diskSnapshot
	// created is what of the file sync created, as
	// userState.HookContainers records it.
	created []string
}

func mergeUserHooksData(path, format string, target userHookTarget, entries []spec.Entry, previous, next map[string][]any, created []string, legacy bool, merge *userHooksMerge, warn io.Writer) ([]byte, error) {
	f, err := readUserHooksFile(path, format, previous, created, legacy, merge)
	if err != nil {
		return nil, err
	}
	if format == "copilot" && !f.absent && f.doc["version"] != float64(1) {
		return nil, fmt.Errorf("%s: Copilot user hooks require integer version 1", path)
	}
	// Nothing managed, nothing previously managed, no file: leave the
	// user's home directory untouched rather than seeding an empty one.
	if f.absent && len(entries) == 0 && len(previous) == 0 {
		return nil, nil
	}
	f.target, f.next, f.warn = target, next, warn
	f.rawHooks, f.hadHooks = f.doc["hooks"]
	f.hooks, _ = f.rawHooks.(map[string]any)
	if f.hooks == nil {
		f.hooks = map[string]any{}
	}
	planned, err := f.plannedHooks(entries)
	if err != nil {
		return nil, err
	}
	if err := f.reconcilePrevious(previous, planned); err != nil {
		return nil, err
	}
	told, err := f.placeHooks(planned)
	if err != nil {
		return nil, err
	}
	restore := f.settleContainers(told, merge)
	// Nothing of ours left and nothing of the user's either: drop a file
	// sync created rather than leave a shell behind.
	if len(f.doc) == 0 && f.absent {
		merge.created = nil
		return nil, nil
	}
	if !f.absent {
		return f.rewriteExisting(restore, merge)
	}
	return f.writeNew(told)
}

// plannedUserHook is one entry as sync writes it now, one per command.
type plannedUserHook struct {
	event       string
	item, plain any
	spec        bool
}

// userHooksFile is a hooks file as one merge reads and edits it.
type userHooksFile struct {
	path, format string
	target       userHookTarget
	data         []byte
	absent       bool
	// doc is the file the merge edits; before is the file as read.
	doc, before map[string]any
	ordered     *adapters.OrderedJSON
	rawHooks    any
	hadHooks    bool
	hooks       map[string]any
	// created is what the state records sync made; made grows with what
	// this merge makes.
	created, made []string
	legacy        bool
	// next collects the managed entries this merge writes, per event.
	next map[string][]any
	warn io.Writer
}

func (f *userHooksFile) owns(token string) bool {
	return f.legacy || slices.Contains(f.created, token)
}

func (f *userHooksFile) mark(token string) {
	if !slices.Contains(f.made, token) {
		f.made = append(f.made, token)
	}
}

// readUserHooksFile reads and parses the hooks file at path.
func readUserHooksFile(path, format string, previous map[string][]any, created []string, legacy bool, merge *userHooksMerge) (*userHooksFile, error) {
	f := &userHooksFile{path: path, format: format, doc: map[string]any{}, before: map[string]any{}, ordered: adapters.NewOrderedJSON(), created: created, made: slices.Clone(created)}
	data, err := os.ReadFile(path)
	f.data, f.absent = data, os.IsNotExist(err)
	merge.read = &diskSnapshot{data: data, absent: f.absent}
	// Version 5 state treated every empty container as sync's, which
	// holds only for a file that already carried its hooks.
	f.legacy = legacy && len(previous) > 0
	if f.legacy {
		for _, token := range []string{"<file>", "hooks", "env", "version"} {
			f.mark(token)
		}
		for event := range previous {
			f.mark("hooks." + event)
		}
	}
	if f.absent {
		f.mark("<file>")
	}
	if err == nil {
		// The text edit keeps comments, so only the parse drops them.
		stripped, _ := adapters.StripJSONC(data)
		if err := json.Unmarshal(stripped, &f.doc); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if err := json.Unmarshal(stripped, &f.before); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if err := json.Unmarshal(stripped, f.ordered); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	} else if !f.absent {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return f, nil
}

// plannedHooks renders each entry in the file's native form.
func (f *userHooksFile) plannedHooks(entries []spec.Entry) ([]plannedUserHook, error) {
	var planned []plannedUserHook
	var claudeHooks []spec.Entry
	for i, entry := range entries {
		event, _ := entry.Meta["event"].(string)
		if event == "" || f.format == "augment" && !slices.Contains(augment.HookEvents(), event) {
			continue
		}
		fromSpec := i < f.target.specHooks
		if f.format == "copilot" {
			items, err := f.plannedCopilotHooks(entry, event, fromSpec)
			if err != nil {
				return nil, err
			}
			planned = append(planned, items...)
			continue
		}
		args := stringSliceFromAny(entry.Meta["args"])
		wraps := entry.WrapsCommand(f.target.name)
		commands := userHookCommands(entry.Meta["command"])
		if f.target.name == "claude" && len(commands) > 0 {
			hook := entry
			hook.Meta = maps.Clone(entry.Meta)
			hook.Meta["type"] = "command"
			claudeHooks = append(claudeHooks, hook)
		}
		for _, command := range commands {
			item, plain := f.nativeHook(entry, event, command, args, wraps)
			planned = append(planned, plannedUserHook{event: event, item: item, plain: plain, spec: fromSpec})
		}
	}
	claude.NoteFailClosedNoOps(claudeHooks)
	claude.NoteHookEventNoOps(claudeHooks)
	return planned, nil
}

func (f *userHooksFile) plannedCopilotHooks(entry spec.Entry, event string, fromSpec bool) ([]plannedUserHook, error) {
	native, err := userCopilotHookEntries(entry, f.target.scriptsDir)
	if err != nil {
		return nil, fmt.Errorf("render %s hooks: %w", f.path, err)
	}
	var planned []plannedUserHook
	for _, item := range native[event] {
		plain := jsonRoundTrip(item)
		dropHookTargetEnv(plain.(map[string]any), "copilot")
		adopted, err := adoptUserCopilotHook(f.path, event, item, plain, entry.Meta)
		if err != nil {
			return nil, err
		}
		if !adopted {
			planned = append(planned, plannedUserHook{event: event, item: item, plain: plain, spec: fromSpec})
		}
	}
	return planned, nil
}

// nativeHook renders one command of entry as the file's format writes it,
// and plain, the same entry without the target signal.
func (f *userHooksFile) nativeHook(entry spec.Entry, event, command string, args []string, wraps bool) (item, plain any) {
	target := f.target
	source := command
	rewrite := func(command string, meta ...map[string]any) string {
		if target.scriptsDir != "" {
			return adapters.RewriteUserHookPath(command, target.name, filepath.ToSlash(target.scriptsDir), meta...)
		}
		return adapters.RewriteUserHookRoot(command, target.name, meta...)
	}
	command = rewrite(command, entry.Meta)
	// The wrapper runs a command line, so args fold in first.
	if wraps {
		command = adapters.PortableHookCommand(entry, target.name, adapters.ExecFormCommand(command, args), func(path string) string { return rewrite(path) })
	}
	switch f.format {
	case "augment":
		// Augment documents type, command, args, and timeout (ms)
		// on a hook, and a matcher only on tool events. Without
		// args, Auggie hands the command to cmd.exe on Windows.
		if target.goos == "windows" && len(args) == 0 && !wraps && target.scriptsDir != "" {
			command = adapters.RewriteUserHookRoot(adapters.RewriteUserWindowsHookPath(source, filepath.ToSlash(target.scriptsDir)), target.name, entry.Meta)
		}
		commandHook := map[string]any{"type": "command", "command": command}
		if len(args) > 0 && !wraps {
			commandHook["args"] = args
		}
		if timeout, ok := augment.HookTimeout(entry.Meta); ok {
			commandHook["timeout"] = timeout
		}
		group := map[string]any{"hooks": []any{commandHook}}
		if matcher, _ := entry.Meta["matcher"].(string); matcher != "" && !augment.SessionOnlyEvent(event) {
			group["matcher"] = matcher
		}
		item, plain = group, group
		// An empty matcher means the same as none, so a hand-written
		// `"matcher": ""` entry satisfies the spec.
		if _, has := group["matcher"]; !has {
			plain = map[string]any{"matcher": "", "hooks": []any{maps.Clone(commandHook)}}
		}
	case "claude":
		commandHook := map[string]any{"type": "command", "command": command}
		for _, key := range []string{"timeout", "statusMessage", "async", "asyncRewake", "shell", "if", "once"} {
			if value, ok := entry.Meta[key]; ok {
				commandHook[key] = value
			}
		}
		if target.name == "claude" && metaTrue(entry.Meta["failClosed"]) {
			commandHook["onFailure"] = "block"
		}
		if len(args) > 0 && !wraps {
			switch {
			case target.args:
				commandHook["args"] = args
			case target.foldArgs:
				commandHook["command"] = adapters.ExecFormCommand(command, args)
			}
		}
		if target.timeout != nil {
			delete(commandHook, "timeout")
			if timeout, ok := target.timeout(entry.Meta); ok {
				commandHook["timeout"] = timeout
			}
		}
		matcher, _ := entry.Meta["matcher"].(string)
		plain = map[string]any{"matcher": matcher, "hooks": []any{maps.Clone(commandHook)}}
		target.tell(commandHook, entry.Meta)
		item = map[string]any{"matcher": matcher, "hooks": []any{commandHook}}
	default:
		if target.foldArgs && !wraps {
			command = adapters.ExecFormCommand(command, args)
		}
		cursorHook := map[string]any{"command": command}
		for _, key := range []string{"matcher", "timeout", "loop_limit", "failClosed"} {
			if value, ok := entry.Meta[key]; ok {
				cursorHook[key] = value
			}
		}
		item, plain = cursorHook, cursorHook
	}
	return item, plain
}

// reconcilePrevious takes out of the file each entry the last sync
// recorded, warning about one that is gone and stopping on one edited
// by hand.
func (f *userHooksFile) reconcilePrevious(previous map[string][]any, planned []plannedUserHook) error {
	// writesNow reports that an edited copy of a recorded entry is what
	// sync writes now. The form without the target signal counts only
	// for a record that lacked it too, as versions before it wrote:
	// removing the signal from a record that had it is an edit.
	writesNow := func(event string, existing, recorded any) bool {
		return slices.ContainsFunc(planned, func(p plannedUserHook) bool {
			if p.event != event {
				return false
			}
			return reflect.DeepEqual(existing, jsonRoundTrip(p.item)) ||
				(!carriesHookTarget(recorded) && reflect.DeepEqual(existing, jsonRoundTrip(p.plain)))
		})
	}
	hooks := f.hooks
	for _, event := range slices.Sorted(maps.Keys(previous)) {
		current, _ := hooks[event].([]any)
		var lost []any
		for _, oldEntry := range previous[event] {
			var found bool
			if current, found = removeEqual(current, oldEntry); !found {
				lost = append(lost, oldEntry)
			}
		}
		// An edit that matches what sync writes now, such as a timeout
		// an older version wrote in the wrong unit and the user fixed,
		// is not a conflict: it goes, and the planned entry replaces it.
		stillLost := lost[:0]
		for _, oldEntry := range lost {
			edited := slices.IndexFunc(current, func(item any) bool {
				return sameUserHook(item, oldEntry) || extendsUserHook(item, oldEntry)
			})
			switch {
			case edited < 0:
				stillLost = append(stillLost, oldEntry)
			case writesNow(event, current[edited], oldEntry):
				current = slices.Delete(current, edited, edited+1)
			default:
				return fmt.Errorf("%s: managed %s hook was edited; restore it or remove it, then sync", f.path, event)
			}
		}
		lost = stillLost
		if len(lost) > 0 {
			if _, err := fmt.Fprintf(f.warn, "warning: %s: managed %s hook is missing, so it counts as removed\n", f.path, event); err != nil {
				return fmt.Errorf("write user hook warning: %w", err)
			}
		}
		switch {
		case len(current) > 0:
			hooks[event] = current
		case f.owns("hooks." + event):
			delete(hooks, event)
		case f.owns("hooks." + event + "=null"):
			hooks[event] = nil
		default:
			if _, isList := hooks[event].([]any); isList {
				hooks[event] = []any{}
			}
		}
	}
	return nil
}

// placeHooks adds each planned entry to the file. told reports that a
// spec hook is in the file, written by sync or by the user, so the target
// signal outside the entries (settings env, session hook) is due. An
// adopted entry keeps its own form, so a codex, gemini, or qoder hook the
// user wrote gets no signal.
func (f *userHooksFile) placeHooks(planned []plannedUserHook) (told bool, err error) {
	hooks := f.hooks
	unrecorded := map[string][]any{}
	for event, items := range hooks {
		current, _ := items.([]any)
		unrecorded[event] = slices.Clone(current)
	}
	// place adds one managed item unless an unrecorded entry already
	// matches it, or matches plain, the item without the target signal:
	// a hook the user wrote stays exactly as written. Either way the
	// source hook is in the file.
	place := func(event string, item, plain any) error {
		for _, want := range []any{item, plain} {
			var satisfied bool
			if unrecorded[event], satisfied = removeEqual(unrecorded[event], jsonRoundTrip(want)); satisfied {
				return nil
			}
		}
		if slices.ContainsFunc(unrecorded[event], func(existing any) bool {
			return sameUserHook(existing, item) || sameUserHook(existing, plain) || (f.format == "copilot" && sameUserCopilotHandler(existing, item))
		}) {
			return fmt.Errorf("%s: a %s hook not recorded as managed runs a source hook's matcher and command with other settings; remove that entry, or give the command its own entry that matches the source, then sync", f.path, event)
		}
		if value, ok := hooks[event]; !ok {
			f.mark("hooks." + event)
		} else if value == nil {
			f.mark("hooks." + event + "=null")
		}
		current, _ := hooks[event].([]any)
		hooks[event] = append(current, item)
		f.next[event] = append(f.next[event], item)
		return nil
	}
	for _, p := range planned {
		if err := place(p.event, p.item, p.plain); err != nil {
			return false, err
		}
		told = told || p.spec
	}
	if f.target.mode == hookTargetSessionEnv && told {
		item := map[string]any{"command": cursor.HookTargetCommand}
		if err := place(cursor.HookTargetEvent, item, item); err != nil {
			return false, err
		}
	}
	return told, nil
}

// settleContainers keeps or drops the containers around the hooks and
// records in merge what of the file sync created. It returns the inside
// each emptied container the user wrote gets back.
func (f *userHooksFile) settleContainers(told bool, merge *userHooksMerge) map[string]string {
	doc, hooks := f.doc, f.hooks
	keepUserContainer(doc, "hooks", f.rawHooks, f.hadHooks, hooks, f.owns, f.mark)
	if f.target.mode == hookTargetSettingsEnv {
		f.target.setSettingsEnv(doc, told, f.owns, f.mark)
	}
	// Cursor's schema version goes with the last managed hook when sync
	// added it.
	if f.format == "cursor" || f.format == "copilot" {
		_, hasVersion := doc["version"]
		switch {
		case len(f.next) > 0:
			if !hasVersion {
				f.mark("version")
			}
			doc["version"] = float64(1)
		case hasVersion && f.owns("version") && (f.format != "copilot" || len(hooks) == 0):
			delete(doc, "version")
		}
	}
	// An empty container the user wrote gets back its own inside, such
	// as a line break, once sync's entries leave it: the text alone
	// cannot tell that break from the one an insert adds.
	for _, key := range userHookContainerKeys(f.before) {
		was, _ := userHookContainer(f.before, key)
		now, _ := userHookContainer(doc, key)
		if emptyJSONContainer(was) && !emptyJSONContainer(now) && now != nil {
			if inner, ok := jsonContainerInner(string(f.data), key); ok {
				f.mark(userInnerToken(key, inner))
			}
		}
	}
	restore := map[string]string{}
	for _, token := range f.made {
		key, inner, ok := parseUserInnerToken(token)
		if now, present := userHookContainer(doc, key); ok && present && emptyJSONContainer(now) {
			restore[key] = inner
		}
	}
	merge.created = slices.DeleteFunc(f.made, func(token string) bool {
		if key, _, ok := parseUserInnerToken(token); ok {
			now, present := userHookContainer(doc, key)
			return !present || emptyJSONContainer(now)
		}
		key, null := strings.CutSuffix(token, "=null")
		if token == "<file>" {
			return false
		}
		if event, ok := strings.CutPrefix(key, "hooks."); ok {
			value, kept := hooks[event]
			_, isObject := doc["hooks"].(map[string]any)
			return !kept || !isObject || (null && value == nil)
		}
		value, kept := doc[key]
		return !kept || (null && value == nil)
	})
	slices.Sort(merge.created)
	// With no managed hook and no container of sync's left, what remains
	// is the user's, file included.
	if len(f.next) == 0 && slices.Equal(merge.created, []string{"<file>"}) {
		merge.created = nil
	}
	return restore
}

// rewriteExisting edits the file's text in place, or reports it unchanged.
func (f *userHooksFile) rewriteExisting(restore map[string]string, merge *userHooksMerge) ([]byte, error) {
	// Round-trip so spec integers compare equal to decoded JSON numbers.
	raw, err := json.Marshal(f.doc)
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", f.path, err)
	}
	var merged map[string]any
	if err := json.Unmarshal(raw, &merged); err != nil {
		return nil, fmt.Errorf("parse %s: %w", f.path, err)
	}
	out := f.data
	if !reflect.DeepEqual(merged, f.before) {
		if out, err = editUserHooksText(f.path, f.data, f.before, merged, restore); err != nil {
			return nil, err
		}
	}
	// A comment left after the edit is the user's, so it keeps the
	// file; one inside what the edit removed went with it.
	if _, commented := adapters.StripJSONC(out); len(f.doc) == 0 && f.owns("<file>") && !commented {
		merge.created = nil
		return nil, nil
	}
	if reflect.DeepEqual(merged, f.before) {
		return nil, errUserFileUnchanged
	}
	return out, nil
}

// writeNew encodes a file sync creates.
func (f *userHooksFile) writeNew(told bool) ([]byte, error) {
	ordered := f.ordered
	for _, key := range []string{"version", "hooks"} {
		value, ok := f.doc[key]
		if !ok {
			ordered.Delete(key)
			continue
		}
		if key == "hooks" {
			original, _ := ordered.Get("hooks")
			raw, err := orderedHooksRaw(original, f.hooks)
			if err != nil {
				return nil, fmt.Errorf("marshal %s: %w", f.path, err)
			}
			ordered.SetRaw("hooks", raw)
			continue
		}
		if err := ordered.Set(key, value); err != nil {
			return nil, fmt.Errorf("marshal %s: %w", f.path, err)
		}
	}
	if f.target.mode == hookTargetSettingsEnv {
		if err := adapters.SetHookTargetEnv(ordered, f.target.name, told); err != nil {
			return nil, fmt.Errorf("%s: %w", f.path, err)
		}
	}
	out, err := adapters.MarshalJSONIndentWith(ordered, adapters.DetectJSONIndent(f.data))
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", f.path, err)
	}
	return append(out, '\n'), nil
}

// editUserHooksText turns the existing file's text from before into
// after by editing only what sync owns: the schema version, the target
// variable in env, each changed hooks event, and within an event each
// entry. Every other byte stays as written, so removing what an earlier
// run added gives back the original file.
func editUserHooksText(path string, data []byte, before, after map[string]any, restore map[string]string) ([]byte, error) {
	var order, remove []string
	set := map[string]any{}
	edit := func(key string, was, now any, had, has bool) {
		switch {
		case !has && had:
			remove = append(remove, key)
		case has && (!had || !reflect.DeepEqual(was, now)):
			order = append(order, key)
			set[key] = now
		}
	}
	version, hadVersion := before["version"]
	newVersion, hasVersion := after["version"]
	edit("version", version, newVersion, hadVersion, hasVersion)

	// A dot would split an event name into a key path.
	dotted := func(events map[string]any) bool {
		return slices.ContainsFunc(slices.Collect(maps.Keys(events)), func(event string) bool { return strings.Contains(event, ".") })
	}
	oldHooks, hadHooks := before["hooks"].(map[string]any)
	newHooks, hasHooks := after["hooks"].(map[string]any)
	var events []string
	switch {
	case hadHooks && hasHooks && !dotted(oldHooks) && !dotted(newHooks):
		for _, event := range slices.Sorted(maps.Keys(oldHooks)) {
			if _, ok := newHooks[event]; !ok {
				remove = append(remove, "hooks."+event)
			}
		}
		for _, event := range slices.Sorted(maps.Keys(newHooks)) {
			was, had := oldHooks[event]
			now := newHooks[event]
			_, wasList := was.([]any)
			_, isList := now.([]any)
			if had && wasList && isList {
				if !reflect.DeepEqual(was, now) {
					events = append(events, event)
				}
				continue
			}
			edit("hooks."+event, was, now, had, true)
		}
	default:
		_, hadKey := before["hooks"]
		_, hasKey := after["hooks"]
		edit("hooks", before["hooks"], after["hooks"], hadKey, hasKey)
	}

	oldEnv, oldIsObject := before["env"].(map[string]any)
	newEnv, newIsObject := after["env"].(map[string]any)
	_, hadEnv := before["env"]
	_, hasEnv := after["env"]
	if hasEnv && (oldIsObject || !hadEnv) && newIsObject {
		was, had := oldEnv[adapters.HookTargetEnv]
		now, has := newEnv[adapters.HookTargetEnv]
		edit("env."+adapters.HookTargetEnv, was, now, had, has)
	} else {
		edit("env", before["env"], after["env"], hadEnv, hasEnv)
	}

	// Sets go first, so a parent such as hooks that loses its last old
	// member while gaining a new one keeps its place. A removal keeps the
	// parent: merge already dropped every container sync created.
	out, err := editJSONRootIndent(path, data, "  ", order, set, nil)
	if err != nil {
		return nil, err
	}
	for _, key := range remove {
		if out, err = removeJSONKey(path, out, key); err != nil {
			return nil, err
		}
	}
	for _, event := range events {
		items, _ := newHooks[event].([]any)
		if out, err = editJSONArray(path, out, "hooks."+event, items); err != nil {
			return nil, err
		}
	}
	for _, key := range slices.Sorted(maps.Keys(restore)) {
		if out, err = setJSONContainerInner(path, out, key, restore[key]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// userHookContainerKeys lists the containers of a hooks file whose
// inside sync may change: the root (""), hooks, env, and each event.
func userHookContainerKeys(doc map[string]any) []string {
	keys := []string{"", "hooks", "env"}
	hooks, _ := doc["hooks"].(map[string]any)
	for _, event := range slices.Sorted(maps.Keys(hooks)) {
		if !strings.Contains(event, ".") {
			keys = append(keys, "hooks."+event)
		}
	}
	return keys
}

// userHookContainer is the value at a userHookContainerKeys key.
func userHookContainer(doc map[string]any, key string) (any, bool) {
	if key == "" {
		return doc, true
	}
	if event, ok := strings.CutPrefix(key, "hooks."); ok {
		hooks, _ := doc["hooks"].(map[string]any)
		value, ok := hooks[event]
		return value, ok
	}
	value, ok := doc[key]
	return value, ok
}

func emptyJSONContainer(v any) bool {
	switch v := v.(type) {
	case map[string]any:
		return len(v) == 0
	case []any:
		return len(v) == 0
	}
	return false
}

// userInnerToken records in userState.HookContainers what the empty
// container at key held between its brackets.
func userInnerToken(key, inner string) string {
	return "inner:" + key + "=" + jsonString(inner)
}

func parseUserInnerToken(token string) (key, inner string, ok bool) {
	rest, ok := strings.CutPrefix(token, "inner:")
	if !ok {
		return "", "", false
	}
	key, quoted, ok := strings.Cut(rest, "=")
	if !ok || json.Unmarshal([]byte(quoted), &inner) != nil {
		return "", "", false
	}
	return key, inner, true
}

// orderedHooksRaw renders the hooks object keeping the file's event
// order and each unchanged entry's own bytes, so a rewrite does not
// reorder keys inside hooks the user wrote. New events and entries,
// sync's own, are encoded fresh.
func orderedHooksRaw(original json.RawMessage, hooks map[string]any) (json.RawMessage, error) {
	orig := adapters.NewOrderedJSON()
	if len(original) > 0 && json.Unmarshal(original, orig) != nil {
		orig = adapters.NewOrderedJSON()
	}
	var events []string
	for _, event := range orig.Keys() {
		if _, ok := hooks[event]; ok {
			events = append(events, event)
		}
	}
	for _, event := range slices.Sorted(maps.Keys(hooks)) {
		if !slices.Contains(events, event) {
			events = append(events, event)
		}
	}
	encode := func(v any) (string, error) {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(v); err != nil {
			return "", err
		}
		return strings.TrimSuffix(buf.String(), "\n"), nil
	}
	out := adapters.NewOrderedJSON()
	for _, event := range events {
		items, isList := hooks[event].([]any)
		if !isList {
			if raw, ok := orig.Get(event); ok {
				var have any
				if json.Unmarshal(raw, &have) == nil && reflect.DeepEqual(have, jsonRoundTrip(hooks[event])) {
					out.SetRaw(event, raw)
					continue
				}
			}
			if err := out.Set(event, hooks[event]); err != nil {
				return nil, err
			}
			continue
		}
		var existing []json.RawMessage
		if raw, ok := orig.Get(event); ok {
			_ = json.Unmarshal(raw, &existing)
		}
		used := make([]bool, len(existing))
		parts := make([]string, 0, len(items))
		for _, item := range items {
			want := jsonRoundTrip(item)
			kept := false
			for i, raw := range existing {
				var have any
				if used[i] || json.Unmarshal(raw, &have) != nil || !reflect.DeepEqual(have, want) {
					continue
				}
				used[i], kept = true, true
				parts = append(parts, string(raw))
				break
			}
			if kept {
				continue
			}
			text, err := encode(item)
			if err != nil {
				return nil, err
			}
			parts = append(parts, text)
		}
		out.SetRaw(event, json.RawMessage("["+strings.Join(parts, ",")+"]"))
	}
	text, err := encode(out)
	return json.RawMessage(text), err
}

// jsonRoundTrip returns v as JSON decodes it, so a spec integer compares
// equal to the number read back from a native file.
func jsonRoundTrip(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if json.Unmarshal(raw, &out) != nil {
		return v
	}
	return out
}

func removeEqual(items []any, want any) ([]any, bool) {
	for i, item := range items {
		if reflect.DeepEqual(item, want) {
			return append(items[:i], items[i+1:]...), true
		}
	}
	return items, false
}

// userHookTarget tells a user hook which target ran it, the way the
// target's userTarget.hookTarget says.
type userHookTarget struct {
	scriptsDir string
	// goos is the platform the user files are written for.
	goos       string
	name, mode string
	// timeout converts the spec's timeout, when the target reads
	// another unit than seconds.
	timeout func(meta map[string]any) (any, bool)
	// args says the handler takes exec-form `args`; foldArgs, that it
	// has no such field and the args fold into the command.
	args, foldArgs bool
	// specHooks counts the leading entries that come from hook specs;
	// the rest, such as Cursor's context bridge, are sync's own.
	specHooks int
}

// tell adds the target to one Claude-shaped command handler.
func (t userHookTarget) tell(handler, meta map[string]any) {
	switch t.mode {
	case hookTargetExport:
		command, _ := handler["command"].(string)
		windows, _ := meta["commandWindows"].(string)
		if windows == "" {
			windows = command
		} else if t.scriptsDir != "" {
			windows = adapters.RewriteUserWindowsHookPath(windows, t.scriptsDir)
		}
		handler["command"] = adapters.ExportHookTarget(command, t.name)
		handler["commandWindows"] = windows
	case hookTargetHandlerEnv:
		handler["env"] = map[string]any{adapters.HookTargetEnv: t.name}
	}
}

// setSettingsEnv keeps the target in the settings `env` while managed
// hooks exist, and drops the value sync wrote once none are left.
func (t userHookTarget) setSettingsEnv(doc map[string]any, want bool, owns func(string) bool, mark func(string)) {
	raw, had := doc["env"]
	env, _ := raw.(map[string]any)
	next := adapters.WithoutHookTarget(env, any(t.name))
	if want {
		next = adapters.WithHookTarget(env, any(t.name))
	}
	keepUserContainer(doc, "env", raw, had, next, owns, mark)
}

// keepUserContainer stores value under key in doc. Once value is empty
// it removes the key only when sync created it, and otherwise gives back
// the user's own empty value, such as {} or null. mark records a
// container sync creates or fills in place of a null.
func keepUserContainer(doc map[string]any, key string, raw any, had bool, value map[string]any, owns func(string) bool, mark func(string)) {
	_, wasObject := raw.(map[string]any)
	switch {
	case len(value) > 0:
		if !had {
			mark(key)
		} else if raw == nil {
			mark(key + "=null")
		}
		doc[key] = value
	case !had || owns(key):
		delete(doc, key)
	case owns(key + "=null"):
		doc[key] = nil
	case wasObject:
		doc[key] = map[string]any{}
	default:
		doc[key] = raw
	}
}

// carriesHookTarget reports whether a native hook entry holds the target
// signal a handler carries itself: an `env` naming the target, or the
// export prefix.
func carriesHookTarget(entry any) bool {
	item, _ := entry.(map[string]any)
	handlers := []any{item}
	if group, ok := item["hooks"].([]any); ok {
		handlers = group
	}
	for _, raw := range handlers {
		handler, _ := raw.(map[string]any)
		if env, ok := handler["env"].(map[string]any); ok {
			if _, ok := env[adapters.HookTargetEnv]; ok {
				return true
			}
		}
		if command, _ := handler["command"].(string); strings.HasPrefix(command, "export "+adapters.HookTargetEnv+"=") {
			return true
		}
	}
	return false
}

// sameUserHook reports whether item runs the recorded entry's command
// under its matcher, whatever its other fields say.
func sameUserHook(item, recorded any) bool {
	want := userHookKeys(recorded)
	return slices.ContainsFunc(userHookKeys(item), func(key [2]string) bool { return slices.Contains(want, key) })
}

// extendsUserHook reports whether item runs the recorded entry's
// command with more words after it, under the same matcher: a bare
// interpreter an older version wrote for an exec-form spec, with its
// args added back by hand.
func extendsUserHook(item, recorded any) bool {
	want := userHookKeys(recorded)
	return slices.ContainsFunc(userHookKeys(item), func(key [2]string) bool {
		return slices.ContainsFunc(want, func(w [2]string) bool {
			return key[0] == w[0] && strings.HasPrefix(key[1], w[1]+" ")
		})
	})
}

// userHookKeys lists the matcher and command pairs a native hook entry
// runs: a Claude-style group holds several commands, a Cursor entry one.
func userHookKeys(item any) [][2]string {
	entry, _ := item.(map[string]any)
	matcher, _ := entry["matcher"].(string)
	commands := []any{entry}
	if group, ok := entry["hooks"].([]any); ok {
		commands = group
	}
	var out [][2]string
	for _, raw := range commands {
		hook, _ := raw.(map[string]any)
		if command, ok := hook["command"].(string); ok {
			out = append(out, [2]string{matcher, command})
		}
	}
	return out
}

func userHookCommands(raw any) []string {
	switch value := raw.(type) {
	case string:
		if value != "" {
			return []string{value}
		}
	case []any:
		var out []string
		for _, item := range value {
			if command, ok := item.(string); ok && command != "" {
				out = append(out, command)
			}
		}
		return out
	case []string:
		return value
	}
	return nil
}

func cloneHookState(in map[string][]any) map[string][]any {
	out := map[string][]any{}
	for k, v := range in {
		out[k] = append([]any(nil), v...)
	}
	return out
}
