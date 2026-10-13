package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// Values `memory import --from` accepts.
const (
	memorySourceClaude   = "claude"
	memorySourceCheckout = config.PersonalMemoryCheckout
	memorySourceRepo     = config.PersonalMemoryRepo
)

var memoryImportSources = []string{memorySourceClaude, memorySourceCheckout, memorySourceRepo}

func newMemoryImportCmd() *cobra.Command {
	var from, path string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Copy facts from another memory store into personal memory",
		Long: "Copies each fact file from another memory store into this project's personal memory and adds its index line. " +
			"--from claude reads Claude Code's own auto memory store, where Claude saved before sync pointed it at personal memory. " +
			"--from checkout and --from repo read the other personal store after a memory.personal switch. " +
			"--path reads any folder with a MEMORY.md and one Markdown file per fact, such as Gemini CLI's ~/.gemini/tmp/<project>/memory or Qoder's ~/.qoder/projects/<project>/memory. " +
			"The source never changes. A fact whose file name personal memory already has stays as it is, " +
			"and a symlink or a file with a line that looks like a secret is skipped; the report names each.",
		Example: `  # Bring over what Claude Code saved before sync redirected it
  agnostic-ai memory import --from claude --dry-run
  agnostic-ai memory import --from claude

  # After switching memory.personal to repo
  agnostic-ai memory import --from checkout`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if (from == "") == (path == "") {
				return errors.New("pass one of --from or --path")
			}
			root := memoryProjectRoot("")
			if root == "" {
				return errors.New("no project found: run this inside a project with an agnostic-ai.yaml or a Git checkout")
			}
			stores, err := memoryStoresAt(root)
			if err != nil {
				return err
			}
			target := stores[1].dir
			if !filepath.IsAbs(target) {
				target = filepath.Join(root, target)
			}
			source, err := memoryImportSource(root, from, path)
			if err != nil {
				return err
			}
			if canonicalDir(source) == canonicalDir(target) {
				return fmt.Errorf("%s is the personal memory store itself; nothing to import", filepath.ToSlash(source))
			}
			plan, err := planMemoryImport(source, target)
			if err != nil {
				return err
			}
			plan.print(cmd.OutOrStdout(), dryRun)
			if dryRun {
				return nil
			}
			return plan.apply()
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "Store to copy from: "+strings.Join(memoryImportSources, ", ")+".")
	cmd.Flags().StringVar(&path, "path", "", "Folder to copy from, holding MEMORY.md and one Markdown file per fact.")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print what would be copied and write nothing.")
	return cmd
}

// memoryImportSource returns the folder `memory import` copies from.
func memoryImportSource(root, from, path string) (string, error) {
	var dir string
	switch from {
	case "":
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("%s: %w", path, err)
		}
		dir = abs
	case memorySourceClaude:
		native, ok, err := adapters.NativeMemoryDir(memorySourceClaude, root)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", errors.New("claude: no native memory store")
		}
		dir = native
	case memorySourceCheckout:
		dir = filepath.Join(root, defaultMemoryStores[1].dir)
	case memorySourceRepo:
		repo, err := adapters.PersonalMemoryDir(&config.Config{Memory: config.MemoryConfig{Personal: config.PersonalMemoryRepo}}, root)
		if err != nil {
			return "", err
		}
		dir = repo
	default:
		return "", fmt.Errorf("--from %q is not one of %s", from, strings.Join(memoryImportSources, ", "))
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		hint := ""
		if from == memorySourceClaude {
			hint = "; Claude Code keeps its auto memory there unless a setting moves it, so pass --path <folder> to copy from another one"
		}
		return "", fmt.Errorf("no memory store at %s%s", filepath.ToSlash(dir), hint)
	}
	return dir, nil
}

// memoryFactMaxBytes bounds the fact files import reads; a fact is a few
// lines, so a larger file is not one.
const memoryFactMaxBytes = 1 << 20

// memoryCopy is one fact file import copies, with the index line it gets.
type memoryCopy struct {
	file string
	body []byte
	line string
}

// memoryImportPlan is what import copies from one store into another.
type memoryImportPlan struct {
	source, target string
	copies         []memoryCopy
	// same counts the facts the target already has, byte for byte.
	same int
	// kept names the facts the target already has with other content.
	kept []string
	// skipped names each file left out, with the reason.
	skipped []string
}

// planMemoryImport compares the fact files in source with the store at
// target by file name, and reads the source index line of each fact it
// will copy. A source fact with no line gets one from its frontmatter.
func planMemoryImport(source, target string) (memoryImportPlan, error) {
	plan := memoryImportPlan{source: source, target: target}
	entries, err := os.ReadDir(source)
	if err != nil {
		return plan, fmt.Errorf("%s: %w", source, err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".md" || name == memoryIndexFile {
			continue
		}
		if !e.Type().IsRegular() {
			plan.skipped = append(plan.skipped, name+": not a regular file")
			continue
		}
		body, err := readMemoryFact(filepath.Join(source, name))
		if err != nil {
			plan.skipped = append(plan.skipped, name+": "+err.Error())
			continue
		}
		if line := memorySecretLine(string(body)); line > 0 {
			plan.skipped = append(plan.skipped, fmt.Sprintf("%s: line %d looks like a secret", name, line))
			continue
		}
		existing, err := os.ReadFile(filepath.Join(target, name))
		if err == nil {
			if bytes.Equal(bytes.ReplaceAll(existing, []byte("\r\n"), []byte("\n")), bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n"))) {
				plan.same++
			} else {
				plan.kept = append(plan.kept, name)
			}
			continue
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return plan, fmt.Errorf("%s: %w", filepath.Join(target, name), err)
		}
		plan.copies = append(plan.copies, memoryCopy{file: name, body: body})
	}
	lines := sourceIndexLines(source)
	for i, c := range plan.copies {
		if line, ok := lines[c.file]; ok {
			plan.copies[i].line = line
		} else {
			plan.copies[i].line = parseMemoryTopic(c.file, string(c.body)).indexLine()
		}
	}
	return plan, nil
}

// readMemoryFact reads one fact file, refusing a symlink on the way and a
// file too large to be one fact.
func readMemoryFact(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	body, err := io.ReadAll(io.LimitReader(f, memoryFactMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > memoryFactMaxBytes {
		return nil, errors.New("larger than 1 MiB")
	}
	return body, nil
}

// memorySecretLine returns the first line of text that looks like a
// secret, counting from 1, or 0.
func memorySecretLine(text string) int {
	for i, line := range strings.Split(text, "\n") {
		if memorySecretIn(line) {
			return i + 1
		}
	}
	return 0
}

// sourceIndexLines returns, for each fact file the index in dir links,
// the first index line that links it. A missing or linked index has none.
func sourceIndexLines(dir string) map[string]string {
	out := map[string]string{}
	body, err := readMemoryFact(filepath.Join(dir, memoryIndexFile))
	if err != nil {
		return out
	}
	text := strings.ReplaceAll(string(body), "\r\n", "\n")
	for _, e := range memoryIndexEntries(strings.Split(text, "\n")) {
		if _, seen := out[e.target]; !seen {
			out[e.target] = e.text
		}
	}
	return out
}

// apply copies the planned facts into the target store and appends their
// index lines. It never replaces a file, so a fact created since the plan
// stays as it is.
func (p memoryImportPlan) apply() error {
	if len(p.copies) == 0 {
		return nil
	}
	if err := keepStoreOutOfGit(p.target); err != nil {
		return err
	}
	if err := os.MkdirAll(p.target, 0o700); err != nil {
		return fmt.Errorf("%s: %w", p.target, err)
	}
	var added []string
	for _, c := range p.copies {
		path := filepath.Join(p.target, c.file)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		_, err = f.Write(c.body)
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		added = append(added, c.file)
	}
	return appendMemoryIndexLines(p.target, p.copies, added)
}

// keepStoreOutOfGit adds the checkout's personal store to the
// repository's info/exclude when Git would track it, so facts that were
// private to a tool never reach a commit. A store outside the repository
// needs nothing.
func keepStoreOutOfGit(store string) error {
	err := exec.Command("git", "check-ignore", "-q", filepath.Join(store, memoryIndexFile)).Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		return nil
	}
	return adapters.ExcludeFromGit("**/" + filepath.ToSlash(defaultMemoryStores[1].dir) + "/")
}

// appendMemoryIndexLines adds the index line of each added fact the
// target index does not link yet, keeping every line it has.
func appendMemoryIndexLines(target string, copies []memoryCopy, added []string) error {
	path := filepath.Join(target, memoryIndexFile)
	current, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%s: %w", path, err)
	}
	text := strings.ReplaceAll(string(current), "\r\n", "\n")
	linked := map[string]bool{}
	for _, e := range memoryIndexEntries(strings.Split(text, "\n")) {
		linked[e.target] = true
	}
	var lines []string
	for _, c := range copies {
		if slices.Contains(added, c.file) && !linked[c.file] {
			lines = append(lines, c.line)
		}
	}
	if len(lines) == 0 {
		return nil
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// print reports the plan, or what it would do with dryRun.
func (p memoryImportPlan) print(w io.Writer, dryRun bool) {
	from, to := filepath.ToSlash(p.source), filepath.ToSlash(p.target)
	verb := "Copied"
	if dryRun {
		verb = "Would copy"
	}
	switch {
	case len(p.copies) > 0:
		_, _ = fmt.Fprintf(w, "%s %d fact(s) from %s to %s:\n", verb, len(p.copies), from, to)
		for _, c := range p.copies {
			_, _ = fmt.Fprintf(w, "  %s\n", c.file)
		}
	case p.same > 0 && len(p.kept)+len(p.skipped) == 0:
		_, _ = fmt.Fprintf(w, "Nothing to copy: %s has every fact in %s.\n", to, from)
	case p.same+len(p.kept)+len(p.skipped) == 0:
		_, _ = fmt.Fprintf(w, "No facts in %s.\n", from)
	default:
		_, _ = fmt.Fprintf(w, "Nothing copied from %s.\n", from)
	}
	if len(p.kept) > 0 {
		_, _ = fmt.Fprintf(w, "Kept %d fact(s) personal memory has with other content; merge them by hand:\n", len(p.kept))
		for _, name := range p.kept {
			_, _ = fmt.Fprintf(w, "  %s\n", name)
		}
	}
	if len(p.skipped) > 0 {
		_, _ = fmt.Fprintf(w, "Skipped %d file(s):\n", len(p.skipped))
		for _, s := range p.skipped {
			_, _ = fmt.Fprintf(w, "  %s\n", s)
		}
	}
}

// runMemorySeeds copies, for each seed an adapter queued this sync, the
// source facts into its personal store when that store holds no Markdown
// file yet, and notes the result.
func runMemorySeeds() {
	for _, seed := range adapters.TakeMemorySeeds() {
		if hasMarkdownFile(seed.To) {
			continue
		}
		if info, err := os.Stat(seed.From); err != nil || !info.IsDir() {
			continue
		}
		plan, err := planMemoryImport(seed.From, seed.To)
		if err == nil && len(plan.copies) == 0 {
			continue
		}
		if err == nil {
			err = plan.apply()
		}
		from := filepath.ToSlash(seed.From)
		if err != nil {
			adapters.NoteProject(fmt.Sprintf("%s: could not copy the facts in %s into personal memory: %v; run `agnostic-ai memory import --from %s`", seed.Target, from, err, seed.Target))
			continue
		}
		adapters.NoteProject(fmt.Sprintf("%s: copied %d fact(s) from %s into personal memory, which was empty; the source is unchanged", seed.Target, len(plan.copies), from))
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

// lintMemoryDarkStores reports each memory store whose facts no tool reads
// any longer, and that the personal store lacks by file name (LINT043):
// Claude Code's own store once sync points Claude at personal memory, and
// the personal store of the other memory.personal mode. It compares file
// names only, never contents.
func lintMemoryDarkStores(readers memoryReaders, root string) ([]lintFinding, error) {
	if readers.cfg == nil {
		return nil, nil
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", root, err)
	}
	stores, err := memoryStoresAt(root)
	if err != nil {
		return nil, err
	}
	personal := stores[1].dir
	if !filepath.IsAbs(personal) {
		personal = filepath.Join(root, personal)
	}
	have, err := adapters.MemoryFactFiles(personal)
	if err != nil {
		return nil, err
	}
	type darkStore struct{ dir, from, why string }
	var candidates []darkStore
	if slices.Contains(readers.cfg.Targets, memorySourceClaude) && claudeReadsPersonalMemory(readers.cfg, root, personal) {
		if native, ok, err := adapters.NativeMemoryDir(memorySourceClaude, root); err == nil && ok {
			candidates = append(candidates, darkStore{native, memorySourceClaude, "Claude Code saved them before sync pointed it at personal memory"})
		}
	}
	if readers.cfg.RepoPersonalMemory() {
		candidates = append(candidates, darkStore{filepath.Join(root, defaultMemoryStores[1].dir), memorySourceCheckout, "they are in the checkout store, and memory.personal is repo"})
	} else if repo, err := adapters.PersonalMemoryDir(&config.Config{Memory: config.MemoryConfig{Personal: config.PersonalMemoryRepo}}, root); err == nil {
		candidates = append(candidates, darkStore{repo, memorySourceRepo, "they are in the repository store, and memory.personal is checkout"})
	}
	var out []lintFinding
	for _, c := range candidates {
		if canonicalDir(c.dir) == canonicalDir(personal) {
			continue
		}
		facts, err := adapters.MemoryFactFiles(c.dir)
		if err != nil {
			return nil, err
		}
		missing := 0
		for _, f := range facts {
			if !slices.Contains(have, f) {
				missing++
			}
		}
		if missing == 0 {
			continue
		}
		out = append(out, lintFinding{
			Code:     "LINT043",
			Severity: lintWarn,
			Path:     c.dir,
			Message:  fmt.Sprintf("%d fact(s) here no longer load: %s; copy them into personal memory with `agnostic-ai memory import --from %s`", missing, c.why, c.from),
		})
	}
	return out, nil
}

// claudeReadsPersonalMemory reports whether the Claude Code settings sync
// writes point auto memory at the personal store, so Claude no longer
// reads its own default store. A value the user set elsewhere keeps it.
func claudeReadsPersonalMemory(cfg *config.Config, root, personal string) bool {
	path := filepath.Join(root, claudeLayoutFor(cfg).dir, "settings.local.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var doc map[string]any
	if json.Unmarshal(data, &doc) != nil {
		return false
	}
	dir, _ := doc["autoMemoryDirectory"].(string)
	return dir != "" && canonicalDir(filepath.FromSlash(dir)) == canonicalDir(personal)
}
