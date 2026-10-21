package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claude"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/gitquery"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// claudeSkillOverrideStates are the documented skillOverrides values
// that change a skill, in the order doctor reports and offers them.
// code.claude.com/docs/en/skills: "Override skill visibility from settings".
var claudeSkillOverrideStates = []skillOverrideState{
	{"off", "turns off", "", ", so Claude Code refuses to run them", "turned off"},
	{"user-invocable-only", "hides", " from Claude", "", "hidden from Claude"},
	{"name-only", "lists", " without their description", "", "listed without its description"},
}

type skillOverrideState struct {
	value, effect, object, result, prompt string
}

const maxListedSkillOverrides = 10

var (
	errSkillOverridesChanged  = errors.New("changed while doctor ran; left as is")
	errSkillOverridesRepeated = errors.New("sets skillOverrides more than once, so removing entries could change which overrides apply; edit it in /skills")
)

type skillOverrideFinding struct {
	value  string
	keys   []string
	skills []string
}

// skillOverrideScope is which Claude settings file a report describes.
type skillOverrideScope string

const (
	skillOverrideLocal  skillOverrideScope = "local"
	skillOverrideShared skillOverrideScope = "shared"
	skillOverrideUser   skillOverrideScope = "user"
)

// skillOverrideEntry is one key of a file's skillOverrides object.
type skillOverrideEntry struct {
	// Key is as written in the file.
	Key string
	// Skill is the name Claude Code sees for a synced skill, "" otherwise.
	Skill string
	// Value is the string set, or the JSON text of a value that is not one.
	Value  string
	Synced bool
	// Effective is false when a higher settings file sets the same skill.
	Effective bool
	// Overlay is true for a shared file entry that the claude settings
	// overlay sets to the same value.
	Overlay bool
	// raw is the value's JSON text; text is true when it is a string.
	raw  string
	text bool
}

// documented is true for a value that changes the skill as Claude Code
// documents it.
func (e skillOverrideEntry) documented() bool {
	return e.text && slices.ContainsFunc(claudeSkillOverrideStates, func(s skillOverrideState) bool { return s.value == e.Value })
}

// skillOverrideFile is one settings file Claude Code reads
// skillOverrides from.
type skillOverrideFile struct {
	path  string
	scope skillOverrideScope
	// rootLocal marks the local file at the git root or main checkout,
	// which Claude Code reads when started below it.
	rootLocal bool
	// checkout names where a rootLocal file sits: "git root" or
	// "main checkout".
	checkout string
	// editable is true only for the project's own local file; --fix
	// never touches another one.
	editable bool
}

// skillOverrideReport is what one settings file's skillOverrides does to
// the skills sync writes for Claude.
type skillOverrideReport struct {
	skillOverrideFile
	data    []byte
	synced  int
	problem string
	// uneditable says why doctor --fix cannot change the file, so it
	// does not ask.
	uneditable error
	states     []skillOverrideFinding
	unknown    []skillOverrideFinding
	// overlaid are the shared file's states the overlay sets, which
	// sync wrote on purpose.
	overlaid []skillOverrideFinding
	// entries lists every key, sorted, including "on" and unsynced ones.
	entries []skillOverrideEntry
	// unsynced counts the entries that match no synced skill.
	unsynced int
}

func (r *skillOverrideReport) display() string {
	return filepath.ToSlash(r.path)
}

// label names the file in doctor output: project files relative to the
// project, the user file from ~, and a root local file with where it is.
func (r *skillOverrideReport) label() string {
	if r.rootLocal {
		return r.labelPath() + " (" + r.checkout + ")"
	}
	return r.labelPath()
}

func (r *skillOverrideReport) labelPath() string {
	switch {
	case r.scope == skillOverrideUser:
		return homeRelative(r.path)
	case r.rootLocal:
		if cwd, err := os.Getwd(); err == nil {
			if real, err := filepath.EvalSymlinks(cwd); err == nil {
				cwd = real
			}
			if rel, err := filepath.Rel(cwd, r.path); err == nil {
				return filepath.ToSlash(rel)
			}
		}
	}
	return r.display()
}

func (r *skillOverrideReport) hasFindings() bool {
	return r.problem != "" || len(r.states) > 0 || len(r.unknown) > 0 || len(r.overlaid) > 0
}

// claudeSyncedSkills maps the folded name Claude Code matches to the name
// it sees, for every skill sync writes for claude.
func claudeSyncedSkills(cfg *config.Config, bundle spec.Bundle) map[string]string {
	synced := map[string]string{}
	skillsDir := claudeLayoutFor(cfg).skills
	for _, skill := range bundle.For("claude").Skills {
		if cfg.IsUnmanaged(filepath.Join(skillsDir, skill.Name, "SKILL.md")) {
			continue
		}
		name, _ := adapters.ResolveMeta(skill.Meta, "claude")["name"].(string)
		if name == "" {
			name = skill.Name
		}
		synced[claudeSkillName(name)] = name
	}
	return synced
}

// claudeSettingsFiles lists the files Claude Code reads skillOverrides
// from, highest precedence first, without the managed files: the local
// file at the git root wins over the one an older Claude Code left beside
// the project. projectOnly keeps the shared file alone.
// code.claude.com/docs/en/settings
func claudeSettingsFiles(cfg *config.Config, projectOnly bool) []skillOverrideFile {
	shared := skillOverrideFile{path: claude.SettingsFilePath(cfg), scope: skillOverrideShared}
	if projectOnly {
		return []skillOverrideFile{shared}
	}
	var files []skillOverrideFile
	if root, checkout, ok := claudeLocalSettingsRoot(); ok {
		files = append(files, skillOverrideFile{path: filepath.Join(root, ".claude", "settings.local.json"), scope: skillOverrideLocal, rootLocal: true, checkout: checkout})
	}
	files = append(files, skillOverrideFile{path: filepath.Join(claudeLayoutFor(cfg).dir, "settings.local.json"), scope: skillOverrideLocal, editable: true}, shared)
	if path, ok := claudeUserSettingsPath(); ok {
		files = append(files, skillOverrideFile{path: path, scope: skillOverrideUser})
	}
	var out []skillOverrideFile
	for _, file := range files {
		i := slices.IndexFunc(out, func(kept skillOverrideFile) bool { return sameSettingsFile(kept.path, file.path) })
		switch {
		case i < 0:
			out = append(out, file)
		case file.editable:
			// The project is the root: it keeps the root's place and stays editable.
			out[i] = file
		}
	}
	return out
}

// claudeLocalSettingsRoot is where Claude Code v2.1.211+ keeps the local
// settings file for a session started below it: the repository root, or
// the main checkout in a worktree. It keeps the file beside the project
// on Windows and when that root is the home folder.
func claudeLocalSettingsRoot() (root, checkout string, ok bool) {
	if runtime.GOOS == "windows" {
		return "", "", false
	}
	layout, err := gitquery.LayoutAt(".")
	if err != nil || filepath.Base(layout.CommonDir) != ".git" {
		return "", "", false
	}
	root = filepath.Dir(layout.CommonDir)
	if home, err := globalUserHome(); err == nil && samePath(resolvedPath(home), resolvedPath(root)) {
		return "", "", false
	}
	checkout = "git root"
	if !samePath(layout.GitDir, layout.CommonDir) {
		checkout = "main checkout"
	}
	return root, checkout, true
}

// claudeUserSettingsPath honors CLAUDE_CONFIG_DIR. Without it and with no
// home folder, there is no user file to read.
func claudeUserSettingsPath() (string, bool) {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		home, err := globalUserHome()
		if err != nil {
			return "", false
		}
		dir = filepath.Join(home, ".claude")
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return filepath.Join(dir, "settings.json"), true
}

func sameSettingsFile(a, b string) bool {
	return samePath(resolvedPath(a), resolvedPath(b)) || sameFile(a, b)
}

// resolvedPath is path made absolute with its folder's links resolved,
// so a file that does not exist yet still compares by location.
func resolvedPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	if dir, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		return filepath.Join(dir, filepath.Base(abs))
	}
	return abs
}

// collectClaudeSkillOverrides returns one report per settings file that
// sets skillOverrides, highest precedence first. It is empty when claude
// has no synced skill.
func collectClaudeSkillOverrides(cfg *config.Config, bundle spec.Bundle, projectOnly bool) []*skillOverrideReport {
	synced := claudeSyncedSkills(cfg, bundle)
	if len(synced) == 0 {
		return nil
	}
	overlay := claudeOverlaySkillOverrides()
	set := map[string]bool{}
	var reports []*skillOverrideReport
	for _, file := range claudeSettingsFiles(cfg, projectOnly) {
		report := readSkillOverrideFile(cfg, file, synced)
		if report == nil {
			continue
		}
		var here []string
		for i := range report.entries {
			entry := &report.entries[i]
			folded := claudeSkillName(entry.Key)
			entry.Effective = !set[folded]
			entry.Overlay = report.scope == skillOverrideShared && entry.text && entry.documented() && overlay[entry.Key] == entry.Value
			here = append(here, folded)
		}
		for _, folded := range here {
			set[folded] = true
		}
		report.findings()
		reports = append(reports, report)
	}
	return reports
}

// claudeOverlaySkillOverrides maps each key the claude settings overlay
// sets in skillOverrides to its string value. Sync and lint report an
// overlay that does not parse.
func claudeOverlaySkillOverrides() map[string]string {
	data, err := os.ReadFile(claudeOverlayPath("."))
	if err != nil {
		return nil
	}
	var doc struct {
		SkillOverrides map[string]json.RawMessage `json:"skillOverrides"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}
	values := map[string]string{}
	for key, raw := range doc.SkillOverrides {
		var text string
		if json.Unmarshal(raw, &text) == nil {
			values[key] = text
		}
	}
	return values
}

// localSkillOverrides is the one report doctor --fix may edit, or nil.
func localSkillOverrides(reports []*skillOverrideReport) *skillOverrideReport {
	for _, report := range reports {
		if report.editable {
			return report
		}
	}
	return nil
}

// readSkillOverrideFile is nil when the file is missing or sets no
// skillOverrides. A file that repeats the key reads as encoding/json
// does, with the last object.
func readSkillOverrideFile(cfg *config.Config, file skillOverrideFile, synced map[string]string) *skillOverrideReport {
	report := &skillOverrideReport{skillOverrideFile: file, synced: len(synced)}
	data, err := os.ReadFile(report.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		// The label already names the file, so drop the path the error repeats.
		var pathErr *fs.PathError
		if errors.As(err, &pathErr) {
			err = pathErr.Err
		}
		report.problem = fmt.Sprintf("cannot read %s: %v", report.label(), err)
		return report
	}
	report.data = data
	if report.editable {
		report.uneditable = skillOverridesEditBlock(report, data)
		if cfg.IsUnmanaged(report.path) {
			report.uneditable = fmt.Errorf("%s is listed under sync.unmanaged, so doctor leaves it to you", report.display())
		}
	}
	clean, _ := adapters.StripJSONC(data)
	if len(bytes.TrimSpace(clean)) == 0 {
		return nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(clean, &root); err != nil {
		report.problem = fmt.Sprintf("cannot parse %s: %v", report.label(), err)
		return report
	}
	raw, ok := root["skillOverrides"]
	if !ok {
		return nil
	}
	var overrides map[string]json.RawMessage
	if err := json.Unmarshal(raw, &overrides); err != nil || overrides == nil {
		report.problem = fmt.Sprintf(`%s: skillOverrides is not an object, so it was not checked; Claude Code expects {"<skill>": "off"}`, report.label())
		return report
	}
	for key, rawValue := range overrides {
		entry := skillOverrideEntry{Key: key, raw: strings.TrimSpace(string(rawValue)), Effective: true}
		entry.Value = entry.raw
		var text string
		if json.Unmarshal(rawValue, &text) == nil {
			entry.Value, entry.text = text, true
		}
		entry.Skill, entry.Synced = synced[claudeSkillName(key)]
		if !entry.Synced {
			report.unsynced++
		}
		report.entries = append(report.entries, entry)
	}
	slices.SortFunc(report.entries, func(a, b skillOverrideEntry) int { return strings.Compare(a.Key, b.Key) })
	return report
}

// findings groups the entries that take effect on synced skills by value.
// "on" changes nothing, and an entry the overlay sets goes to overlaid.
func (r *skillOverrideReport) findings() {
	states, overlaid := map[string]*skillOverrideFinding{}, map[string]*skillOverrideFinding{}
	unknown := map[string]*skillOverrideFinding{}
	for _, entry := range r.entries {
		if !entry.Synced || !entry.Effective || (entry.text && entry.Value == "on") {
			continue
		}
		group, value := states, entry.Value
		switch {
		case !entry.documented():
			group, value = unknown, entry.raw
		case entry.Overlay:
			group = overlaid
		}
		finding, ok := group[value]
		if !ok {
			finding = &skillOverrideFinding{value: value}
			group[value] = finding
		}
		finding.keys = append(finding.keys, entry.Key)
		if !slices.Contains(finding.skills, entry.Skill) {
			finding.skills = append(finding.skills, entry.Skill)
		}
	}
	r.states, r.overlaid = inStateOrder(states), inStateOrder(overlaid)
	for _, value := range slices.Sorted(maps.Keys(unknown)) {
		finding := unknown[value]
		slices.Sort(finding.skills)
		r.unknown = append(r.unknown, *finding)
	}
}

func inStateOrder(group map[string]*skillOverrideFinding) []skillOverrideFinding {
	var out []skillOverrideFinding
	for _, state := range claudeSkillOverrideStates {
		if finding, ok := group[state.value]; ok {
			slices.Sort(finding.keys)
			slices.Sort(finding.skills)
			out = append(out, *finding)
		}
	}
	return out
}

func reportClaudeSkillOverrides(cmd *cobra.Command, reports []*skillOverrideReport) {
	if !slices.ContainsFunc(reports, (*skillOverrideReport).hasFindings) {
		return
	}
	cmd.Println()
	cmd.Println("Claude skill overrides:")
	for _, report := range reports {
		if !report.hasFindings() {
			continue
		}
		if report.problem != "" {
			cmd.Printf("  ! %s\n", report.problem)
			continue
		}
		for _, finding := range report.states {
			state := claudeSkillOverrideState(finding.value)
			cmd.Printf("  ! %s %s %d of %d synced skills%s (%s)%s: %s\n", report.label(), state.effect, len(finding.skills), report.synced, state.object, finding.value, state.result, listSkillOverrideNames(finding.skills))
		}
		for _, finding := range report.unknown {
			cmd.Printf("  ! %s sets a value Claude Code does not document (%s) for: %s\n", report.label(), finding.value, listSkillOverrideNames(finding.skills))
		}
		if len(report.states) > 0 {
			report.printNextStep(cmd)
		}
		for _, finding := range report.overlaid {
			state := claudeSkillOverrideState(finding.value)
			cmd.Printf("  ~ %s %s %d of %d synced skills%s (%s) as %s sets: %s\n", report.label(), state.effect, len(finding.skills), report.synced, state.object, finding.value, filepath.ToSlash(claudeOverlayPath("")), listSkillOverrideNames(finding.skills))
		}
		if report.unsynced > 0 {
			noun := "entries"
			if report.unsynced == 1 {
				noun = "entry"
			}
			cmd.Printf("  ~ %s also sets %d %s for skills sync does not write\n", report.label(), report.unsynced, noun)
		}
	}
}

func (r *skillOverrideReport) printNextStep(cmd *cobra.Command) {
	switch {
	case r.editable && r.uneditable != nil:
		cmd.Printf("  ! %v\n", r.uneditable)
		cmd.Println("    → review them in Claude Code with /skills")
	case r.editable:
		cmd.Printf("    → review them in Claude Code with /skills, run `agnostic-ai doctor --fix` in a terminal, or delete the entries from %s\n", r.display())
	case r.rootLocal:
		cmd.Printf("    → Claude Code reads it for the whole repository; review them in Claude Code with /skills, or delete the entries from %s\n", r.labelPath())
	case r.scope == skillOverrideShared:
		cmd.Printf("    → team policy: set skillOverrides in %s and run `agnostic-ai sync`\n", filepath.ToSlash(claudeOverlayPath("")))
	default:
		cmd.Printf("    → applies to every project on this machine; delete the entries from %s\n", r.labelPath())
	}
}

// skillOverrideRecord is one skill_overrides item in doctor --json. A
// file that could not be read has only path, scope, and problem.
type skillOverrideRecord struct {
	Path      string `json:"path"`
	Scope     string `json:"scope"`
	Skill     string `json:"skill"`
	Value     string `json:"value"`
	Synced    bool   `json:"synced"`
	Effective bool   `json:"effective"`
	Problem   string `json:"problem,omitempty"`
}

func (r skillOverrideRecord) MarshalJSON() ([]byte, error) {
	if r.Problem != "" {
		return json.Marshal(struct {
			Path    string `json:"path"`
			Scope   string `json:"scope"`
			Problem string `json:"problem"`
		}{r.Path, r.Scope, r.Problem})
	}
	type entry skillOverrideRecord
	return json.Marshal(entry(r))
}

// skillOverrideRecords keeps project files relative, as other doctor JSON
// paths are, and the files outside the project absolute.
func skillOverrideRecords(reports []*skillOverrideReport) []skillOverrideRecord {
	records := []skillOverrideRecord{}
	for _, report := range reports {
		path := report.display()
		if report.rootLocal || report.scope == skillOverrideUser {
			if abs, err := filepath.Abs(report.path); err == nil {
				path = filepath.ToSlash(abs)
			}
		}
		if report.problem != "" {
			records = append(records, skillOverrideRecord{Path: path, Scope: string(report.scope), Problem: report.problem})
			continue
		}
		for _, entry := range report.entries {
			records = append(records, skillOverrideRecord{Path: path, Scope: string(report.scope), Skill: entry.Key, Value: entry.Value, Synced: entry.Synced, Effective: entry.Effective})
		}
	}
	return records
}

// claudeSkillOverridesSelected is true when doctor checks claude.
func claudeSkillOverridesSelected(cfg *config.Config, targets []string) bool {
	return slices.Contains(cfg.Targets, "claude") && (len(targets) == 0 || slices.Contains(targets, "claude"))
}

func claudeSkillOverrideState(value string) skillOverrideState {
	for _, state := range claudeSkillOverrideStates {
		if state.value == value {
			return state
		}
	}
	return skillOverrideState{}
}

func listSkillOverrideNames(names []string) string {
	if len(names) <= maxListedSkillOverrides {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s, and %d more", strings.Join(names[:maxListedSkillOverrides], ", "), len(names)-maxListedSkillOverrides)
}

// fixClaudeSkillOverrides asks, in a terminal only, whether to remove
// each state's entries. The file is the developer's, so nothing changes
// without a yes.
func fixClaudeSkillOverrides(cmd *cobra.Command, report *skillOverrideReport, backup bool) error {
	if report == nil || len(report.states) == 0 || report.uneditable != nil {
		return nil
	}
	in := cmd.InOrStdin()
	f, ok := in.(*os.File)
	if !ok || !term.IsTerminal(f.Fd()) {
		cmd.Printf("  ~ kept skillOverrides in %s; run `agnostic-ai doctor --fix` in a terminal to choose, or use /skills in Claude Code\n", report.display())
		return nil
	}
	reader := bufio.NewReader(in)
	return applySkillOverrideReset(cmd, report, backup, func(question string) (bool, error) {
		cmd.PrintErrf("%s [y/N] ", question)
		line, err := reader.ReadString('\n')
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("confirm skillOverrides reset in %s: %w", report.display(), err)
		}
		answer := strings.TrimSpace(line)
		return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes"), nil
	})
}

// applySkillOverrideReset reports a file it could not change as a
// warning, so the rest of doctor --fix still runs.
func applySkillOverrideReset(cmd *cobra.Command, report *skillOverrideReport, backup bool, confirm func(string) (bool, error)) error {
	if report.uneditable != nil {
		return nil
	}
	removed, err := resetClaudeSkillOverrides(report, backup, confirm)
	if err != nil {
		cmd.Printf("  ! %v\n", err)
		return nil
	}
	if removed > 0 {
		cmd.Printf("  ✓ re-enabled %d synced skill(s) in %s\n", removed, report.display())
	}
	return nil
}

// resetClaudeSkillOverrides removes the confirmed states' keys from the
// file in place, so comments, order, and every other key stay as they
// were. It returns how many synced skills it re-enabled.
func resetClaudeSkillOverrides(report *skillOverrideReport, backup bool, confirm func(string) (bool, error)) (int, error) {
	var keys []string
	skills := 0
	for _, finding := range report.states {
		noun := "skills"
		if len(finding.skills) == 1 {
			noun = "skill"
		}
		question := fmt.Sprintf("Re-enable %d synced %s %s in %s (%s)?", len(finding.skills), noun, claudeSkillOverrideState(finding.value).prompt, report.display(), listSkillOverrideNames(finding.skills))
		yes, err := confirm(question)
		if err != nil {
			return 0, err
		}
		if yes {
			keys = append(keys, finding.keys...)
			skills += len(finding.skills)
		}
	}
	if len(keys) == 0 {
		return 0, nil
	}
	info, err := os.Lstat(report.path)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", report.display(), err)
	}
	now, err := os.ReadFile(report.path)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", report.path, err)
	}
	if string(now) != string(report.data) {
		return 0, fmt.Errorf("%s %w", report.display(), errSkillOverridesChanged)
	}
	if err := skillOverridesEditBlock(report, now); err != nil {
		return 0, err
	}
	out, err := removeSkillOverrideKeys(report.path, now, keys)
	if err != nil {
		return 0, err
	}
	if backup {
		backupPath := report.path + ".bak"
		if backupInfo, err := os.Lstat(backupPath); err == nil && backupInfo.Mode()&os.ModeSymlink != 0 {
			return 0, fmt.Errorf("%s: refusing symlink backup destination", backupPath)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return 0, fmt.Errorf("%s: %w", backupPath, err)
		}
		if err := os.WriteFile(backupPath, now, info.Mode().Perm()); err != nil {
			return 0, fmt.Errorf("%s: %w", backupPath, err)
		}
		// WriteFile keeps the mode of a .bak that already exists.
		if err := os.Chmod(backupPath, info.Mode().Perm()); err != nil {
			return 0, fmt.Errorf("%s: %w", backupPath, err)
		}
	}
	if err := replaceSettingsFile(report.path, out, info.Mode().Perm()); err != nil {
		return 0, err
	}
	return skills, nil
}

// skillOverridesEditBlock says why doctor must not edit the file: a
// link points at a file the user may share, and a repeated
// skillOverrides object shows another copy once the last one empties.
func skillOverridesEditBlock(report *skillOverrideReport, data []byte) error {
	info, err := os.Lstat(report.path)
	if err != nil {
		return fmt.Errorf("%s: %w", report.display(), err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s: not a regular file, so doctor leaves it to you", report.display())
	}
	if info.Mode().Perm()&0o200 == 0 {
		return fmt.Errorf("%s: read-only, so doctor leaves it to you", report.display())
	}
	if reason := settingsFileShared(info); reason != "" {
		return fmt.Errorf("%s %s, so doctor leaves it to you", report.display(), reason)
	}
	repeated, err := rootKeyCount(data, "skillOverrides")
	if err != nil {
		return fmt.Errorf("parse %s: %w", report.display(), err)
	}
	if repeated > 1 {
		return fmt.Errorf("%s %w", report.display(), errSkillOverridesRepeated)
	}
	return nil
}

// replaceSettingsFile writes body beside path and renames it into place,
// so a failed write never leaves the user's settings empty or cut short.
func replaceSettingsFile(path string, body []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Dir(path), err)
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	_, err = tmp.Write(body)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(name, perm)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func rootKeyCount(data []byte, key string) (int, error) {
	text := string(data)
	open, err := jsonRootOpen(text)
	if err != nil {
		return 0, err
	}
	members, _, err := scanJSONObject(text, open)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, m := range members {
		if m.key == key {
			count++
		}
	}
	return count, nil
}

// removeSkillOverrideKeys drops every copy of each key, since a JSON
// object can repeat one, and the skillOverrides object once it is empty.
func removeSkillOverrideKeys(path string, data []byte, keys []string) ([]byte, error) {
	text := string(data)
	for _, key := range keys {
		for {
			open, err := jsonRootOpen(text)
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", path, err)
			}
			next, err := removeJSONPath(text, open, []string{"skillOverrides", key})
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", path, err)
			}
			if next == text {
				break
			}
			text = next
		}
	}
	return []byte(text), nil
}
