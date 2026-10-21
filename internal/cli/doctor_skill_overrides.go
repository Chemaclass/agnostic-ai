package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
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

// skillOverrideReport is what one settings file's skillOverrides does to
// the skills sync writes for Claude.
type skillOverrideReport struct {
	path    string
	data    []byte
	synced  int
	problem string
	// uneditable says why doctor --fix cannot change the file, so it
	// does not ask.
	uneditable error
	states     []skillOverrideFinding
	unknown    []skillOverrideFinding
}

func (r *skillOverrideReport) display() string {
	return filepath.ToSlash(r.path)
}

// collectClaudeSkillOverrides reads the project-local Claude settings
// file, which the /skills menu writes per developer. It returns nil when
// nothing there changes a synced skill.
func collectClaudeSkillOverrides(cfg *config.Config, bundle spec.Bundle) *skillOverrideReport {
	synced := map[string]string{}
	for _, skill := range bundle.For("claude").Skills {
		name, _ := adapters.ResolveMeta(skill.Meta, "claude")["name"].(string)
		if name == "" {
			name = skill.Name
		}
		synced[claudeSkillName(name)] = name
	}
	if len(synced) == 0 {
		return nil
	}
	report := &skillOverrideReport{path: filepath.Join(claudeLayoutFor(cfg).dir, "settings.local.json"), synced: len(synced)}
	data, err := os.ReadFile(report.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		report.problem = fmt.Sprintf("cannot read %s: %v", report.display(), err)
		return report
	}
	report.data = data
	report.uneditable = skillOverridesEditBlock(report, data)
	clean, _ := adapters.StripJSONC(data)
	var root map[string]json.RawMessage
	if err := json.Unmarshal(clean, &root); err != nil {
		report.problem = fmt.Sprintf("cannot read %s: parse %s: %v", report.display(), report.display(), err)
		return report
	}
	raw, ok := root["skillOverrides"]
	if !ok {
		return nil
	}
	var overrides map[string]json.RawMessage
	if err := json.Unmarshal(raw, &overrides); err != nil || overrides == nil {
		report.problem = fmt.Sprintf(`%s: skillOverrides is not an object, so it was not checked; Claude Code expects {"<skill>": "off"}`, report.display())
		return report
	}
	byValue := map[string]*skillOverrideFinding{}
	var unknownOrder []string
	for key, rawValue := range overrides {
		name, ok := synced[claudeSkillName(key)]
		if !ok {
			continue
		}
		value := strings.TrimSpace(string(rawValue))
		var text string
		if json.Unmarshal(rawValue, &text) == nil {
			if text == "on" {
				continue
			}
			if slices.ContainsFunc(claudeSkillOverrideStates, func(s skillOverrideState) bool { return s.value == text }) {
				value = text
			}
		}
		finding, ok := byValue[value]
		if !ok {
			finding = &skillOverrideFinding{value: value}
			byValue[value] = finding
			if value != text {
				unknownOrder = append(unknownOrder, value)
			}
		}
		finding.keys = append(finding.keys, key)
		if !slices.Contains(finding.skills, name) {
			finding.skills = append(finding.skills, name)
		}
	}
	for _, state := range claudeSkillOverrideStates {
		if finding, ok := byValue[state.value]; ok {
			slices.Sort(finding.keys)
			slices.Sort(finding.skills)
			report.states = append(report.states, *finding)
		}
	}
	slices.Sort(unknownOrder)
	for _, value := range unknownOrder {
		finding := byValue[value]
		slices.Sort(finding.skills)
		report.unknown = append(report.unknown, *finding)
	}
	if len(report.states) == 0 && len(report.unknown) == 0 {
		return nil
	}
	return report
}

func reportClaudeSkillOverrides(cmd *cobra.Command, report *skillOverrideReport) {
	if report == nil {
		return
	}
	cmd.Println()
	cmd.Println("Claude skill overrides:")
	if report.problem != "" {
		cmd.Printf("  ! %s\n", report.problem)
		return
	}
	for _, finding := range report.states {
		state := claudeSkillOverrideState(finding.value)
		cmd.Printf("  ! %s %s %d of %d synced skills%s (%s)%s: %s\n", report.display(), state.effect, len(finding.skills), report.synced, state.object, finding.value, state.result, listSkillOverrideNames(finding.skills))
	}
	for _, finding := range report.unknown {
		cmd.Printf("  ! %s sets a value Claude Code does not document (%s) for: %s\n", report.display(), finding.value, listSkillOverrideNames(finding.skills))
	}
	if len(report.states) == 0 {
		return
	}
	if report.uneditable != nil {
		cmd.Printf("  ! %v\n", report.uneditable)
		cmd.Println("    → review them in Claude Code with /skills")
		return
	}
	cmd.Printf("    → review them in Claude Code with /skills, run `agnostic-ai doctor --fix` in a terminal, or delete the entries from %s\n", report.display())
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
	if err := os.WriteFile(report.path, out, info.Mode().Perm()); err != nil {
		return 0, fmt.Errorf("%s: %w", report.path, err)
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
	repeated, err := rootKeyCount(data, "skillOverrides")
	if err != nil {
		return fmt.Errorf("parse %s: %w", report.display(), err)
	}
	if repeated > 1 {
		return fmt.Errorf("%s %w", report.display(), errSkillOverridesRepeated)
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
