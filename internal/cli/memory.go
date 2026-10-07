package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// memoryIndexLineCap matches the cap the shared-memory skill keeps the
// index under, since every session loads the whole index.
const memoryIndexLineCap = 100

const memoryIndexFile = "MEMORY.md"

// memoryStore is one shared memory folder and its index.
type memoryStore struct {
	scope string
	dir   string
}

var defaultMemoryStores = []memoryStore{
	{scope: "project", dir: filepath.Dir(filepath.FromSlash(adapters.ProjectMemoryIndexPath))},
	{scope: "personal", dir: filepath.Dir(filepath.FromSlash(adapters.PersonalMemoryIndexPath))},
}

// memoryStores returns the stores of the project in the working
// directory, with the personal one where memory.personal puts it.
func memoryStores() ([]memoryStore, error) {
	return memoryStoresAt(".")
}

// memoryStoresAt returns the stores of the project at root.
func memoryStoresAt(root string) ([]memoryStore, error) {
	cfg, err := config.Load(root)
	if err != nil || !cfg.RepoPersonalMemory() {
		return defaultMemoryStores, nil
	}
	dir, err := adapters.PersonalMemoryDir(cfg, root)
	if err != nil {
		return nil, err
	}
	return []memoryStore{defaultMemoryStores[0], {scope: "personal", dir: dir}}, nil
}

func (s memoryStore) indexPath() string { return filepath.Join(s.dir, memoryIndexFile) }

// memoryTopic is one fact file and its frontmatter.
type memoryTopic struct {
	file, name, description, typ string
}

// memoryEntry is an index line that links a file.
type memoryEntry struct {
	line   int
	text   string
	title  string
	target string
}

// memoryContents is a store as it is on disk.
type memoryContents struct {
	memoryStore
	index   []string
	entries []memoryEntry
	topics  []memoryTopic
}

var memoryIndexEntry = regexp.MustCompile(`^\s*[-*+]\s+\[([^\]]*)\]\(([^)\s]+)\)`)

// loadMemoryStore reads s, or reports false when its folder is missing,
// as the personal store is in CI.
func loadMemoryStore(s memoryStore) (memoryContents, bool, error) {
	files, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return memoryContents{}, false, nil
	}
	if err != nil {
		return memoryContents{}, false, fmt.Errorf("%s: %w", s.dir, err)
	}
	c := memoryContents{memoryStore: s}
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".md" || f.Name() == memoryIndexFile {
			continue
		}
		text, err := readMemoryFile(filepath.Join(s.dir, f.Name()))
		if err != nil {
			return memoryContents{}, false, err
		}
		c.topics = append(c.topics, parseMemoryTopic(f.Name(), text))
	}
	index, err := readMemoryFile(s.indexPath())
	if errors.Is(err, fs.ErrNotExist) {
		return c, true, nil
	}
	if err != nil {
		return memoryContents{}, false, err
	}
	if index != "" {
		c.index = strings.Split(strings.TrimSuffix(index, "\n"), "\n")
	}
	for i, line := range c.index {
		m := memoryIndexEntry.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if target, ok := memoryLinkTarget(m[2]); ok {
			c.entries = append(c.entries, memoryEntry{line: i + 1, text: strings.TrimRight(line, " \t"), title: m[1], target: target})
		}
	}
	return c, true, nil
}

func readMemoryFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n"), nil
}

func parseMemoryTopic(file, text string) memoryTopic {
	t := memoryTopic{file: file}
	_, _, meta := claudeFrontmatter(text)
	t.name, _ = meta["name"].(string)
	t.description, _ = meta["description"].(string)
	if m, ok := meta["metadata"].(map[string]any); ok {
		t.typ, _ = m["type"].(string)
	}
	if t.typ == "" {
		// Claude Code's auto memory writes the type at the top level.
		t.typ, _ = meta["type"].(string)
	}
	return t
}

// memoryLinkTarget returns the store-relative file a link names, or
// false for a URL or an anchor in the index itself.
func memoryLinkTarget(link string) (string, bool) {
	if strings.Contains(link, "://") || strings.HasPrefix(link, "#") || strings.HasPrefix(link, "mailto:") {
		return "", false
	}
	link, _, _ = strings.Cut(link, "#")
	return filepath.Clean(filepath.FromSlash(link)), true
}

func anyMemoryStore(stores []memoryStore) bool {
	for _, s := range stores {
		if info, err := os.Stat(s.dir); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func (c memoryContents) linked() map[string]bool {
	out := map[string]bool{}
	for _, e := range c.entries {
		out[e.target] = true
	}
	return out
}

// lintMemory checks every memory store that exists (LINT039 to LINT042).
func lintMemory() ([]lintFinding, error) {
	stores, err := memoryStores()
	if err != nil {
		return nil, err
	}
	var out []lintFinding
	for _, s := range stores {
		c, ok, err := loadMemoryStore(s)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, c.lint()...)
		}
	}
	return out, nil
}

func (c memoryContents) lint() []lintFinding {
	var out []lintFinding
	index := c.indexPath()
	if len(c.index) > memoryIndexLineCap {
		out = append(out, lintFinding{
			Code:     "LINT039",
			Severity: lintWarn,
			Path:     index,
			Message:  fmt.Sprintf("the %s memory index has %d lines, over the %d every session should load; merge or drop facts", c.scope, len(c.index), memoryIndexLineCap),
		})
	}
	for _, e := range c.entries {
		if _, err := os.Stat(filepath.Join(c.dir, e.target)); err != nil {
			out = append(out, lintFinding{
				Code:     "LINT040",
				Severity: lintError,
				Path:     index,
				Message:  fmt.Sprintf("line %d links %s, which does not exist; restore the file or run `agnostic-ai memory index`", e.line, filepath.ToSlash(e.target)),
			})
		}
	}
	linked := c.linked()
	for _, t := range c.topics {
		if !linked[t.file] {
			out = append(out, lintFinding{
				Code:     "LINT041",
				Severity: lintWarn,
				Path:     filepath.Join(c.dir, t.file),
				Message:  "no line in " + memoryIndexFile + " links this fact, so tools never load it; run `agnostic-ai memory index`",
			})
		}
	}
	files := []string{index}
	for _, t := range c.topics {
		files = append(files, filepath.Join(c.dir, t.file))
	}
	for _, path := range files {
		out = append(out, lintMemorySecrets(path)...)
	}
	return out
}

// memorySecretIn reports a credential by its shape alone: a token by its
// prefix, a URL password or credential parameter, or a Bearer token.
// The name checks import also runs on MCP values read prose such as
// `KEY=value` as a secret.
func memorySecretIn(line string) bool {
	text := mcpDetectorText(line)
	if mcpTokenIn(text) || memoryBearerIn(text) {
		return true
	}
	for _, word := range strings.Fields(text) {
		if strings.Contains(word, "://") && mcpURLCredentialIn(word) {
			return true
		}
	}
	return false
}

// memoryBearerIn finds a `Bearer` token with a digit and 16 or more
// characters, so prose such as "bearer authentication." does not count.
func memoryBearerIn(text string) bool {
	for _, m := range mcpBearer.FindAllStringSubmatch(text, -1) {
		token := strings.TrimRight(m[1], ".,;:!?)`")
		if len(token) >= 16 && strings.ContainsAny(token, "0123456789") && !strings.Contains(token, mcpRefSentinel) {
			return true
		}
	}
	return false
}

// lintMemorySecrets flags each line that holds a credential. The finding
// names the line, never the value, since lint output lands in CI logs.
func lintMemorySecrets(path string) []lintFinding {
	text, err := readMemoryFile(path)
	if err != nil {
		return nil
	}
	var out []lintFinding
	for i, line := range strings.Split(text, "\n") {
		if memorySecretIn(line) {
			out = append(out, lintFinding{
				Code:     "LINT042",
				Severity: lintError,
				Path:     path,
				Message:  fmt.Sprintf("line %d looks like a secret; every tool reads memory as plain text, so remove it and keep the secret out of the repository", i+1),
			})
		}
	}
	return out
}

// rebuildIndex returns the index without conflict markers, dead links,
// or repeated links, then a line for each fact no line links, by file
// name. Every other line stays: personal memory has no Git copy.
func (c memoryContents) rebuildIndex() string {
	present := map[string]bool{}
	for _, t := range c.topics {
		present[t.file] = true
	}
	entryAt := map[int]memoryEntry{}
	for _, e := range c.entries {
		entryAt[e.line] = e
	}
	var lines []string
	seen := map[string]bool{}
	for i, line := range c.index {
		if e, ok := entryAt[i+1]; ok {
			if present[e.target] && !seen[e.target] {
				seen[e.target] = true
				lines = append(lines, e.text)
			}
			continue
		}
		if !memoryConflictMarker(line) {
			lines = append(lines, line)
		}
	}
	for _, t := range c.topics {
		if seen[t.file] {
			continue
		}
		line := "- [" + t.title() + "](" + t.file + ")"
		if d := strings.TrimSpace(t.description); d != "" {
			line += ": " + d
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func memoryConflictMarker(line string) bool {
	for _, marker := range []string{"<<<<<<<", "|||||||", "=======", ">>>>>>>"} {
		if line == marker || strings.HasPrefix(line, marker+" ") {
			return true
		}
	}
	return false
}

func (t memoryTopic) title() string {
	if t.name != "" {
		return t.name
	}
	return strings.TrimSuffix(t.file, ".md")
}

func newMemoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Check, list, and rebuild the shared memory",
		Long: "Works on the shared memory stores in the working directory: project memory in " +
			filepath.ToSlash(defaultMemoryStores[0].dir) + "/ and personal memory in " + filepath.ToSlash(defaultMemoryStores[1].dir) +
			"/, or in the repository store with memory.personal: repo. A store whose folder is missing is skipped.",
	}
	cmd.AddCommand(newMemoryLintCmd(), newMemoryIndexCmd(), newMemoryListCmd(), newMemoryPathCmd())
	return cmd
}

func newMemoryLintCmd() *cobra.Command {
	var strict, asJSON bool
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Run only the memory lint checks",
		Long: "Reports an index over 100 lines (LINT039), an index line whose file is missing (LINT040), " +
			"a fact no index line links (LINT041), and a line that looks like a secret (LINT042). " +
			"`lint` and `doctor` report the same findings. Exit code 1 on error findings, or on " +
			"warnings with --strict. --json prints the findings in the `lint --json` format.",
		Example: `  agnostic-ai memory lint
  agnostic-ai memory lint --json | jq -r '.findings[].code'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			findings, err := lintMemory()
			if err != nil {
				return err
			}
			if asJSON {
				return printLintJSON(cmd, "memory lint", findings, strict)
			}
			if stores, err := memoryStores(); err != nil || !anyMemoryStore(stores) {
				cmd.Printf("No memory store found in %s/ or %s/.\n", filepath.ToSlash(defaultMemoryStores[0].dir), filepath.ToSlash(defaultMemoryStores[1].dir))
				return nil
			}
			if len(findings) == 0 {
				cmd.Println("ok, no memory findings")
				return nil
			}
			printLintFindings(cmd, findings)
			return lintExitErr(findings, strict)
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "Treat warnings as errors.")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print findings as JSON on stdout.")
	return cmd
}

func newMemoryIndexCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "index",
		Short: "Rebuild each MEMORY.md from its fact files",
		Long: "Rewrites each store's MEMORY.md without merge conflict markers, links to missing " +
			"files, or repeated links. Every other line keeps its text and order. A fact without " +
			"a line gets `- [name](file.md): description` from its frontmatter.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			stores, err := memoryStores()
			if err != nil {
				return err
			}
			for _, s := range stores {
				c, ok, err := loadMemoryStore(s)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				index := c.rebuildIndex()
				current := ""
				if len(c.index) > 0 {
					current = strings.Join(c.index, "\n") + "\n"
				}
				if index == current {
					cmd.Printf("%s: up to date\n", filepath.ToSlash(c.indexPath()))
					continue
				}
				if err := os.WriteFile(c.indexPath(), []byte(index), 0o644); err != nil {
					return fmt.Errorf("%s: %w", c.indexPath(), err)
				}
				cmd.Printf("%s: rebuilt, %d fact(s)\n", filepath.ToSlash(c.indexPath()), len(c.topics))
			}
			return nil
		},
	}
}

func newMemoryListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List each fact's scope, type, and title",
		Long: "Prints one line per fact file, project memory first, in index order. The title is " +
			"the index line's link text, or the fact's name when no line links it.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			stores, err := memoryStores()
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			for _, s := range stores {
				c, ok, err := loadMemoryStore(s)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				for _, f := range c.facts() {
					typ := f.typ
					if typ == "" {
						typ = "-"
					}
					_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", c.scope, typ, f.name)
				}
			}
			return w.Flush()
		},
	}
}

// facts returns the topics in index order, then the unlinked ones, each
// named by its index title when it has one.
func (c memoryContents) facts() []memoryTopic {
	byFile := map[string]memoryTopic{}
	for _, t := range c.topics {
		byFile[t.file] = t
	}
	var out []memoryTopic
	for _, e := range c.entries {
		t, ok := byFile[e.target]
		if !ok {
			continue
		}
		delete(byFile, e.target)
		t.name = e.title
		out = append(out, t)
	}
	for _, t := range c.topics {
		if _, ok := byFile[t.file]; ok {
			t.name = t.title()
			out = append(out, t)
		}
	}
	return out
}

func newMemoryPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print each memory folder's absolute path",
		Long: "Prints one line per store, project memory first: its scope, then its absolute folder. " +
			"It finds the project from the working directory or any folder below it, and prints a folder " +
			"that does not exist yet. With memory.personal: repo, every worktree of the repository gets " +
			"the same personal folder. Tools with no session-start hook run this to find where to save.",
		Example: "  agnostic-ai memory path",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root := memoryProjectRoot("")
			if root == "" {
				return errors.New("no project found: run this inside a project with an agnostic-ai.yaml or a Git checkout")
			}
			stores, err := memoryStoresAt(root)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			for _, store := range stores {
				dir := store.dir
				if !filepath.IsAbs(dir) {
					dir = filepath.Join(root, dir)
				}
				_, _ = fmt.Fprintf(w, "%s\t%s\n", store.scope, filepath.ToSlash(dir))
			}
			return w.Flush()
		},
	}
}
