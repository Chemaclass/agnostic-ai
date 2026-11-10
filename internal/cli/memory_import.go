package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
	probes := []string{memoryIndexFile, memoryImportRecord, memoryDeclineRecord, ".lock", ".fact.md.0.tmp"}
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

// lintMemoryDarkStores reports each memory store whose facts no tool reads
// any longer, and that the personal store lacks by file name (LINT043):
// Claude Code's own store once sync points Claude at personal memory, and
// the personal store of the other memory.personal mode. It counts facts by
// file name, and notes kept in a store's index as one more, leaves out the
// facts declined with the same content, and never prints what a fact says.
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
	declined := declinedFacts(personal)
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
		isDeclined := func(record string, body func() ([]byte, error)) bool {
			sum, ok := declined[c.from+"/"+record]
			if !ok || sum == "" {
				return ok
			}
			b, err := body()
			return err == nil && memoryFactSum(b) == sum
		}
		missing, declinedHere := 0, 0
		if notes := sourceIndexNotes(c.dir); len(notes) > 0 {
			fact, err := uniqueMemoryNotesFact(c.dir, personal, c.from, notes)
			if err != nil {
				return nil, err
			}
			switch {
			case handled[c.from+"/"+fact.record]:
			case isDeclined(fact.record, func() ([]byte, error) { return fact.body, nil }):
				declinedHere++
			default:
				missing++
			}
		}
		for _, f := range facts {
			switch {
			case slices.Contains(have, f) || handled[c.from+"/"+f]:
			case isDeclined(f, func() ([]byte, error) { return readMemoryFact(filepath.Join(c.dir, f)) }):
				declinedHere++
			default:
				missing++
			}
		}
		if missing == 0 {
			continue
		}
		message := fmt.Sprintf("%d fact(s) here no longer load: %s; copy them into personal memory with `agnostic-ai memory import --from %s`, or decline them with --skip <file> or --skip-rest", missing, c.why, c.from)
		if declinedHere > 0 {
			message += fmt.Sprintf("; %d declined", declinedHere)
		}
		out = append(out, lintFinding{
			Code:     "LINT043",
			Severity: lintWarn,
			Path:     c.dir,
			Message:  message,
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
