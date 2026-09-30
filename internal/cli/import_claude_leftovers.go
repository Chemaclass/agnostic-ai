package cli

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// claudeLeftoverListMax caps how many left-behind paths the import
// summary names before it counts the rest.
const claudeLeftoverListMax = 10

// claudeRuntimeDirs hold what Claude Code writes while it runs, not
// project config, so the left-behind list skips them.
var claudeRuntimeDirs = []string{"worktrees", "agent-memory", "agent-memory-local"}

// noteNestedClaudeEntryFile tells the user to delete .claude/CLAUDE.md
// once its text feeds AGNOSTIC_AI.md. Sync writes that text to the
// Claude entry file, and Claude Code loads both, so every instruction
// would arrive twice. Import leaves the file for the user to delete: it
// never removes a file it did not write.
func noteNestedClaudeEntryFile(root string) {
	cfg, err := config.Load(root)
	if err != nil || !slices.Contains(cfg.Targets, "claude") {
		return
	}
	entry := adapters.EntryPointPath(cfg, "claude")
	if entry == "" || filepath.Clean(filepath.FromSlash(entry)) == nestedClaudeMainFile {
		return
	}
	summaryf("  ! delete %s after the next sync: sync writes its text to %s, and Claude Code loads both files\n",
		filepath.ToSlash(nestedClaudeMainFile), filepath.ToSlash(entry))
}

// noteNestedClaudeMainFiles tells the user to delete each nested
// CLAUDE.md a rule now holds, which `doctor --fix` does. Sync writes that
// rule to the Claude rules directory, and Claude Code would load both
// copies.
func noteNestedClaudeMainFiles(root string, files []importedNestedClaudeFile) {
	if len(files) == 0 {
		return
	}
	cfg, err := config.Load(root)
	if err != nil || !slices.Contains(cfg.Targets, "claude") {
		return
	}
	for _, f := range files {
		summaryf("  ! delete %s with `agnostic-ai doctor --fix`: rule %s holds its text, and after the next sync Claude Code would load it twice\n", f.path, f.rule)
	}
}

// claudeFilesNotImported lists the files under .claude/ that `import
// claude` did not read, as slash paths relative to root: the files a
// skill or script may still use but no other tool receives. Files sync
// wrote, hidden files, and Claude Code's runtime state are left out.
func claudeFilesNotImported(root string, layout claudeLayout, mainSrc string, launchImported bool) ([]string, error) {
	base := filepath.Join(root, claudeDir)
	if !dirExists(base) {
		return nil, nil
	}
	synced := map[string]bool{}
	for _, p := range readStateFile(root).Outputs {
		synced[filepath.ToSlash(p)] = true
	}
	imported := claudeImportedPaths{root: root, layout: layout, mainSrc: filepath.ToSlash(mainSrc), launch: launchImported}
	var out []string
	err := filepath.WalkDir(base, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("%s: %w", p, walkErr)
		}
		if p == base {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		rel = filepath.ToSlash(rel)
		inClaude := strings.TrimPrefix(rel, claudeDir+"/")
		if strings.HasPrefix(d.Name(), ".") || d.IsDir() && slices.Contains(claudeRuntimeDirs, inClaude) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if imported.skill(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || imported.file(rel, inClaude) || synced[rel] || ownedWithoutLedger(p) {
			return nil
		}
		out = append(out, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(out)
	return out, nil
}

// claudeImportedPaths answers which .claude/ paths the Claude importer
// reads, following the configured layout.
type claudeImportedPaths struct {
	root    string
	layout  claudeLayout
	mainSrc string
	// launch reports whether this run imported launch.json.
	launch bool
}

// skill reports whether rel is a skill folder the import copied, or a
// file inside one.
func (c claudeImportedPaths) skill(rel string) bool {
	skills := filepath.ToSlash(c.layout.skills) + "/"
	name, _, _ := strings.Cut(strings.TrimPrefix(rel, skills), "/")
	if !strings.HasPrefix(rel, skills) || name == "" {
		return false
	}
	return fileExists(filepath.Join(c.root, c.layout.skills, name, "SKILL.md"))
}

// file reports whether the import reads the file at rel, which is
// inClaude relative to .claude/.
func (c claudeImportedPaths) file(rel, inClaude string) bool {
	switch inClaude {
	case "settings.json", "settings.local.json", claudeMainFile:
		return true
	case claudeLaunchFileName:
		return c.launch
	case claudeAgentsMainFile:
		return c.mainSrc == filepath.ToSlash(nestedClaudeAgentsMainFile)
	}
	for _, h := range helperFilesByTool["claude"] {
		if inClaude == h.basename {
			return true
		}
	}
	if dir := path.Dir(inClaude); dir == "hooks" {
		return true
	}
	md := strings.HasSuffix(rel, ".md")
	if strings.HasPrefix(rel, filepath.ToSlash(c.layout.rules)+"/") {
		return md
	}
	for _, dir := range []string{c.layout.agents, c.layout.commands} {
		if path.Dir(rel) == filepath.ToSlash(dir) {
			return md
		}
	}
	return false
}

// reportClaudeFilesNotImported prints the left-behind list, capped at
// claudeLeftoverListMax paths.
func reportClaudeFilesNotImported(paths []string) {
	if len(paths) == 0 {
		return
	}
	summaryf("  ! not imported, left in place (sync neither copies nor removes them):\n")
	for i, p := range paths {
		if i == claudeLeftoverListMax {
			summaryf("      and %d more\n", len(paths)-claudeLeftoverListMax)
			break
		}
		summaryf("      %s\n", filepath.ToSlash(p))
	}
}
