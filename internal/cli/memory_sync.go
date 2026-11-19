package cli

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// runMemorySeeds copies, for each seed an adapter queued this sync, the
// source facts into its personal store when that store holds no Markdown
// file yet, and notes the result.
func runMemorySeeds() {
	for _, seed := range adapters.TakeMemorySeeds() {
		if info, err := os.Stat(seed.From); err != nil || !info.IsDir() {
			continue
		}
		plan, done, err := importMemory(seed.From, seed.To, seed.Target, memoryImportPick{}, true)
		from := filepath.ToSlash(seed.From)
		if err != nil {
			adapters.NoteProject(fmt.Sprintf("%s: could not copy the facts in %s into personal memory: %v; run `agnostic-ai memory import --from %s`", seed.Target, from, err, seed.Target))
			continue
		}
		if done && len(plan.copies) > 0 {
			adapters.NoteProject(fmt.Sprintf("%s: copied %d fact(s) from %s into personal memory, which was empty; every tool reads them now, so review them with `agnostic-ai memory list`; the source is unchanged", seed.Target, len(plan.copies), from))
		}
		if done && len(plan.skipped) > 0 {
			adapters.NoteProject(fmt.Sprintf("%s: skipped %d fact(s) in %s; `agnostic-ai memory import --from %s --dry-run` says why", seed.Target, len(plan.skipped), from, seed.Target))
		}
	}
}

// hasMarkdownFile reports whether dir holds any Markdown file, the index
// included.
func hasMarkdownFile(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(entries, func(e fs.DirEntry) bool {
		return !e.IsDir() && filepath.Ext(e.Name()) == ".md"
	})
}

func preparePersonalMemorySync(root string, cfg *config.Config) {
	if !slices.Contains(cfg.Builtins, "memory") {
		return
	}
	if !cfg.RepoPersonalMemory() {
		if raw, err := exec.Command("git", "-C", root, "worktree", "list", "--porcelain").Output(); err == nil {
			count := 0
			for _, entry := range strings.Split(strings.TrimSpace(string(raw)), "\n\n") {
				lines := strings.Split(entry, "\n")
				if !slices.Contains(lines, "bare") && !slices.ContainsFunc(lines, func(line string) bool { return line == "prunable" || strings.HasPrefix(line, "prunable ") }) {
					count++
				}
			}
			if count > 1 {
				adapters.NoteProject("Personal memory is separate in each checkout; set memory.personal: repo in agnostic-ai.local.yaml to share it across worktrees")
			}
		}
	}
	repo, err := adapters.PersonalMemoryDir(&config.Config{Memory: config.MemoryConfig{Personal: config.PersonalMemoryRepo}}, root)
	if err != nil {
		return
	}
	checkout := filepath.Join(root, defaultMemoryStores[1].dir)
	from, to, label := repo, checkout, memorySourceRepo
	if cfg.RepoPersonalMemory() {
		from, to, label = checkout, repo, memorySourceCheckout
	}
	adapters.RequestMemorySeed(adapters.MemorySeed{Target: label, From: from, To: to})
}
