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
	"time"

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
	var only []string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Copy facts from another memory store into personal memory",
		Long: "Copies each fact file from another memory store into this project's personal memory and adds its index line. " +
			"--from claude reads Claude Code's own auto memory store, where Claude saved before sync pointed it at personal memory. " +
			"--from checkout and --from repo read the other personal store after a memory.personal switch. " +
			"--path reads any folder with a MEMORY.md and one Markdown file per fact, such as Gemini CLI's ~/.gemini/tmp/<project>/memory or Qoder's ~/.qoder/projects/<project>/memory. " +
			"Every tool reads personal memory, so read the facts --dry-run lists and pass --only <file> to copy just some. " +
			"The source never changes. A fact whose file name personal memory already has stays as it is, " +
			"and a symlink or a file with a line that looks like a secret is skipped; the report names each.",
		Example: `  # Bring over what Claude Code saved before sync redirected it
  agnostic-ai memory import --from claude --dry-run
  agnostic-ai memory import --from claude

  # Copy only the facts every tool should read
  agnostic-ai memory import --from claude --only tabs.md --only ci-ubuntu.md

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
			label := from
			if label == "" {
				label = "imported"
			}
			if dryRun {
				plan, err := planMemoryImport(source, target, label, only)
				if err != nil {
					return err
				}
				plan.print(cmd.OutOrStdout(), true)
				return nil
			}
			plan, _, err := importMemory(source, target, label, only, false)
			if err != nil {
				return err
			}
			plan.print(cmd.OutOrStdout(), false)
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "Store to copy from: "+strings.Join(memoryImportSources, ", ")+".")
	cmd.Flags().StringVar(&path, "path", "", "Folder to copy from, holding MEMORY.md and one Markdown file per fact.")
	cmd.Flags().StringSliceVar(&only, "only", nil, "Copy only this fact file; repeat the flag for more.")
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

// memoryCopy is one fact file import writes, with the index line it gets.
type memoryCopy struct {
	file   string
	body   []byte
	line   string
	record string
}

// memoryImportPlan is what import copies from one store into another.
type memoryImportPlan struct {
	source, target string
	// copies are the facts the target lacks.
	copies []memoryCopy
	// present are the facts the target holds byte for byte. Import adds
	// the index line of each one the target index lacks, which also
	// finishes an import that stopped part way.
	present []memoryCopy
	// kept names the facts the target holds with other content.
	kept []string
	// relined names the facts whose source index line looks like a secret,
	// so they get a line built from their own frontmatter instead.
	relined []string
	// skipped names each file left out, with the reason.
	skipped []memorySkip
	// label names the source in the record of handled facts.
	label string
}

// memorySkip is a source file import leaves out, and why.
type memorySkip struct{ file, reason string }

func (s memorySkip) String() string { return s.file + ": " + s.reason }

// importMemory copies the facts in source into the personal store at
// target while holding the store's lock. With intoEmpty it copies nothing
// once the store holds a Markdown file, and reports false. label names the
// source in the fact that keeps its index notes.
func importMemory(source, target, label string, only []string, intoEmpty bool) (memoryImportPlan, bool, error) {
	if intoEmpty && hasMarkdownFile(target) {
		return memoryImportPlan{}, false, nil
	}
	var err error
	target, err = effectiveMemoryPath(target)
	if err != nil {
		return memoryImportPlan{}, false, err
	}
	if err := guardStoreFromGit(target); err != nil {
		return memoryImportPlan{}, false, err
	}
	if err := os.MkdirAll(target, 0o700); err != nil {
		return memoryImportPlan{}, false, fmt.Errorf("%s: %w", target, err)
	}
	unlock, err := lockMemoryStore(target)
	if err != nil {
		return memoryImportPlan{}, false, err
	}
	defer unlock()
	if intoEmpty && hasMarkdownFile(target) {
		return memoryImportPlan{}, false, nil
	}
	plan, err := planMemoryImport(source, target, label, only)
	if err != nil {
		return plan, false, err
	}
	if intoEmpty && (label == memorySourceCheckout || label == memorySourceRepo) {
		handled := importedFacts(target)
		plan.copies = slices.DeleteFunc(plan.copies, func(fact memoryCopy) bool {
			record := fact.record
			if record == "" {
				record = fact.file
			}
			return handled[label+"/"+record]
		})
	}
	return plan, true, plan.apply()
}

// planMemoryImport compares the fact files in source with the store at
// target by file name, and reads the source index line of each fact. A
// fact with no line gets one from its frontmatter. Lines of the source
// index that link no fact become one more fact, so notes a tool kept in
// its index come along too.
func planMemoryImport(source, target, label string, only []string) (memoryImportPlan, error) {
	plan := memoryImportPlan{source: source, target: target, label: label}
	entries, err := os.ReadDir(source)
	if err != nil {
		return plan, fmt.Errorf("%s: %w", source, err)
	}
	var facts []memoryCopy
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".md" || name == memoryIndexFile {
			continue
		}
		if !e.Type().IsRegular() {
			plan.skipped = append(plan.skipped, memorySkip{name, "not a regular file"})
			continue
		}
		body, err := readMemoryFact(filepath.Join(source, name))
		if err != nil {
			plan.skipped = append(plan.skipped, memorySkip{name, err.Error()})
			continue
		}
		facts = append(facts, memoryCopy{file: name, body: body})
	}
	lines := sourceIndexLines(source)
	for i, f := range facts {
		if line, ok := lines[f.file]; ok {
			facts[i].line = line
		} else {
			facts[i].line = parseMemoryTopic(f.file, string(f.body)).indexLine()
		}
	}
	if notes := sourceIndexNotes(source); len(notes) > 0 {
		fact, err := uniqueMemoryNotesFact(source, target, label, notes)
		if err != nil {
			return plan, err
		}
		facts = append(facts, fact)
	}
	if len(only) > 0 {
		for _, name := range only {
			if !slices.ContainsFunc(facts, func(f memoryCopy) bool { return f.file == name }) {
				return plan, fmt.Errorf("--only %s: %s has no such fact file", name, filepath.ToSlash(source))
			}
		}
		facts = slices.DeleteFunc(facts, func(f memoryCopy) bool { return !slices.Contains(only, f.file) })
	}
	for _, f := range facts {
		if line := memorySecretLine(string(f.body)); line > 0 {
			plan.skipped = append(plan.skipped, memorySkip{f.file, fmt.Sprintf("line %d looks like a secret", line)})
			continue
		}
		if memorySecretIn(f.line) {
			f.line = parseMemoryTopic(f.file, string(f.body)).indexLine()
			plan.relined = append(plan.relined, f.file)
		}
		existing, err := os.ReadFile(filepath.Join(target, f.file))
		switch {
		case err == nil && bytes.Equal(normalizeNewlines(existing), normalizeNewlines(f.body)):
			plan.present = append(plan.present, f)
		case err == nil:
			plan.kept = append(plan.kept, f.file)
		case errors.Is(err, fs.ErrNotExist):
			plan.copies = append(plan.copies, f)
		default:
			return plan, fmt.Errorf("%s: %w", filepath.Join(target, f.file), err)
		}
	}
	return plan, nil
}

func normalizeNewlines(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
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

// sourceIndex returns the lines of the index in dir, or nil when it is
// missing or not a regular file.
func sourceIndex(dir string) []string {
	body, err := readMemoryFact(filepath.Join(dir, memoryIndexFile))
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimRight(string(normalizeNewlines(body)), "\n"), "\n")
}

// sourceIndexLines returns, for each fact file the index in dir links,
// the first index line that links it.
func sourceIndexLines(dir string) map[string]string {
	out := map[string]string{}
	for _, e := range memoryIndexEntries(sourceIndex(dir)) {
		if _, seen := out[e.target]; !seen {
			out[e.target] = e.text
		}
	}
	return out
}

// sourceIndexNotes returns the lines of the index in dir that link no
// fact and are not blank, or nil when headings are all there is.
func sourceIndexNotes(dir string) []string {
	index := sourceIndex(dir)
	linked := map[int]bool{}
	for _, e := range memoryIndexEntries(index) {
		linked[e.line] = true
	}
	var notes []string
	prose := false
	for i, line := range index {
		if linked[i+1] || strings.TrimSpace(line) == "" || memoryConflictMarker(line) {
			continue
		}
		notes = append(notes, strings.TrimRight(line, " \t"))
		prose = prose || !strings.HasPrefix(strings.TrimSpace(line), "#")
	}
	if !prose {
		return nil
	}
	return notes
}

// memoryNotesLabels describes each import source in the fact that keeps
// the notes from its index.
var memoryNotesLabels = map[string]string{
	memorySourceClaude:   "Claude Code's own memory",
	memorySourceCheckout: "the checkout's personal memory",
	memorySourceRepo:     "the repository's personal memory",
}

// memoryNotesFact keeps the notes of a source index as one fact, since
// the target index holds only links.
func memoryNotesFact(label string, notes []string) memoryCopy {
	what, ok := memoryNotesLabels[label]
	if !ok {
		label, what = "imported", "the imported memory"
	}
	name := label + "-index-notes"
	description := "Notes kept in the MEMORY.md of " + what
	body := "---\nname: " + name + "\ndescription: " + description + "\nmetadata:\n  type: reference\n---\n\n" + strings.Join(notes, "\n") + "\n"
	file := name + ".md"
	return memoryCopy{file: file, body: []byte(body), line: "- [" + name + "](" + file + "): " + description, record: "@index-notes/" + file}
}

func uniqueMemoryNotesFact(source, target, label string, notes []string) (memoryCopy, error) {
	base := memoryNotesFact(label, notes)
	handled := importedFacts(target)
	for n := 1; ; n++ {
		fact := base
		if n > 1 {
			name := strings.TrimSuffix(base.file, ".md")
			renamed := fmt.Sprintf("%s-%d", name, n)
			fact.file = renamed + ".md"
			fact.record = "@index-notes/" + fact.file
			fact.body = bytes.Replace(base.body, []byte("name: "+name+"\n"), []byte("name: "+renamed+"\n"), 1)
			fact.line = strings.ReplaceAll(base.line, name, renamed)
		}
		if _, err := os.Lstat(filepath.Join(source, fact.file)); err == nil {
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fact, fmt.Errorf("%s: %w", source, err)
		}
		if handled[label+"/"+fact.record] {
			return fact, nil
		}
		existing, err := os.ReadFile(filepath.Join(target, fact.file))
		if errors.Is(err, fs.ErrNotExist) || err == nil && bytes.Equal(normalizeNewlines(existing), normalizeNewlines(fact.body)) {
			return fact, nil
		}
		if err != nil {
			return fact, fmt.Errorf("%s: %w", filepath.Join(target, fact.file), err)
		}
	}
}

// apply writes the planned facts into the target store and adds the index
// line of each fact there that its index lacks. It never replaces a file:
// a fact written since the plan stays as it is.
func (p memoryImportPlan) apply() error {
	var err error
	p.target, err = effectiveMemoryPath(p.target)
	if err != nil {
		return err
	}
	if err := guardStoreFromGit(p.target); err != nil {
		return err
	}
	for _, name := range []string{memoryIndexFile, memoryImportRecord} {
		if err := requireRegularOrAbsent(filepath.Join(p.target, name)); err != nil {
			return err
		}
	}
	repo := existingAncestor(p.target)
	var written []memoryCopy
	for _, c := range p.copies {
		path := filepath.Join(p.target, c.file)
		if gitWouldTrack(repo, path) {
			return fmt.Errorf("%s: Git would track it; ignore %s first", filepath.ToSlash(path), filepath.ToSlash(p.target))
		}
		created, err := writeNewFile(path, c.body)
		if err != nil {
			return err
		}
		if created {
			written = append(written, c)
		}
	}
	if err := appendMemoryIndexLines(p.target, append(written, p.present...)); err != nil {
		return err
	}
	return p.record()
}

// The record suppresses warnings for facts deleted or merged after import.
const memoryImportRecord = ".imported"

// record adds each fact the plan handled to the store's record.
func (p memoryImportPlan) record() error {
	var names []string
	for _, c := range append(slices.Clone(p.copies), p.present...) {
		name := c.record
		if name == "" {
			name = c.file
		}
		names = append(names, name)
	}
	names = append(names, p.kept...)
	if len(names) == 0 {
		return nil
	}
	path := filepath.Join(p.target, memoryImportRecord)
	have := importedFacts(p.target)
	var add strings.Builder
	for _, name := range names {
		entry := p.label + "/" + name
		if !have[entry] {
			have[entry] = true
			add.WriteString(entry + "\n")
		}
	}
	if add.Len() == 0 {
		return nil
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	_, err = f.WriteString(add.String())
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// importedFacts reads the record of handled facts in store.
func importedFacts(store string) map[string]bool {
	out := map[string]bool{}
	data, err := os.ReadFile(filepath.Join(store, memoryImportRecord))
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out[line] = true
		}
	}
	return out
}

// requireRegularOrAbsent refuses a path that is a link or anything but a
// regular file, so import never writes through it into another file.
func requireRegularOrAbsent(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file; import never writes through a link", filepath.ToSlash(path))
	}
	return nil
}

// writeNewFile writes body to path only when no file is there, and never
// leaves part of a file: the bytes go to a temporary file first, then a
// hard link puts it in place, which fails when path exists.
func writeNewFile(path string, body []byte) (bool, error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return false, fmt.Errorf("%s: %w", filepath.Dir(path), err)
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	_, err = tmp.Write(body)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(name, 0o644)
	}
	if err != nil {
		return false, fmt.Errorf("%s: %w", name, err)
	}
	err = os.Link(name, path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	// A file system without hard links: create the file exclusively, and
	// remove it again if the write fails.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	_, err = f.Write(body)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return false, fmt.Errorf("%s: %w", path, err)
	}
	return true, nil
}

// appendMemoryIndexLines appends the index line of each fact the target
// index does not link yet. Appending, not rewriting, keeps every line a
// tool adds to the index at the same time.
func appendMemoryIndexLines(target string, facts []memoryCopy) error {
	path := filepath.Join(target, memoryIndexFile)
	current, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%s: %w", path, err)
	}
	linked := map[string]bool{}
	for _, e := range memoryIndexEntries(strings.Split(string(normalizeNewlines(current)), "\n")) {
		linked[e.target] = true
	}
	var add strings.Builder
	if len(current) > 0 && current[len(current)-1] != '\n' {
		add.WriteString("\n")
	}
	lines := 0
	for _, f := range facts {
		if !linked[f.file] {
			linked[f.file] = true
			add.WriteString(f.line + "\n")
			lines++
		}
	}
	if lines == 0 {
		return nil
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	_, err = file.WriteString(add.String())
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// guardStoreFromGit keeps facts that were private to a tool out of Git. It
// refuses a store with files Git tracks, and adds the store to
// info/exclude when Git would track new files there, if the store is in
// the working directory's repository. A store outside any repository
// needs nothing.
func guardStoreFromGit(store string) error {
	var err error
	store, err = effectiveMemoryPath(store)
	if err != nil {
		return err
	}
	repo := existingAncestor(store)
	if out, err := exec.Command("git", "-C", repo, "ls-files", "--", store).Output(); err == nil && len(bytes.TrimSpace(out)) > 0 {
		return fmt.Errorf("%s holds files Git tracks, so imported facts would reach a commit; untrack them with `git rm -r --cached %s`", filepath.ToSlash(store), filepath.ToSlash(store))
	}
	// Import writes facts, the index, its record, its lock, and temporary
	// files; a rule that ignores only Markdown leaves the rest committable.
	probes := []string{memoryIndexFile, memoryImportRecord, ".lock", ".fact.md.0.tmp"}
	trackable := func() bool {
		for _, name := range probes {
			if gitWouldTrack(repo, filepath.Join(store, name)) {
				return true
			}
		}
		return false
	}
	if !trackable() {
		return nil
	}
	top, err := exec.Command("git", "-C", repo, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return fmt.Errorf("%s: %w", repo, err)
	}
	here, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil || canonicalDir(strings.TrimSpace(string(here))) != canonicalDir(strings.TrimSpace(string(top))) {
		return fmt.Errorf("%s: the Git repository in %s would track it; ignore the folder there first", filepath.ToSlash(store), strings.TrimSpace(string(top)))
	}
	rel, err := filepath.Rel(canonicalDir(strings.TrimSpace(string(top))), canonicalDir(store))
	if err != nil {
		return fmt.Errorf("%s: %w", store, err)
	}
	if err := adapters.ExcludeFromGit("/" + filepath.ToSlash(rel) + "/"); err != nil {
		return err
	}
	if trackable() {
		return fmt.Errorf("%s: Git would still track it; ignore the folder in .gitignore", filepath.ToSlash(store))
	}
	return nil
}

// Resolve existing parents too, since a new store may live below a symlink.
func effectiveMemoryPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	ancestor := abs
	var tail []string
	for {
		if _, err := os.Lstat(ancestor); err == nil {
			resolved, err := filepath.EvalSymlinks(ancestor)
			if err != nil {
				return "", fmt.Errorf("%s: %w", ancestor, err)
			}
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("%s: %w", ancestor, err)
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", fmt.Errorf("%s: no existing parent", path)
		}
		tail = append(tail, filepath.Base(ancestor))
		ancestor = parent
	}
}

// gitWouldTrack reports whether the repository holding dir would track a
// new file at path: Git reports the path as not ignored. Outside a
// repository it reports false.
func gitWouldTrack(dir, path string) bool {
	err := exec.Command("git", "-C", dir, "check-ignore", "-q", path).Run()
	var exit *exec.ExitError
	return errors.As(err, &exit) && exit.ExitCode() == 1
}

// existingAncestor returns path, or its nearest parent that exists.
func existingAncestor(path string) string {
	for {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return path
		}
		path = parent
	}
}

// memoryStoreLockWait bounds how long import waits for another import
// into the same store.
const memoryStoreLockWait = 5 * time.Second

// lockMemoryStore holds the lock file inside store until the returned
// func runs, so two imports never append index lines at once.
func lockMemoryStore(store string) (func(), error) {
	path := filepath.Join(store, ".lock")
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	deadline := time.Now().Add(memoryStoreLockWait)
	for {
		err := tryLockProjectFile(file)
		if err == nil {
			return func() { _ = file.Close() }, nil
		}
		if !projectLockContended(err) || time.Now().After(deadline) {
			_ = file.Close()
			return nil, fmt.Errorf("lock %s: %w", path, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
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
			if d := strings.TrimSpace(parseMemoryTopic(c.file, string(c.body)).description); d != "" {
				_, _ = fmt.Fprintf(w, "  %s: %s\n", c.file, d)
			} else {
				_, _ = fmt.Fprintf(w, "  %s\n", c.file)
			}
		}
		if dryRun {
			_, _ = fmt.Fprintln(w, "Every tool reads personal memory. Check each fact above, and pass --only <file> to copy just some.")
		} else {
			_, _ = fmt.Fprintln(w, "Every tool reads personal memory, so these facts now guide all of them. Review them with `agnostic-ai memory list`; delete a file to remove one.")
		}
	case len(p.present) > 0 && len(p.kept)+len(p.skipped) == 0:
		_, _ = fmt.Fprintf(w, "Nothing to copy: %s has every fact in %s.\n", to, from)
	case len(p.present)+len(p.kept)+len(p.skipped) == 0:
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
	if len(p.relined) > 0 {
		_, _ = fmt.Fprintf(w, "Built a new index line for %d fact(s), since the source line looks like a secret:\n", len(p.relined))
		for _, name := range p.relined {
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
		if info, err := os.Stat(seed.From); err != nil || !info.IsDir() {
			continue
		}
		plan, done, err := importMemory(seed.From, seed.To, seed.Target, nil, true)
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

// lintMemoryDarkStores reports each memory store whose facts no tool reads
// any longer, and that the personal store lacks by file name (LINT043):
// Claude Code's own store once sync points Claude at personal memory, and
// the personal store of the other memory.personal mode. It counts facts by
// file name, and notes kept in a store's index as one more, and never
// prints what a fact says.
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
	handled := importedFacts(personal)
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
		if len(sourceIndexNotes(c.dir)) > 0 {
			fact, err := uniqueMemoryNotesFact(c.dir, personal, c.from, sourceIndexNotes(c.dir))
			if err != nil {
				return nil, err
			}
			if !handled[c.from+"/"+fact.record] {
				facts = append(facts, fact.file)
			}
		}
		missing := 0
		for _, f := range facts {
			if !slices.Contains(have, f) && !handled[c.from+"/"+f] {
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
