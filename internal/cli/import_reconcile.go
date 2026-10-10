package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

type reconciliationEntry struct {
	Action      string `json:"action"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

type reconciliationPlan struct {
	Base     string                `json:"base"`
	Migrated string                `json:"migrated"`
	Upstream string                `json:"upstream"`
	Current  string                `json:"current"`
	Entries  []reconciliationEntry `json:"entries"`
}

func newImportReconcileCmd() *cobra.Command {
	var base, migrated, upstream string
	var mappings []string
	var asJSON bool
	cmd := &cobra.Command{
		Use:     "reconcile",
		Short:   "Plan upstream skill changes after moving a native source tree.",
		Long:    "Compare committed skill trees before and after a migration. --base is the original native tree, --migrated is the first canonical tree, --upstream holds later native changes, and HEAD holds current canonical changes. Map native skill directories to the configured skills source. Reports file additions, removals, updates, and conflicts, including assets. Does not apply or stage changes. Dirty mapped trees must be committed first.",
		Example: "  agnostic-ai import reconcile --base before --migrated migration --upstream main --map .cursor/skills=.agnostic-ai/skills",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			plan, err := planSkillReconciliation(base, migrated, upstream, mappings)
			if err != nil {
				return err
			}
			if asJSON {
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(plan); err != nil {
					return fmt.Errorf("write reconciliation plan: %w", err)
				}
				return nil
			}
			cmd.Printf("base %s\nmigrated %s\nupstream %s\ncurrent %s\n", plan.Base, plan.Migrated, plan.Upstream, plan.Current)
			for _, entry := range plan.Entries {
				cmd.Printf("%s\t%s\t%s\n", entry.Action, entry.Source, entry.Destination)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "Git revision before the source migration.")
	cmd.Flags().StringVar(&migrated, "migrated", "", "Git revision immediately after the canonical source migration.")
	cmd.Flags().StringVar(&upstream, "upstream", "", "Git revision with later native source changes.")
	cmd.Flags().StringArrayVar(&mappings, "map", nil, "Native-to-canonical skill directory mapping, old=new. Repeat for distinct native trees.")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print revisions and plan entries as JSON.")
	for _, name := range []string{"base", "migrated", "upstream", "map"} {
		_ = cmd.MarkFlagRequired(name)
	}
	return cmd
}

func planSkillReconciliation(base, migrated, upstream string, mappings []string) (reconciliationPlan, error) {
	plan := reconciliationPlan{Entries: []reconciliationEntry{}}
	cfg, err := config.Load(".")
	if err != nil {
		return plan, fmt.Errorf("load reconciliation config: %w", err)
	}
	for _, revision := range []struct {
		input  string
		output *string
	}{
		{base, &plan.Base}, {migrated, &plan.Migrated}, {upstream, &plan.Upstream}, {"HEAD", &plan.Current},
	} {
		data, err := reconciliationGit("rev-parse", "--verify", "--end-of-options", revision.input+"^{commit}")
		if err != nil {
			return plan, fmt.Errorf("resolve revision %q: %w", revision.input, err)
		}
		*revision.output = strings.TrimSpace(string(data))
	}
	for _, pair := range [][2]string{{plan.Base, plan.Migrated}, {plan.Base, plan.Upstream}, {plan.Migrated, plan.Current}} {
		if _, err := reconciliationGit("merge-base", "--is-ancestor", pair[0], pair[1]); err != nil {
			return plan, fmt.Errorf("reconciliation requires %s to be an ancestor of %s: %w", pair[0], pair[1], err)
		}
	}
	seen := map[string]bool{}
	for _, mapping := range mappings {
		old, canonical, ok := strings.Cut(mapping, "=")
		if !ok || !reconciliationPath(old) || !reconciliationPath(canonical) || old == canonical || strings.HasPrefix(old, canonical+"/") || strings.HasPrefix(canonical, old+"/") {
			return plan, fmt.Errorf("invalid skill mapping %q: use distinct project-relative directories, old=new", mapping)
		}
		if canonical != cfg.Sources.Skills {
			return plan, fmt.Errorf("mapping %q: destination must be configured sources.skills %q", mapping, cfg.Sources.Skills)
		}
		if seen[old] {
			return plan, fmt.Errorf("duplicate native source mapping %q", old)
		}
		seen[old] = true
		status, err := reconciliationGit("status", "--porcelain=v1", "-z", "--untracked-files=all", "--", ":(literal)"+old, ":(literal)"+canonical)
		if err != nil {
			return plan, fmt.Errorf("check mapped trees: %w", err)
		}
		if len(status) > 0 {
			return plan, fmt.Errorf("mapped trees %q and %q have uncommitted changes; commit them before planning", old, canonical)
		}
		trees := make([]map[string]string, 4)
		for i, tree := range []struct{ revision, dir string }{{plan.Base, old}, {plan.Upstream, old}, {plan.Migrated, canonical}, {plan.Current, canonical}} {
			trees[i], err = reconciliationTree(tree.revision, tree.dir)
			if err != nil {
				return plan, err
			}
		}
		files := map[string]bool{}
		for _, tree := range trees {
			for file := range tree {
				files[file] = true
			}
		}
		for file := range files {
			before, after, initial, current := trees[0][file], trees[1][file], trees[2][file], trees[3][file]
			if before != "" && initial == "" {
				return plan, fmt.Errorf("migration %s has no canonical counterpart for %s/%s; choose the complete migration revision", plan.Migrated, old, file)
			}
			if before == after || after == current {
				continue
			}
			action := "update"
			if before == "" {
				action = "add"
			} else if after == "" {
				action = "remove"
			}
			if initial != current {
				action = "conflict"
			}
			plan.Entries = append(plan.Entries, reconciliationEntry{action, old + "/" + file, canonical + "/" + file})
		}
	}
	destinations := map[string]int{}
	for _, entry := range plan.Entries {
		destinations[entry.Destination]++
	}
	for i := range plan.Entries {
		if destinations[plan.Entries[i].Destination] > 1 {
			plan.Entries[i].Action = "conflict"
		}
	}
	sort.Slice(plan.Entries, func(i, j int) bool { return plan.Entries[i].Source < plan.Entries[j].Source })
	return plan, nil
}

func reconciliationPath(value string) bool {
	return value != "" && value != "." && path.Clean(value) == value && !strings.HasPrefix(value, "/") && value != ".." && !strings.HasPrefix(value, "../") && !strings.ContainsAny(value, "\\:\x00\n\r\t")
}

// Blob IDs include content; the mode also detects changes to executable assets.
func reconciliationTree(revision, dir string) (map[string]string, error) {
	data, err := reconciliationGit("ls-tree", "-rz", "--full-tree", revision, "--", ":(literal)"+dir)
	if err != nil {
		return nil, fmt.Errorf("read tree %s:%s: %w", revision, dir, err)
	}
	files := map[string]string{}
	skills := map[string]bool{}
	for _, record := range bytes.Split(data, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		meta, name, ok := strings.Cut(string(record), "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 {
			return nil, fmt.Errorf("read tree %s:%s: invalid Git entry", revision, dir)
		}
		if fields[0] != "100644" && fields[0] != "100755" {
			return nil, fmt.Errorf("read tree %s:%s: unsupported linked or special file %q", revision, dir, name)
		}
		rel := strings.TrimPrefix(name, dir+"/")
		if rel == name {
			return nil, fmt.Errorf("read tree %s:%s: file outside mapped directory %q", revision, dir, name)
		}
		if strings.ContainsAny(rel, "\n\r\t") {
			return nil, fmt.Errorf("read tree %s:%s: unsupported control character in path %q", revision, dir, name)
		}
		files[rel] = meta
		parts := strings.Split(rel, "/")
		if len(parts) == 2 && parts[1] == "SKILL.md" {
			skills[parts[0]] = true
		}
	}
	for file := range files {
		name, _, _ := strings.Cut(file, "/")
		if !skills[name] {
			delete(files, file)
		}
	}
	return files, nil
}

func reconciliationGit(args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"--no-optional-locks"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return data, nil
}
