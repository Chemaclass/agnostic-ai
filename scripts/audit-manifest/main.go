package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

type app struct{ root, lock string }

type artifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type outcome struct {
	Disposition string   `json:"disposition"`
	Reason      string   `json:"reason"`
	Artifact    artifact `json:"artifact"`
}

type assignment struct {
	Target  string   `json:"target"`
	Owner   string   `json:"owner"`
	Outcome *outcome `json:"outcome,omitempty"`
}

type source struct {
	Target  string   `json:"target"`
	Kind    string   `json:"kind"`
	URL     string   `json:"url"`
	Fetch   *fetch   `json:"fetch,omitempty"`
	Outcome *outcome `json:"outcome,omitempty"`
}

type fetch struct {
	Row              []string   `json:"row"`
	ComparisonSHA256 string     `json:"comparison_sha256"`
	Evidence         artifact   `json:"evidence"`
	Artifacts        []artifact `json:"artifacts"`
}

type manifest struct {
	Version           int          `json:"version"`
	ResearchVersion   int          `json:"research_version"`
	ResumeFrom        string       `json:"resume_prior_run_id,omitempty"`
	Reused            []reuse      `json:"reused,omitempty"`
	RunID             string       `json:"run_id"`
	Directory         string       `json:"directory"`
	Repository        string       `json:"repository"`
	SourceLock        string       `json:"source_lock"`
	Created           string       `json:"created"`
	Commit            string       `json:"audited_commit"`
	Context           string       `json:"audited_context_sha256"`
	BuiltinsSince     string       `json:"builtins_since"`
	BuiltinsInventory artifact     `json:"builtins_inventory"`
	BuiltinDeep       []string     `json:"builtin_deep_targets"`
	SourceList        string       `json:"source_list_sha256"`
	LockBaseline      string       `json:"lock_baseline_sha256"`
	Targets           []string     `json:"targets"`
	Inventory         []source     `json:"inventory"`
	Sources           []source     `json:"sources"`
	Assignments       []assignment `json:"assignments"`
	RowsSHA256        string       `json:"rows_sha256,omitempty"`
	Finalized         string       `json:"finalized,omitempty"`
}

func main() {
	f := flag.NewFlagSet("audit-manifest", flag.ContinueOnError)
	root := f.String("root", ".", "repository root")
	lock := f.String("lock", "", "source lock path")
	if err := f.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	abs, err := filepath.Abs(*root)
	if err == nil {
		if *lock == "" {
			*lock = filepath.Join(abs, "scripts/target-audit/sources.lock")
		}
		var out string
		out, err = (app{root: abs, lock: *lock}).run(f.Args())
		if err == nil {
			fmt.Print(out)
			return
		}
	}
	fmt.Fprintln(os.Stderr, "audit-manifest:", err)
	os.Exit(1)
}

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func fileDigest(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return digest(b), nil
}

func (a app) command(name string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	c.Dir = a.root
	b, err := c.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(b)), nil
}

func (a app) inventory() ([]source, []string, error) {
	listed, err := a.command("bash", "scripts/target-facts.sh", "--list")
	if err != nil {
		return nil, nil, err
	}
	targets := strings.Fields(listed)
	if len(targets) == 0 {
		return nil, nil, errors.New("empty target registry")
	}
	rows, err := a.command("bash", append([]string{"scripts/docfetch.sh", "--urls"}, targets...)...)
	if err != nil {
		return nil, nil, err
	}
	var result []source
	seen := map[string]bool{}
	for _, line := range strings.Split(rows, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 3 || !slices.Contains(targets, f[0]) || !slices.Contains([]string{"docs", "changelog", "schema", "code"}, f[1]) || (!strings.HasPrefix(f[2], "https://") && !strings.HasPrefix(f[2], "http://")) {
			return nil, nil, fmt.Errorf("invalid source inventory row %q", line)
		}
		s := source{Target: f[0], Kind: f[1], URL: f[2]}
		if seen[s.key()] {
			return nil, nil, fmt.Errorf("duplicate source %s", s.key())
		}
		seen[s.key()] = true
		result = append(result, s)
	}
	for _, target := range targets {
		found := false
		for _, s := range result {
			found = found || s.Target == target
		}
		if !found {
			return nil, nil, fmt.Errorf("%s: no source URLs", target)
		}
	}
	return result, targets, nil
}

func (s source) key() string { return s.Target + "\t" + s.Kind + "\t" + s.URL }

func (a app) context() (string, string, error) {
	commit, err := a.command("git", "rev-parse", "HEAD")
	if err != nil {
		return "", "", err
	}
	paths, err := a.command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return "", "", err
	}
	set := map[string]bool{}
	for _, path := range strings.Split(paths, "\x00") {
		if path != "" {
			set[path] = true
		}
	}
	data := filepath.Join(a.root, "internal/builtins/data")
	err = filepath.WalkDir(data, func(path string, d fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return nil
		}
		if walkErr != nil {
			return fmt.Errorf("%s: %w", path, walkErr)
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(a.root, path)
		if relErr != nil {
			return relErr
		}
		set[rel] = true
		return nil
	})
	if err != nil {
		return "", "", err
	}
	var names []string
	for path := range set {
		if full := filepath.Join(a.root, path); full != a.lock && full != a.lock+".audit-finalizing" {
			names = append(names, path)
		}
	}
	sort.Strings(names)
	index, err := a.command("git", "ls-files", "--stage", "-z")
	if err != nil {
		return "", "", err
	}
	var b bytes.Buffer
	b.WriteString(index)
	b.WriteByte(0)
	for _, path := range names {
		full := filepath.Join(a.root, path)
		info, statErr := os.Lstat(full)
		if errors.Is(statErr, os.ErrNotExist) {
			fmt.Fprintf(&b, "%s\x00deleted\x00", path)
			continue
		}
		if statErr != nil {
			return "", "", fmt.Errorf("%s: %w", full, statErr)
		}
		var sum string
		if info.Mode()&os.ModeSymlink != 0 {
			target, linkErr := os.Readlink(full)
			if linkErr != nil {
				return "", "", fmt.Errorf("%s: %w", full, linkErr)
			}
			sum = digest([]byte(target))
		} else {
			var readErr error
			sum, readErr = fileDigest(full)
			if readErr != nil {
				return "", "", readErr
			}
		}
		fmt.Fprintf(&b, "%s\x00%s\x00%s\x00", path, info.Mode(), sum)
	}
	return commit, digest(b.Bytes()), nil
}

func (a app) lockDigest() (string, error) {
	s, err := fileDigest(a.lock)
	if errors.Is(err, os.ErrNotExist) {
		return "absent", nil
	}
	return s, err
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return filepath.Join(parent, filepath.Base(abs)), nil
}

func atomicWrite(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".audit-*")
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(f.Name(), 0o644)
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func save(m *manifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	return atomicWrite(filepath.Join(m.Directory, "audit.json"), append(b, '\n'))
}

func load(dir string) (*manifest, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	path := filepath.Join(abs, "audit.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	var m manifest
	if err = d.Decode(&m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err = d.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("%s: trailing JSON", path)
	}
	if m.Version != 1 {
		return nil, fmt.Errorf("%s: unsupported version %d", path, m.Version)
	}
	_, idErr := hex.DecodeString(m.RunID)
	if idErr != nil || len(m.RunID) != 32 || m.Directory != abs || m.Created == "" {
		return nil, fmt.Errorf("%s: invalid run identity", path)
	}
	return &m, nil
}

func (a app) start(args []string) (string, error) {
	f := flag.NewFlagSet("start", flag.ContinueOnError)
	out := f.String("out", "", "fresh run directory")
	baseline := f.String("builtins-since", "", "completed built-in baseline")
	if err := f.Parse(args); err != nil {
		return "", err
	}
	inventory, registered, err := a.inventory()
	if err != nil {
		return "", err
	}
	targets := f.Args()
	if len(targets) == 0 {
		targets = registered
	}
	seen := map[string]bool{}
	for _, target := range targets {
		if !slices.Contains(registered, target) || seen[target] {
			return "", fmt.Errorf("unknown or duplicate requested target %q", target)
		}
		seen[target] = true
	}
	if *baseline != "" {
		*baseline, err = a.command("git", "rev-parse", "--verify", *baseline+"^{commit}")
		if err != nil {
			return "", err
		}
	}
	commit, context, err := a.context()
	if err != nil {
		return "", err
	}
	sourceList, err := fileDigest(filepath.Join(a.root, ".agnostic-ai/skills/target-audit/references/sources.md"))
	if err != nil {
		return "", err
	}
	lock, err := a.lockDigest()
	if err != nil {
		return "", err
	}
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		return "", fmt.Errorf("run identity: %w", err)
	}
	runID := hex.EncodeToString(id)
	if *out == "" {
		*out = filepath.Join(a.root, "local/target-audit", time.Now().UTC().Format("2006-01-02T150405Z")+"-"+runID)
	}
	abs, err := filepath.Abs(*out)
	if err != nil {
		return "", fmt.Errorf("%s: %w", *out, err)
	}
	if err = os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", fmt.Errorf("%s: %w", abs, err)
	}
	if err = os.Mkdir(abs, 0o755); err != nil {
		return "", fmt.Errorf("%s: fresh run directory required: %w", abs, err)
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("%s: %w", *out, err)
	}
	m := &manifest{Version: 1, ResearchVersion: researchVersion, RunID: runID, Directory: abs, Created: time.Now().UTC().Format(time.RFC3339), Commit: commit, Context: context, BuiltinsSince: *baseline, SourceList: sourceList, LockBaseline: lock, Targets: targets, Inventory: inventory, Assignments: []assignment{}}
	m.Repository, err = filepath.EvalSymlinks(a.root)
	if err != nil {
		return "", fmt.Errorf("%s: %w", a.root, err)
	}
	m.SourceLock, err = canonicalPath(a.lock)
	if err != nil {
		return "", err
	}
	builtins, err := a.command("bash", "scripts/target-facts.sh", "--builtins")
	if err != nil {
		return "", err
	}
	if err = atomicWrite(filepath.Join(abs, "builtins.txt"), []byte(builtins+"\n")); err != nil {
		return "", err
	}
	m.BuiltinsInventory, err = getArtifact(abs, "builtins.txt")
	if err != nil {
		return "", err
	}
	for _, s := range inventory {
		if seen[s.Target] {
			m.Sources = append(m.Sources, s)
		}
	}
	if err = atomicWrite(filepath.Join(abs, "requested-targets.txt"), []byte(strings.Join(targets, "\n")+"\n")); err != nil {
		return "", err
	}
	if err = save(m); err != nil {
		return "", err
	}
	return abs + "\n", nil
}

func equalSources(a, b []source) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].key() != b[i].key() {
			return false
		}
	}
	return true
}

func (a app) inputs(m *manifest) error {
	return a.inputsAtLock(m, m.LockBaseline)
}

func (a app) inputsAtLock(m *manifest, expectedLock string) error {
	if m.ResearchVersion != researchVersion {
		return errors.New("research policy/extraction version changed; start a fresh run")
	}
	root, err := filepath.EvalSymlinks(a.root)
	if err != nil {
		return fmt.Errorf("%s: %w", a.root, err)
	}
	lockPath, err := canonicalPath(a.lock)
	if err != nil {
		return err
	}
	if root != m.Repository || lockPath != m.SourceLock {
		return errors.New("audited repository or source-lock path changed")
	}
	if err := checkArtifact(m.Directory, m.BuiltinsInventory); err != nil {
		return fmt.Errorf("built-in inventory: %w", err)
	}
	commit, context, err := a.context()
	if err != nil {
		return err
	}
	if commit != m.Commit || context != m.Context {
		return errors.New("audited repository context changed; start a fresh run")
	}
	list, err := fileDigest(filepath.Join(a.root, ".agnostic-ai/skills/target-audit/references/sources.md"))
	if err != nil {
		return err
	}
	if list != m.SourceList {
		return errors.New("source-list digest changed; start a fresh run")
	}
	lock, err := a.lockDigest()
	if err != nil {
		return err
	}
	if lock != expectedLock {
		return errors.New("source-lock baseline changed; start a fresh run")
	}
	inventory, registered, err := a.inventory()
	if err != nil {
		return err
	}
	if !equalSources(inventory, m.Inventory) {
		return errors.New("source inventory changed")
	}
	seen := map[string]bool{}
	for _, target := range m.Targets {
		if seen[target] || !slices.Contains(registered, target) {
			return fmt.Errorf("unknown or duplicate target %q", target)
		}
		seen[target] = true
	}
	if len(seen) == 0 {
		return errors.New("missing requested targets")
	}
	requested, err := os.ReadFile(filepath.Join(m.Directory, "requested-targets.txt"))
	if err != nil {
		return fmt.Errorf("requested targets: %w", err)
	}
	if string(requested) != strings.Join(m.Targets, "\n")+"\n" {
		return errors.New("requested targets changed")
	}
	var expected []source
	for _, s := range inventory {
		if seen[s.Target] {
			expected = append(expected, s)
		}
	}
	if !equalSources(expected, m.Sources) {
		return errors.New("missing, extra or unknown source identities")
	}
	return nil
}

func getArtifact(dir, path string) (artifact, error) {
	if path == "" || filepath.IsAbs(path) {
		return artifact{}, fmt.Errorf("%q: artifact must be relative to the run directory", path)
	}
	full := filepath.Join(dir, path)
	real, err := filepath.EvalSymlinks(full)
	if err != nil {
		return artifact{}, fmt.Errorf("%s: %w", full, err)
	}
	rel, err := filepath.Rel(dir, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return artifact{}, fmt.Errorf("%s: artifact escapes run directory", path)
	}
	info, err := os.Stat(real)
	if err != nil {
		return artifact{}, fmt.Errorf("%s: %w", real, err)
	}
	if !info.Mode().IsRegular() {
		return artifact{}, fmt.Errorf("%s: artifact is not a regular file", path)
	}
	hash, err := fileDigest(real)
	return artifact{Path: path, SHA256: hash}, err
}

func checkArtifact(dir string, expected artifact) error {
	actual, err := getArtifact(dir, expected.Path)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("%s: evidence or artifact bytes changed", expected.Path)
	}
	return nil
}

func comparisonValid(dir string, row []string) error {
	if row[7] == "failed" {
		return nil
	}
	path := row[9]
	if strings.HasSuffix(path, ".body") {
		extracted := strings.TrimSuffix(path, ".body") + ".txt"
		if _, err := os.Stat(filepath.Join(dir, extracted)); err == nil {
			path = extracted
		}
	}
	a, err := getArtifact(dir, path)
	if err != nil {
		return err
	}
	if a.SHA256 == row[5] {
		return nil
	}
	c := exec.Command("jq", "-S", "-c", ".", filepath.Join(dir, path))
	b, err := c.Output()
	if err == nil && digest(b) == row[5] {
		return nil
	}
	return fmt.Errorf("%s: normalized comparison hash does not match evidence", path)
}

func (a app) capture(m *manifest) error {
	if m.RowsSHA256 != "" {
		return errors.New("fetch rows already captured; start a fresh run for a new fetch")
	}
	if err := a.inputs(m); err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(m.Directory, "docfetch.tsv"))
	if err != nil {
		return fmt.Errorf("docfetch.tsv: %w", err)
	}
	rows := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 10 || !slices.Contains([]string{"new", "changed", "unchanged", "failed"}, f[7]) {
			return fmt.Errorf("invalid fetch row %q", line)
		}
		key := strings.Join(f[:3], "\t")
		if rows[key] != nil {
			return fmt.Errorf("duplicate fetched source %s", key)
		}
		if f[7] != "failed" && (len(f[5]) != 64 || f[3] != "200") {
			return fmt.Errorf("%s: invalid successful fetch", key)
		}
		rows[key] = f
	}
	if len(rows) != len(m.Sources) {
		return errors.New("fetch rows do not cover the fixed requested source set")
	}
	for i := range m.Sources {
		s := &m.Sources[i]
		row := rows[s.key()]
		if row == nil {
			return fmt.Errorf("%s: source missing from fetch", s.key())
		}
		if err = comparisonValid(m.Directory, row); err != nil {
			return err
		}
		path := row[9]
		if row[7] == "failed" && path == "" {
			path = "failures/" + digest([]byte(s.key())) + ".tsv"
			if err = os.MkdirAll(filepath.Join(m.Directory, "failures"), 0o755); err != nil {
				return fmt.Errorf("failure evidence: %w", err)
			}
			if err = atomicWrite(filepath.Join(m.Directory, path), []byte(strings.Join(row, "\t")+"\n")); err != nil {
				return err
			}
		}
		if row[7] != "failed" && strings.HasSuffix(path, ".body") {
			extracted := strings.TrimSuffix(path, ".body") + ".txt"
			if _, statErr := os.Stat(filepath.Join(m.Directory, extracted)); statErr == nil {
				path = extracted
			}
		}
		evidence, artifactErr := getArtifact(m.Directory, path)
		if artifactErr != nil {
			return artifactErr
		}
		s.Fetch = &fetch{Row: row, ComparisonSHA256: row[5], Evidence: evidence}
		for _, name := range []string{row[9], strings.TrimSuffix(row[9], ".body") + ".entries.md"} {
			if name == "" || name == path {
				continue
			}
			if _, statErr := os.Stat(filepath.Join(m.Directory, name)); errors.Is(statErr, os.ErrNotExist) {
				continue
			}
			entry, entryErr := getArtifact(m.Directory, name)
			if entryErr != nil {
				return entryErr
			}
			s.Fetch.Artifacts = append(s.Fetch.Artifacts, entry)
		}
	}
	m.RowsSHA256 = digest(b)
	m.BuiltinDeep, err = a.builtinDeep(m)
	if err != nil {
		return err
	}
	return save(m)
}

func (a app) builtinDeep(m *manifest) ([]string, error) {
	args := []string{"scripts/target-facts.sh", "--changed", filepath.Join(m.Directory, "docfetch.tsv")}
	if m.BuiltinsSince != "" {
		args = append(args, "--builtins-since", m.BuiltinsSince)
	}
	text, err := a.command("bash", args...)
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "builtin-deep:") {
			return strings.Fields(strings.TrimPrefix(line, "builtin-deep:")), nil
		}
	}
	return nil, nil
}

func readOutcome(m *manifest, args []string) (*outcome, error) {
	if len(args) != 3 || !slices.Contains([]string{"reviewed", "fast-path", "research-limit"}, args[0]) || strings.TrimSpace(args[2]) == "" {
		return nil, errors.New("outcome requires reviewed|fast-path|research-limit, artifact path and reason")
	}
	if args[0] == "fast-path" && m.BuiltinsSince == "" {
		return nil, errors.New("fast path requires a built-in baseline")
	}
	a, err := getArtifact(m.Directory, args[1])
	if err != nil {
		return nil, err
	}
	return &outcome{Disposition: args[0], Reason: args[2], Artifact: a}, nil
}

func validateOutcome(m *manifest, out *outcome) error {
	if out == nil {
		return errors.New("missing disposition")
	}
	if !slices.Contains([]string{"reviewed", "fast-path", "research-limit"}, out.Disposition) || strings.TrimSpace(out.Reason) == "" {
		return errors.New("unknown or unjustified disposition")
	}
	if out.Disposition == "fast-path" && m.BuiltinsSince == "" {
		return errors.New("fast path requires a built-in baseline")
	}
	return checkArtifact(m.Directory, out.Artifact)
}

func (a app) validateEvidence(m *manifest, expectedLock string) error {
	if err := a.inputsAtLock(m, expectedLock); err != nil {
		return err
	}
	if m.RowsSHA256 == "" {
		return errors.New("fetch rows have not been captured")
	}
	hash, err := fileDigest(filepath.Join(m.Directory, "docfetch.tsv"))
	if err != nil {
		return err
	}
	if hash != m.RowsSHA256 {
		return errors.New("fetched rows changed after capture")
	}
	deep, err := a.builtinDeep(m)
	if err != nil {
		return err
	}
	if !slices.Equal(deep, m.BuiltinDeep) {
		return errors.New("built-in baseline classification changed")
	}
	rowBytes, err := os.ReadFile(filepath.Join(m.Directory, "docfetch.tsv"))
	if err != nil {
		return fmt.Errorf("docfetch.tsv: %w", err)
	}
	fetchedRows := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(rowBytes), "\n"), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 10 {
			return errors.New("malformed captured fetch row")
		}
		key := strings.Join(fields[:3], "\t")
		if fetchedRows[key] != "" {
			return errors.New("duplicate captured fetch row")
		}
		fetchedRows[key] = line
	}
	if len(fetchedRows) != len(m.Sources) {
		return errors.New("captured fetch coverage changed")
	}
	for _, s := range m.Sources {
		if s.Fetch == nil || len(s.Fetch.Row) != 10 || strings.Join(s.Fetch.Row[:3], "\t") != s.key() {
			return fmt.Errorf("%s: missing or invalid fetch", s.key())
		}
		if strings.Join(s.Fetch.Row, "\t") != fetchedRows[s.key()] || s.Fetch.ComparisonSHA256 != s.Fetch.Row[5] {
			return errors.New("captured comparison rows changed")
		}
		if err = checkArtifact(m.Directory, s.Fetch.Evidence); err != nil {
			return err
		}
		for _, entry := range s.Fetch.Artifacts {
			if err = checkArtifact(m.Directory, entry); err != nil {
				return err
			}
		}
		if err = comparisonValid(m.Directory, s.Fetch.Row); err != nil {
			return err
		}
	}
	for _, entry := range m.Reused {
		if err := checkArtifact(m.Directory, entry.Manifest); err != nil {
			return err
		}
	}
	return nil
}

func (a app) validate(m *manifest) error {
	return a.validateAtLock(m, m.LockBaseline)
}

func (a app) validateAtLock(m *manifest, expectedLock string) error {
	if err := a.validateEvidence(m, expectedLock); err != nil {
		return err
	}
	var err error
	seen := map[string]bool{}
	for _, as := range m.Assignments {
		if !slices.Contains(m.Targets, as.Target) || seen[as.Target] || strings.TrimSpace(as.Owner) == "" {
			return fmt.Errorf("%s: unknown, duplicate or unowned assignment", as.Target)
		}
		seen[as.Target] = true
		if err = validateOutcome(m, as.Outcome); err != nil {
			return fmt.Errorf("assignment %s: %w", as.Target, err)
		}
	}
	if len(seen) != len(m.Targets) {
		return errors.New("every requested target needs exactly one completed assignment")
	}

	deep := m.BuiltinDeep
	for _, as := range m.Assignments {
		if slices.Contains(deep, as.Target) && as.Outcome.Disposition == "fast-path" {
			return fmt.Errorf("%s: changed built-ins require deep review", as.Target)
		}
	}

	for _, s := range m.Sources {
		if err = validateOutcome(m, s.Outcome); err != nil {
			return fmt.Errorf("source %s: %w", s.key(), err)
		}
		if s.Fetch.Row[7] == "failed" && s.Outcome.Disposition != "research-limit" {
			return fmt.Errorf("%s: failed fetch requires research-limit", s.key())
		}
		if slices.Contains(deep, s.Target) && s.Outcome.Disposition == "fast-path" {
			return fmt.Errorf("%s: changed built-ins require deep review", s.key())
		}
	}
	return nil
}

func (a app) finalize(m *manifest) (string, error) {
	guard := a.lock + ".audit-finalizing"
	if err := os.MkdirAll(filepath.Dir(a.lock), 0o755); err != nil {
		return "", fmt.Errorf("%s: %w", a.lock, err)
	}
	f, err := os.OpenFile(guard, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("%s: finalization is busy or was interrupted: %w", guard, err)
	}
	defer func() { _ = os.Remove(guard) }()
	_, writeErr := fmt.Fprintf(f, "pid=%d\nrun=%s\n", os.Getpid(), m.Directory)
	closeErr := f.Close()
	if writeErr != nil {
		return "", fmt.Errorf("%s: %w", guard, writeErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("%s: %w", guard, closeErr)
	}
	if out, handled, err := a.reconcileFinalization(m); handled {
		return out, err
	}
	if err := a.validate(m); err != nil {
		return "", err
	}
	old := map[string]string{}
	b, err := os.ReadFile(a.lock)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%s: %w", a.lock, err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 7 || old[f[2]] != "" {
			return "", fmt.Errorf("%s: malformed or duplicate lock row", a.lock)
		}
		old[f[2]] = line
	}
	owners := map[string][]source{}
	for _, s := range m.Inventory {
		owners[s.URL] = append(owners[s.URL], s)
	}
	covered := map[string]source{}
	for _, s := range m.Sources {
		covered[s.key()] = s
	}
	withheld, advanced := 0, 0
	for url, group := range owners {
		var row []string
		eligible := true
		for _, owner := range group {
			s, ok := covered[owner.key()]
			if !ok || s.Outcome.Disposition == "research-limit" || s.Fetch.Row[7] == "failed" {
				eligible = false
				continue
			}
			if row != nil && row[5] != s.Fetch.Row[5] {
				return "", fmt.Errorf("%s: shared URL owners disagree on comparison hash", url)
			}
			if row == nil {
				row = s.Fetch.Row
			}
		}
		if !eligible {
			withheld++
			continue
		}
		old[url] = strings.Join(row[:7], "\t")
		advanced++
	}
	var rows []string
	for url, line := range old {
		if _, ok := owners[url]; ok {
			rows = append(rows, line)
		}
	}
	sort.Strings(rows)
	result := "# target-audit sources lock. Written by scripts/docfetch.sh --update. Do not hand-edit rows.\n# target\tkind\turl\thttp\tmode\tsha256\tchanged\n" + strings.Join(rows, "\n")
	if len(rows) > 0 {
		result += "\n"
	}
	if err = os.MkdirAll(filepath.Dir(a.lock), 0o755); err != nil {
		return "", fmt.Errorf("%s: %w", a.lock, err)
	}
	if err = writeFinalizationReceipt(m, digest([]byte(result))); err != nil {
		return "", err
	}
	if err = atomicWrite(a.lock, []byte(result)); err != nil {
		return "", err
	}
	m.Finalized = digest([]byte(result))
	if err = save(m); err != nil {
		return "", fmt.Errorf("%s: lock committed; manifest finalization marker failed (committed sha256 %s); preserve the run and see scripts/target-audit/manifest.md: %w", a.lock, m.Finalized, err)
	}
	return fmt.Sprintf("coverage validated: %d targets, %d sources; %d eligible URLs, %d retained URLs\n", len(m.Targets), len(m.Sources), advanced, withheld), nil
}

func (a app) run(args []string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("usage: start|capture|resume|assign|complete|record|validate|finalize (see scripts/target-audit/manifest.md)")
	}
	if args[0] == "start" {
		return a.start(args[1:])
	}
	if len(args) < 2 {
		return "", fmt.Errorf("%s requires a run directory", args[0])
	}
	m, err := load(args[1])
	if err != nil {
		return "", err
	}
	guard := filepath.Join(m.Directory, ".audit-writing")
	if err = os.Mkdir(guard, 0o700); err != nil {
		return "", fmt.Errorf("%s: run state is busy or was interrupted: %w", guard, err)
	}
	defer func() { _ = os.Remove(guard) }()
	m, err = load(args[1])
	if err != nil {
		return "", err
	}
	if m.Finalized != "" && args[0] != "finalize" {
		return "", errors.New("run already finalized")
	}
	switch args[0] {
	case "resume":
		if len(args) != 3 {
			return "", errors.New("resume requires fresh captured run and explicit prior run directory")
		}
		return a.resume(m, args[2])
	case "capture":
		if len(args) != 2 {
			return "", errors.New("capture requires only a run directory")
		}
		return "", a.capture(m)
	case "assign":
		if len(args) != 4 || !slices.Contains(m.Targets, args[2]) || strings.TrimSpace(args[3]) == "" {
			return "", errors.New("assign requires run, requested target and owner")
		}
		for _, as := range m.Assignments {
			if as.Target == args[2] {
				return "", fmt.Errorf("%s: already assigned", args[2])
			}
		}
		m.Assignments = append(m.Assignments, assignment{Target: args[2], Owner: args[3]})
	case "complete":
		if len(args) != 6 {
			return "", errors.New("complete requires run, target, disposition, artifact and reason")
		}
		out, outErr := readOutcome(m, args[3:])
		if outErr != nil {
			return "", outErr
		}
		found := false
		for i := range m.Assignments {
			if m.Assignments[i].Target == args[2] {
				if m.Assignments[i].Outcome != nil {
					return "", errors.New("assignment already completed")
				}
				m.Assignments[i].Outcome = out
				found = true
			}
		}
		if !found {
			return "", fmt.Errorf("%s: no assignment", args[2])
		}
	case "record":
		if len(args) != 8 {
			return "", errors.New("record requires run, target, kind, URL, disposition, artifact and reason")
		}
		out, outErr := readOutcome(m, args[5:])
		if outErr != nil {
			return "", outErr
		}
		key := strings.Join(args[2:5], "\t")
		found := false
		for i := range m.Sources {
			if m.Sources[i].key() == key {
				if m.Sources[i].Fetch == nil || m.Sources[i].Outcome != nil {
					return "", errors.New("source must be captured and not yet recorded")
				}
				m.Sources[i].Outcome = out
				found = true
			}
		}
		if !found {
			return "", fmt.Errorf("%s: unknown source", key)
		}
	case "validate", "finalize":
		if len(args) > 2 && !slices.Equal(args[2:], m.Targets) {
			return "", errors.New("finalization targets differ from fixed requested targets")
		}
		if args[0] == "finalize" {
			return a.finalize(m)
		}
		return "coverage valid\n", a.validate(m)
	default:
		return "", fmt.Errorf("unknown command %q", args[0])
	}
	return "", save(m)
}
