package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
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
	var pick memoryImportPick
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Copy facts from another memory store into personal memory",
		Long: "Copies each fact file from another memory store into this project's personal memory and adds its index line. " +
			"--from claude reads Claude Code's own auto memory store, where Claude saved before sync pointed it at personal memory. " +
			"--from checkout and --from repo read the other personal store after a memory.personal switch. " +
			"--path reads any folder with a MEMORY.md and one Markdown file per fact, such as Gemini CLI's ~/.gemini/tmp/<project>/memory or Qoder's ~/.qoder/projects/<project>/memory. " +
			"Every tool reads personal memory, so read the facts --dry-run lists and pass --only <file> to copy just some. " +
			"--skip <file> declines a fact and --skip-rest declines every fact not imported yet, so LINT043 stops counting them; " +
			"without --only they copy nothing. Declines go to .declined in personal memory, and a declined fact that changes counts again. " +
			"The source never changes. A fact whose file name personal memory already has stays as it is, " +
			"and a symlink or a file with a line that looks like a secret is skipped; the report names each.",
		Example: `  # Bring over what Claude Code saved before sync redirected it
  agnostic-ai memory import --from claude --dry-run
  agnostic-ai memory import --from claude

  # Copy only the facts every tool should read
  agnostic-ai memory import --from claude --only tabs.md --only ci-ubuntu.md

  # Decline the facts left, so lint stops counting them
  agnostic-ai memory import --from claude --skip-rest

  # After switching memory.personal to repo
  agnostic-ai memory import --from checkout`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if (from == "") == (path == "") {
				return errors.New("pass one of --from or --path")
			}
			for _, name := range pick.skip {
				if slices.Contains(pick.only, name) {
					return fmt.Errorf("%s is in both --only and --skip; pick one", name)
				}
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
				plan, err := planMemoryImport(source, target, label, pick)
				if err != nil {
					return err
				}
				plan.print(cmd.OutOrStdout(), true)
				return nil
			}
			plan, _, err := importMemory(source, target, label, pick, false)
			if err != nil {
				return err
			}
			plan.print(cmd.OutOrStdout(), false)
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "Store to copy from: "+strings.Join(memoryImportSources, ", ")+".")
	cmd.Flags().StringVar(&path, "path", "", "Folder to copy from, holding MEMORY.md and one Markdown file per fact.")
	cmd.Flags().StringSliceVar(&pick.only, "only", nil, "Copy only this fact file; repeat the flag for more.")
	cmd.Flags().StringSliceVar(&pick.skip, "skip", nil, "Decline this fact file so lint stops counting it; repeat the flag for more.")
	cmd.Flags().BoolVar(&pick.skipRest, "skip-rest", false, "Decline every fact not imported yet, after copying the --only picks.")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print which facts are imported, declined and pending, and write nothing.")
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

// memoryImportPick is what the person chose to copy and to decline.
type memoryImportPick struct {
	only, skip []string
	skipRest   bool
}

func (p memoryImportPick) declines() bool { return len(p.skip) > 0 || p.skipRest }

// memoryCopy is one fact file import writes, with the index line it gets.
type memoryCopy struct {
	file   string
	body   []byte
	line   string
	record string
}

// recordName is the fact's name in the records of imported and declined
// facts.
func (c memoryCopy) recordName() string {
	if c.record != "" {
		return c.record
	}
	return c.file
}

// memoryDecline is a fact to add to the record of declined facts. An empty
// sum declines it by name, for a file import could not read.
type memoryDecline struct{ file, record, sum string }

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
	// imported names the facts personal memory has by name or by record,
	// outside the ones this run copies.
	imported []string
	// declined names the facts declined before, left out of this run.
	declined []string
	// changed names the facts declined before whose content changed since,
	// so they are pending again.
	changed []string
	// pending names the facts neither imported nor declined that this run
	// leaves alone.
	pending []string
	// toDecline are the facts this run declines.
	toDecline []memoryDecline
	// skipImported names the --skip facts already imported, so not declined.
	skipImported []string
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
func importMemory(source, target, label string, pick memoryImportPick, intoEmpty bool) (memoryImportPlan, bool, error) {
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
	plan, err := planMemoryImport(source, target, label, pick)
	if err != nil {
		return plan, false, err
	}
	if intoEmpty && (label == memorySourceCheckout || label == memorySourceRepo) {
		handled := importedFacts(target)
		plan.copies = slices.DeleteFunc(plan.copies, func(fact memoryCopy) bool {
			return handled[label+"/"+fact.recordName()]
		})
	}
	return plan, true, plan.apply()
}

// planMemoryImport compares the fact files in source with the store at
// target by file name, and reads the source index line of each fact. A
// fact with no line gets one from its frontmatter. Lines of the source
// index that link no fact become one more fact, so notes a tool kept in
// its index come along too. A fact declined with the same content stays
// out unless --only names it; pick also says which facts to decline.
func planMemoryImport(source, target, label string, pick memoryImportPick) (memoryImportPlan, error) {
	plan := memoryImportPlan{source: source, target: target, label: label}
	entries, err := os.ReadDir(source)
	if err != nil {
		return plan, fmt.Errorf("%s: %w", source, err)
	}
	handled := importedFacts(target)
	declined := declinedFacts(target)
	isImported := func(file, record string) bool {
		_, err := os.Lstat(filepath.Join(target, file))
		return err == nil || handled[label+"/"+record]
	}
	declines := func(file string) bool { return pick.skipRest || slices.Contains(pick.skip, file) }
	var facts []memoryCopy
	var unread []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".md" || name == memoryIndexFile {
			continue
		}
		reason := "not a regular file"
		if e.Type().IsRegular() {
			body, err := readMemoryFact(filepath.Join(source, name))
			if err == nil {
				facts = append(facts, memoryCopy{file: name, body: body})
				continue
			}
			reason = err.Error()
		}
		unread = append(unread, name)
		switch sum, ok := declined[label+"/"+name]; {
		case slices.Contains(pick.only, name) || isImported(name, name):
		case declines(name):
			if !ok || sum != "" {
				plan.toDecline = append(plan.toDecline, memoryDecline{file: name, record: name})
			}
			continue
		case ok && sum == "":
			plan.declined = append(plan.declined, name)
			continue
		}
		plan.skipped = append(plan.skipped, memorySkip{name, reason})
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
	for _, name := range pick.only {
		if !slices.ContainsFunc(facts, func(f memoryCopy) bool { return f.file == name }) {
			return plan, fmt.Errorf("--only %s: %s has no such fact file", name, filepath.ToSlash(source))
		}
	}
	for _, name := range pick.skip {
		if !slices.ContainsFunc(facts, func(f memoryCopy) bool { return f.file == name }) && !slices.Contains(unread, name) {
			return plan, fmt.Errorf("--skip %s: %s has no such fact file", name, filepath.ToSlash(source))
		}
	}
	for _, f := range facts {
		record := f.recordName()
		imported := isImported(f.file, record)
		sum := memoryFactSum(f.body)
		was, wasDeclined := declined[label+"/"+record]
		stillDeclined := wasDeclined && !imported && (was == "" || was == sum)
		if wasDeclined && !imported && !stillDeclined {
			plan.changed = append(plan.changed, f.file)
		}
		if !slices.Contains(pick.only, f.file) {
			if len(pick.only) > 0 || pick.declines() {
				switch {
				case imported:
					plan.imported = append(plan.imported, f.file)
					if slices.Contains(pick.skip, f.file) {
						plan.skipImported = append(plan.skipImported, f.file)
					}
				case declines(f.file) && was != sum:
					plan.toDecline = append(plan.toDecline, memoryDecline{file: f.file, record: record, sum: sum})
				case stillDeclined:
					plan.declined = append(plan.declined, f.file)
				default:
					plan.pending = append(plan.pending, f.file)
				}
				continue
			}
			if stillDeclined {
				plan.declined = append(plan.declined, f.file)
				continue
			}
		}
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

// memoryFactSum fingerprints a fact's content, so a declined fact that
// changes counts again.
func memoryFactSum(body []byte) string {
	sum := sha256.Sum256(normalizeNewlines(body))
	return "sha256:" + hex.EncodeToString(sum[:])
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
	for _, name := range []string{memoryIndexFile, memoryImportRecord, memoryDeclineRecord} {
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
	if err := p.record(); err != nil {
		return err
	}
	return p.decline()
}

// The record suppresses warnings for facts deleted or merged after import.
const memoryImportRecord = ".imported"

// record adds each fact the plan handled to the store's record.
func (p memoryImportPlan) record() error {
	var names []string
	for _, c := range append(slices.Clone(p.copies), p.present...) {
		names = append(names, c.recordName())
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
	return appendPrivateFile(path, add.String())
}

// appendPrivateFile starts text on a line of its own, even after a record
// someone edited by hand without a final newline.
func appendPrivateFile(path, text string) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	last := make([]byte, 1)
	if info, statErr := f.Stat(); statErr != nil {
		err = statErr
	} else if info.Size() > 0 {
		if _, err = f.ReadAt(last, info.Size()-1); err == nil && last[0] != '\n' {
			text = "\n" + text
		}
	}
	if err == nil {
		_, err = f.WriteString(text)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// The decline record is separate from .imported, which means handled for
// good and has no sum, so older releases keep reading .imported as before.
// Each line is `<label>/<record>`, then a tab and the fact's sum when import
// could read it; the last line for a fact wins.
const memoryDeclineRecord = ".declined"

// decline adds each fact the plan declines to the store's record.
func (p memoryImportPlan) decline() error {
	if len(p.toDecline) == 0 {
		return nil
	}
	var add strings.Builder
	for _, d := range p.toDecline {
		add.WriteString(p.label + "/" + d.record)
		if d.sum != "" {
			add.WriteString("\t" + d.sum)
		}
		add.WriteString("\n")
	}
	return appendPrivateFile(filepath.Join(p.target, memoryDeclineRecord), add.String())
}

// declinedFacts reads the record of declined facts in store, mapping each
// fact to its sum, or to "" for one declined by name.
func declinedFacts(store string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(filepath.Join(store, memoryDeclineRecord))
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, sum, _ := strings.Cut(strings.TrimSpace(line), "\t")
		if key != "" {
			out[key] = sum
		}
	}
	return out
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
				_, _ = fmt.Fprintf(w, "  %s: %s%s\n", c.file, d, p.changedNote(c.file))
			} else {
				_, _ = fmt.Fprintf(w, "  %s%s\n", c.file, p.changedNote(c.file))
			}
		}
		if dryRun {
			_, _ = fmt.Fprintln(w, "Every tool reads personal memory. Check each fact above, and pass --only <file> to copy just some.")
		} else {
			_, _ = fmt.Fprintln(w, "Every tool reads personal memory, so these facts now guide all of them. Review them with `agnostic-ai memory list`; delete a file to remove one.")
		}
	case len(p.present) > 0 && len(p.kept)+len(p.skipped)+len(p.declined)+len(p.pending)+len(p.toDecline) == 0:
		_, _ = fmt.Fprintf(w, "Nothing to copy: %s has every fact in %s.\n", to, from)
	case len(p.present)+len(p.kept)+len(p.skipped)+len(p.imported)+len(p.declined)+len(p.pending)+len(p.toDecline) == 0:
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
	if dryRun {
		var imported []string
		for _, c := range p.present {
			imported = append(imported, c.file)
		}
		printMemoryFacts(w, append(imported, p.imported...), "Already imported %d fact(s):")
		var pending []string
		for _, name := range p.pending {
			pending = append(pending, name+p.changedNote(name))
		}
		printMemoryFacts(w, pending, "Pending %d fact(s); pass --only <file> to copy one or --skip <file> to decline it:")
	}
	printMemoryFacts(w, p.declined, "Left out %d declined fact(s); pass --only <file> to import one anyway:")
	var declining []string
	for _, d := range p.toDecline {
		declining = append(declining, d.file)
	}
	verb = "Declined"
	if dryRun {
		verb = "Would decline"
	}
	printMemoryFacts(w, declining, verb+" %d fact(s); lint stops counting them, and the source keeps them:")
	printMemoryFacts(w, p.skipImported, "Did not decline %d fact(s) already imported:")
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

// changedNote marks a fact declined before whose content changed since.
func (p memoryImportPlan) changedNote(file string) string {
	if slices.Contains(p.changed, file) {
		return " (changed since declined)"
	}
	return ""
}

func printMemoryFacts(w io.Writer, names []string, header string) {
	if len(names) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, header+"\n", len(names))
	for _, name := range names {
		_, _ = fmt.Fprintf(w, "  %s\n", name)
	}
}
