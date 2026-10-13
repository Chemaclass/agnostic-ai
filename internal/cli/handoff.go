package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

type handoffWorktree struct {
	path, branch, head string
	bare, prunable     bool
}
type listedHandoff struct {
	path, branch, worktree, head, date, status, pr, dirty string
	automatic                                             bool
	modified                                              time.Time
}

func newHandoffCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "handoff", Short: "Inspect task handoffs shared across worktrees"}
	cmd.AddCommand(&cobra.Command{
		Use: "list", Short: "List saved task handoffs and their local Git state", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root := memoryProjectRoot("")
			if root == "" {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "No task handoffs found.")
				return err
			}
			worktrees := handoffWorktrees(root)
			handoffs, err := listHandoffFiles(root, worktrees)
			if err != nil {
				return err
			}
			if len(handoffs) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "No task handoffs found.")
				return err
			}
			out := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			if _, err := fmt.Fprintln(out, "BRANCH\tWORKTREE\tHEAD\tDIRTY\tDATE\tSTATUS\tPR\tFILE"); err != nil {
				return err
			}
			for _, note := range handoffs {
				if _, err := fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", note.branch, note.worktree, note.head, note.dirty, note.date, note.status, note.pr, note.path); err != nil {
					return err
				}
			}
			if err := out.Flush(); err != nil {
				return err
			}
			if len(worktrees) > 0 && !worktrees[0].bare {
				for _, note := range handoffs {
					if handoffBranchMerged(root, note, worktrees[0]) {
						if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Review %s for removal: %s is merged into %s.\n", note.path, note.branch, worktrees[0].branch); err != nil {
							return err
						}
					}
				}
			}
			return nil
		},
	})
	return cmd
}

func handoffGit(root string, args ...string) (string, error) {
	return gitOutput(root, isolatedGitEnv(), append([]string{"--no-optional-locks", "-c", "core.fsmonitor=false"}, args...)...)
}

func handoffWorktrees(root string) []handoffWorktree {
	raw, err := handoffGit(root, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil
	}
	var out []handoffWorktree
	current := handoffWorktree{}
	for _, field := range strings.Split(raw, "\x00") {
		if field == "" {
			if current.path != "" {
				out = append(out, current)
				current = handoffWorktree{}
			}
			continue
		}
		key, value, _ := strings.Cut(field, " ")
		switch key {
		case "worktree":
			current.path = canonicalDir(value)
		case "branch":
			current.branch = strings.TrimPrefix(value, "refs/heads/")
		case "HEAD":
			current.head = value
		case "bare":
			current.bare = true
		case "prunable":
			current.prunable = true
		}
	}
	if current.path != "" {
		out = append(out, current)
	}
	return out
}

func listHandoffFiles(root string, worktrees []handoffWorktree) ([]listedHandoff, error) {
	projects := []string{root}
	if len(worktrees) > 0 && !worktrees[0].bare && !worktrees[0].prunable {
		top, err := handoffGit(root, "rev-parse", "--show-toplevel")
		if err == nil {
			prefix, err := filepath.Rel(canonicalDir(strings.TrimSpace(top)), root)
			if err == nil {
				shared := canonicalDir(filepath.Join(worktrees[0].path, prefix))
				relative, err := filepath.Rel(worktrees[0].path, shared)
				if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && handoffIgnoredStore(shared) {
					projects = append([]string{shared}, projects...)
				}
			}
		}
	}
	seen := map[string]bool{}
	notes := map[string]listedHandoff{}
	for _, project := range projects {
		project = canonicalDir(project)
		if seen[project] {
			continue
		}
		seen[project] = true
		var files []string
		if handoffIgnoredStore(project) {
			store := filepath.Join(project, ".agnostic-ai", "local", "handoffs")
			entries, err := os.ReadDir(store)
			if err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("%s: %w", store, err)
			}
			for _, entry := range entries {
				if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".md") {
					files = append(files, filepath.Join(store, entry.Name()))
				}
			}
		}
		files = append(files, filepath.Join(project, ".agnostic-ai", "local", "HANDOFF.md"), filepath.Join(project, ".agnostic-ai", "local", "HANDOFF.auto.md"))
		for _, path := range files {
			if !handoffSafeFile(project, path) {
				continue
			}
			note, ok, err := readHandoffHeader(path)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			previous, exists := notes[note.branch]
			if !exists || previous.automatic && !note.automatic || previous.automatic == note.automatic && handoffNewer(note, previous) {
				notes[note.branch] = note
			}
		}
	}
	out := make([]listedHandoff, 0, len(notes))
	dirty := map[string]string{}
	for _, note := range notes {
		note.worktree = "removed"
		note.dirty = "unknown"
		for _, worktree := range worktrees {
			branch := worktree.branch
			if branch == "" {
				branch = "detached-" + worktree.head
			}
			if worktree.bare || worktree.prunable || branch != note.branch {
				continue
			}
			note.worktree = worktree.path
			if state, ok := dirty[worktree.path]; ok {
				note.dirty = state
			} else {
				raw, err := handoffGit(worktree.path, "status", "--porcelain")
				state := "unknown"
				if err == nil {
					state = "clean"
					if strings.TrimSpace(raw) != "" {
						state = "dirty"
					}
				}
				dirty[worktree.path] = state
				note.dirty = state
			}
			break
		}
		out = append(out, note)
	}
	slices.SortFunc(out, func(a, b listedHandoff) int { return strings.Compare(a.branch, b.branch) })
	return out, nil
}

func handoffIgnoredStore(project string) bool {
	if !handoffNoSymlinks(project, filepath.Join(project, ".agnostic-ai", "local", "handoffs")) {
		return false
	}
	_, err := handoffGit(project, "check-ignore", "-q", "--", filepath.Join(project, ".agnostic-ai", "local", "handoffs", "handoff.md"))
	return err == nil
}

func handoffSafeFile(project, path string) bool {
	if !handoffNoSymlinks(project, path) {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if _, err := handoffGit(project, "check-ignore", "-q", "--", path); err != nil {
		return false
	}
	_, err = handoffGit(project, "ls-files", "--error-unmatch", "--", path)
	return err != nil
}

func handoffNoSymlinks(root, path string) bool {
	for path != root {
		if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return false
		} else if err != nil && !os.IsNotExist(err) {
			return false
		}
		parent := filepath.Dir(path)
		if parent == path {
			return false
		}
		path = parent
	}
	return true
}

func readHandoffHeader(path string) (listedHandoff, bool, error) {
	note := listedHandoff{path: path, automatic: strings.HasSuffix(path, ".auto.md"), status: "-", pr: "-", head: "-", date: "-"}
	file, err := os.Open(path)
	if err != nil {
		return note, false, fmt.Errorf("%s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return note, false, fmt.Errorf("%s: %w", path, err)
	}
	note.modified = info.ModTime()
	scan := bufio.NewScanner(io.LimitReader(file, 16*1024))
	scan.Buffer(make([]byte, 1024), 16*1024)
	for line := 0; line < 3 && scan.Scan(); line++ {
		for _, part := range strings.Split(scan.Text(), "|") {
			key, value, ok := strings.Cut(strings.TrimSpace(part), ":")
			if !ok {
				continue
			}
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			switch key {
			case "branch":
				note.branch = value
			case "head":
				note.head = value
			case "date":
				note.date = value
			case "status":
				note.status = value
			case "pr":
				note.pr = value
			}
		}
	}
	if err := scan.Err(); err != nil {
		return note, false, fmt.Errorf("%s: read header: %w", path, err)
	}
	return note, note.branch != "", nil
}

func handoffNewer(a, b listedHandoff) bool {
	for _, format := range []string{time.RFC3339, "2006-01-02T15:04Z"} {
		at, ae := time.Parse(format, a.date)
		bt, be := time.Parse(format, b.date)
		if ae == nil && be == nil && !at.Equal(bt) {
			return at.After(bt)
		}
	}
	return a.modified.After(b.modified)
}

func handoffBranchMerged(root string, note listedHandoff, main handoffWorktree) bool {
	if note.branch == main.branch || main.branch == "" || note.head == "-" || note.worktree != "removed" && note.dirty != "clean" {
		return false
	}
	if len(note.head) < 7 || len(note.head) > 64 {
		return false
	}
	for _, char := range note.head {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f' || char >= 'A' && char <= 'F') {
			return false
		}
	}
	ref := "refs/heads/" + note.branch
	if _, err := handoffGit(root, "show-ref", "--verify", "--quiet", ref); err != nil {
		return false
	}
	if _, err := handoffGit(root, "merge-base", "--is-ancestor", note.head, ref); err != nil {
		return false
	}
	_, err := handoffGit(root, "merge-base", "--is-ancestor", ref, main.head)
	return err == nil
}
