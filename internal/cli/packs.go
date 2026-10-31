package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	// packsDir is the on-disk root for all installed packs, relative to
	// the project root.
	packsDir = ".agnostic-ai/packs"
	// packsLockfile pins each installed pack to a specific source and
	// revision so layered loads stay reproducible across machines.
	packsLockfile = "agnostic.packs.lock"

	layerNamePackPrefix = "pack:"
)

// packEntry is one row of agnostic.packs.lock. Source is the user-given
// argument verbatim (Git URL or local path); Ref is the resolved branch,
// tag, or commit; Sha is the commit hash captured at install time, which
// `update` restores exactly.
type packEntry struct {
	Name   string `yaml:"name"`
	Source string `yaml:"source"`
	Ref    string `yaml:"ref,omitempty"`
	Sha    string `yaml:"sha,omitempty"`
}

// packsLock is the on-disk format of agnostic.packs.lock.
type packsLock struct {
	Version int         `yaml:"version"`
	Packs   []packEntry `yaml:"packs"`
}

func newPacksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "packs",
		Short: "Manage shareable spec packs.",
		Long: "Spec packs are versioned directories of agnostic specs " +
			"published as Git repos or local directories. Installed packs " +
			"are loaded as a layer below the project layer, so the project " +
			"can override any pack-supplied entry by name.",
		Example: `  # Install a pack at a tag
  agnostic-ai packs add github.com/obra/superpowers@v6.4.2

  # Install from a local directory
  agnostic-ai packs add ./path/to/pack

  # List installed packs
  agnostic-ai packs list

  # Reinstall one or all packs at the commits in agnostic.packs.lock
  agnostic-ai packs update
  agnostic-ai packs update superpowers

  # Move packs to the newest commit of their recorded tag or branch
  agnostic-ai packs update --latest

  # Remove a pack
  agnostic-ai packs remove superpowers`,
	}
	cmd.AddCommand(newPacksAddCmd(), newPacksRemoveCmd(), newPacksListCmd(), newPacksUpdateCmd())
	return cmd
}

func newPacksAddCmd() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "add <source>",
		Short: "Install a spec pack from a Git URL or local directory.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := refuseGlobalHome(".", globalHomeSpecsRemedy); err != nil {
				return err
			}
			return runPacksAdd(".", args[0], name, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Override the install directory name (default: derived from source)")
	return cmd
}

func newPacksRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Uninstall a spec pack.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := refuseGlobalHome(".", globalHomeSpecsRemedy); err != nil {
				return err
			}
			return runPacksRemove(".", args[0], cmd.OutOrStdout())
		},
	}
}

func newPacksListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed spec packs.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPacksList(".", cmd.OutOrStdout())
		},
	}
}

func newPacksUpdateCmd() *cobra.Command {
	var latest bool
	cmd := &cobra.Command{
		Use:   "update [<name>]",
		Short: "Reinstall pack(s) at the commit recorded in the lockfile.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := refuseGlobalHome(".", globalHomeSpecsRemedy); err != nil {
				return err
			}
			only := ""
			if len(args) == 1 {
				only = args[0]
			}
			return runPacksUpdate(".", only, latest, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&latest, "latest", false, "Move each pack to the newest commit of its recorded tag or branch and record it; it does not switch to a newer tag")
	return cmd
}

// runPacksAdd installs one pack and updates the lockfile. Re-installing
// an existing pack overwrites its directory and lockfile entry so an
// `add` after a manual edit restores a clean state.
func runPacksAdd(root, source, nameOverride string, out io.Writer) error {
	name := nameOverride
	src, ref := splitSourceRef(source)
	if name == "" {
		name = derivePackName(src)
	}
	if name == "" {
		return fmt.Errorf("packs add: cannot derive a pack name from %q; pass --name", source)
	}
	if err := validatePackName(name); err != nil {
		return err
	}
	if !isLocalSource(src) {
		if err := checkLockedSource(root, src); err != nil {
			return fmt.Errorf("packs add: %w", err)
		}
	}
	if err := checkPacksDirInsideProject(root); err != nil {
		return fmt.Errorf("packs add %q: %w", name, err)
	}

	lock, err := readPacksLock(root)
	if err != nil {
		return err
	}
	dest := filepath.Join(root, packsDir, name)
	err = installPack(dest, func(staged string) (string, error) {
		return fetchPack(src, ref, staged)
	}, func(sha string) error {
		lock.upsert(packEntry{Name: name, Source: src, Ref: ref, Sha: sha})
		return writePacksLock(root, lock)
	})
	if err != nil {
		return fmt.Errorf("packs add: %w", err)
	}
	if err := ensureManagedGitignore(root); err != nil {
		return fmt.Errorf("packs add: gitignore: %w", err)
	}
	_, _ = fmt.Fprintf(out, "✓ added %s\n", name)
	return nil
}

func runPacksRemove(root, name string, out io.Writer) error {
	lock, err := readPacksLock(root)
	if err != nil {
		return err
	}
	if !lock.has(name) {
		return fmt.Errorf("packs remove: %q not installed", name)
	}
	if err := validatePackName(name); err != nil {
		return fmt.Errorf("packs remove %q: %w", name, err)
	}
	if err := checkPacksDirInsideProject(root); err != nil {
		return fmt.Errorf("packs remove %q: %w", name, err)
	}
	dest := filepath.Join(root, packsDir, name)
	if err := os.RemoveAll(dest); err != nil {
		return fmt.Errorf("packs remove: %w", err)
	}
	lock.remove(name)
	if err := writePacksLock(root, lock); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "✓ removed %s\n", name)
	return nil
}

func runPacksList(root string, out io.Writer) error {
	lock, err := readPacksLock(root)
	if err != nil {
		return err
	}
	if len(lock.Packs) == 0 {
		_, _ = fmt.Fprintln(out, "no packs installed.")
		return nil
	}
	for _, p := range lock.Packs {
		ref := p.Ref
		if ref == "" {
			ref = "HEAD"
		}
		_, _ = fmt.Fprintf(out, "%s\t%s@%s", p.Name, p.Source, ref)
		if p.Sha != "" {
			_, _ = fmt.Fprintf(out, " (%s)", short(p.Sha))
		}
		_, _ = fmt.Fprintln(out)
	}
	return nil
}

func runPacksUpdate(root, only string, latest bool, out io.Writer) error {
	lock, err := readPacksLock(root)
	if err != nil {
		return err
	}
	if len(lock.Packs) == 0 {
		_, _ = fmt.Fprintln(out, "no packs installed.")
		return nil
	}
	skipped := map[string]bool{}
	for _, p := range lock.Packs {
		if only != "" && p.Name != only {
			continue
		}
		err := checkLockedEntry(root, p)
		if only == "" && errors.Is(err, errLocalSourceOutsideProject) {
			_, _ = fmt.Fprintf(out, "! skipped %s: %v\n", p.Name, err)
			skipped[p.Name] = true
			continue
		}
		if err != nil {
			return fmt.Errorf("packs update %q: %w", p.Name, err)
		}
	}
	updated := 0
	for _, p := range slices.Clone(lock.Packs) {
		if (only != "" && p.Name != only) || skipped[p.Name] {
			continue
		}
		dest := filepath.Join(root, packsDir, p.Name)
		var sha string
		err := installPack(dest, func(staged string) (string, error) {
			return fetchLockedPack(p, latest, staged)
		}, func(fetched string) error {
			sha = fetched
			updated := p
			updated.Sha = fetched
			lock.upsert(updated)
			return writePacksLock(root, lock)
		})
		if err != nil {
			return fmt.Errorf("packs update %s: %w", p.Name, err)
		}
		reportPackUpdate(out, p, sha)
		updated++
	}
	if only != "" && updated == 0 {
		return fmt.Errorf("packs update: %q not installed", only)
	}
	return nil
}

func installPack(dest string, fetch func(string) (string, error), record func(string) error) error {
	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", parent, err)
	}
	tmp, err := os.MkdirTemp(parent, ".pack-*")
	if err != nil {
		return fmt.Errorf("stage %s: %w", dest, err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmp)
		}
	}()
	staged, backup := filepath.Join(tmp, "new"), filepath.Join(tmp, "previous")
	sha, err := fetch(staged)
	if err != nil {
		return err
	}
	hadPrevious := false
	if err := os.Rename(dest, backup); err == nil {
		hadPrevious = true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("save %s: %w", dest, err)
	}
	restore := func() error {
		if err := os.RemoveAll(dest); err != nil {
			cleanup = false
			return fmt.Errorf("remove %s: %w; previous pack saved at %s", dest, err, backup)
		}
		if hadPrevious {
			if err := os.Rename(backup, dest); err != nil {
				cleanup = false
				return fmt.Errorf("restore %s from %s: %w", dest, backup, err)
			}
		}
		return nil
	}
	if err := os.Rename(staged, dest); err != nil {
		return errors.Join(fmt.Errorf("install %s: %w", dest, err), restore())
	}
	if err := record(sha); err != nil {
		return errors.Join(err, restore())
	}
	return nil
}

// checkLockedEntry refuses a lockfile entry that update does not trust,
// because a cloned project's lockfile is not trusted input.
func checkLockedEntry(root string, p packEntry) error {
	if err := validatePackName(p.Name); err != nil {
		return lockedNameError(err)
	}
	if err := checkPacksDirInsideProject(root); err != nil {
		return err
	}
	if strings.HasPrefix(p.Ref, "-") {
		return fmt.Errorf("ref %q must not start with -", p.Ref)
	}
	err := checkLockedSource(root, p.Source)
	if errors.Is(err, errLocalSourceOutsideProject) {
		return fmt.Errorf("%w; refresh it with `agnostic-ai packs add %s --name %s`", err, shellQuote(p.Source), shellQuote(p.Name))
	}
	return err
}

func checkPacksDirInsideProject(root string) error {
	realRoot, err := realAbs(root)
	if err != nil {
		return err
	}
	for _, dir := range []string{packsDir, filepath.Dir(packsDir)} {
		path := filepath.Join(root, dir)
		real, err := realAbs(path)
		if errors.Is(err, fs.ErrNotExist) {
			if _, lerr := os.Lstat(path); lerr == nil {
				return fmt.Errorf("%s is a broken link; remove it and try again", dir)
			}
			continue
		}
		if err != nil {
			return err
		}
		if !pathInside(realRoot, real) {
			return fmt.Errorf("%s links outside the project; remove the link and try again", dir)
		}
		return nil
	}
	return nil
}

func checkLockedSource(root, source string) error {
	if strings.HasPrefix(source, "-") {
		return fmt.Errorf("source %q must not start with -", source)
	}
	if isLocalSource(source) {
		return checkLocalSource(root, source)
	}
	switch {
	case strings.HasPrefix(source, "https://"), strings.HasPrefix(source, "ssh://"):
		return nil
	case strings.HasPrefix(source, "git@"):
		if !isSCPLikeSource(source) {
			return fmt.Errorf("source %q must have the form git@host:path", source)
		}
		return nil
	case strings.Contains(source, "://") || strings.Contains(source, "::"):
		return fmt.Errorf("source %q: only https://, ssh://, git@host:path, and local folders are allowed", source)
	}
	return nil
}

func isSCPLikeSource(source string) bool {
	host, path, ok := strings.Cut(strings.TrimPrefix(source, "git@"), ":")
	return ok && host != "" && path != "" && !strings.ContainsAny(host, `/\`)
}

var errLocalSourceOutsideProject = errors.New("local source is outside the project")

// checkLocalSource compares volumes before resolving links so a Windows
// network path is refused without being opened.
func checkLocalSource(root, source string) error {
	path := localPath(source)
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.VolumeName(absRoot), filepath.VolumeName(absPath)) {
		return errLocalSourceOutsideProject
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return err
	}
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return fmt.Errorf("local source %q: %w", source, err)
	}
	if !pathInside(realRoot, realPath) {
		return errLocalSourceOutsideProject
	}
	// Lowercase so a case-insensitive disk cannot spell the packs folder differently.
	packs := strings.ToLower(filepath.Join(realRoot, packsDir))
	lowerPath := strings.ToLower(realPath)
	if pathInside(lowerPath, packs) || pathInside(packs, lowerPath) {
		return fmt.Errorf("local source %q must not hold or sit inside %s", source, packsDir)
	}
	return nil
}

func realAbs(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

func pathInside(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// fetchLockedPack restores a Git pack at its recorded commit, so a moved
// tag or a force-pushed branch cannot change what the project gets. With
// latest, or when no commit is recorded yet, it fetches the ref's tip.
func fetchLockedPack(p packEntry, latest bool, dest string) (string, error) {
	if latest || p.Sha == "" || isLocalSource(p.Source) {
		return fetchPack(p.Source, p.Ref, dest)
	}
	sha, err := gitFetchCommit(p.Source, p.Sha, dest)
	if err == nil {
		return sha, nil
	}
	// A server on the old wire protocol refuses a fetch by commit unless
	// that commit is a branch or tag tip, so try the ref and keep it only
	// when it still points at the recorded commit.
	if isCommitHash(p.Sha) && os.RemoveAll(dest) == nil {
		if tip, cloneErr := gitClone(p.Source, p.Ref, dest); cloneErr == nil && strings.EqualFold(tip, p.Sha) {
			return p.Sha, nil
		}
	}
	_ = os.RemoveAll(dest)
	return "", fmt.Errorf("restore recorded commit %s: %w; run `agnostic-ai packs update --latest %s` to move to the newest commit of its ref", short(p.Sha), err, p.Name)
}

func reportPackUpdate(out io.Writer, p packEntry, sha string) {
	switch {
	case p.Sha != "" && sha != "" && p.Sha != sha:
		_, _ = fmt.Fprintf(out, "✓ updated %s from %s to %s\n", p.Name, short(p.Sha), short(sha))
	case sha != "":
		_, _ = fmt.Fprintf(out, "✓ updated %s at %s\n", p.Name, short(sha))
	default:
		_, _ = fmt.Fprintf(out, "✓ updated %s\n", p.Name)
	}
}

// fetchPack populates dest with the pack contents and returns the
// resolved commit sha when the source is a Git URL. Local-directory
// sources (file://, ./, ../, /) copy the tree and return an empty sha.
func fetchPack(source, ref, dest string) (string, error) {
	if isLocalSource(source) {
		return "", copyTree(localPath(source), dest)
	}
	return gitClone(source, ref, dest)
}

func isLocalSource(s string) bool {
	if strings.HasPrefix(s, "file://") {
		return true
	}
	if strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") {
		return true
	}
	if filepath.IsAbs(s) {
		return true
	}
	return false
}

func localPath(s string) string {
	return strings.TrimPrefix(s, "file://")
}

func copyTree(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s: not a directory", src)
	}
	return filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// gitClone clones source at ref into dest using the system git binary
// and returns the resolved HEAD commit. When ref is empty the default
// branch is fetched.
func gitClone(source, ref, dest string) (string, error) {
	if strings.HasPrefix(source, "-") {
		return "", fmt.Errorf("source %q must not start with -", source)
	}
	if strings.HasPrefix(ref, "-") {
		return "", fmt.Errorf("ref %q must not start with -", ref)
	}
	cloneURL := gitSourceURL(source)
	if err := runCmd("git", gitCloneArgs(cloneURL, ref, dest)...); err != nil {
		return "", fmt.Errorf("git clone: %w", err)
	}
	sha, err := captureCmd(dest, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(dest, ".git")); err != nil {
		return "", fmt.Errorf("strip .git: %w", err)
	}
	return strings.TrimSpace(sha), nil
}

func gitCloneArgs(source, ref, dest string) []string {
	args := []string{"-c", "protocol.ext.allow=never", "clone", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	return append(args, "--", source, dest)
}

func gitSourceURL(source string) string {
	if !strings.Contains(source, "://") && !strings.HasPrefix(source, "git@") {
		return "https://" + source
	}
	return source
}

// gitFetchCommit fetches exactly one commit into dest and fails when the
// remote no longer has it.
func gitFetchCommit(source, sha, dest string) (string, error) {
	if !isCommitHash(sha) {
		return "", fmt.Errorf("%q is not a commit hash", sha)
	}
	steps := []struct {
		name string
		args []string
	}{
		{"init", initArgs(sha, dest)},
		{"fetch", []string{"-C", dest, "-c", "protocol.ext.allow=never", "fetch", "--quiet", "--depth", "1", "--", gitSourceURL(source), sha}},
		{"checkout", []string{"-C", dest, "-c", "advice.detachedHead=false", "checkout", "--quiet", "FETCH_HEAD"}},
	}
	for _, step := range steps {
		if err := runCmd("git", step.args...); err != nil {
			return "", fmt.Errorf("git %s: %w", step.name, err)
		}
	}
	got, err := captureCmd(dest, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	if got = strings.TrimSpace(got); !strings.EqualFold(got, sha) {
		return "", fmt.Errorf("fetched commit %s, want %s", short(got), short(sha))
	}
	if err := os.RemoveAll(filepath.Join(dest, ".git")); err != nil {
		return "", fmt.Errorf("strip .git: %w", err)
	}
	return sha, nil
}

func initArgs(sha, dest string) []string {
	if len(sha) == 64 {
		return []string{"init", "--quiet", "--object-format=sha256", dest}
	}
	return []string{"init", "--quiet", dest}
}

func isCommitHash(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func captureCmd(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

// splitSourceRef splits "github.com/foo/bar@v1.2.0" into
// ("github.com/foo/bar", "v1.2.0"). A bare source returns ref "".
// The split honors only the last "@" so SSH-style "git@host:path@ref"
// still parses correctly.
func splitSourceRef(s string) (src, ref string) {
	at := strings.LastIndex(s, "@")
	if at < 0 {
		return s, ""
	}
	return s[:at], s[at+1:]
}

// derivePackName picks an install directory name from a source string.
// Strategy: take the last path component, drop a `.git` suffix. Path
// separators are normalized so an absolute Windows path like
// `C:\Users\me\pack` still produces `pack`.
func derivePackName(source string) string {
	s := strings.TrimPrefix(source, "file://")
	s = filepath.ToSlash(s)
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimSuffix(s, ".git")
	if u, err := url.Parse(s); err == nil && u.Path != "" && u.Host != "" {
		s = strings.TrimPrefix(u.Path, "/")
	}
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.LastIndex(s, ":"); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// validatePackName rejects names that would escape the packs directory
// or collide with disallowed characters. A pack name is a single path
// segment.
func validatePackName(name string) error {
	if name == "" {
		return fmt.Errorf("packs: invalid pack name %q", name)
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("packs: pack name must not start with a dot, got %q", name)
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("packs: pack name must be a single path segment, got %q", name)
	}
	return nil
}

func short(sha string) string {
	if len(sha) < 8 {
		return sha
	}
	return sha[:8]
}

// readPacksLock loads agnostic.packs.lock, returning an empty lock if
// the file does not exist.
func readPacksLock(root string) (*packsLock, error) {
	path := filepath.Join(root, packsLockfile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &packsLock{Version: 1}, nil
		}
		return nil, fmt.Errorf("read %s: %w", packsLockfile, err)
	}
	var lock packsLock
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("parse %s: %w", packsLockfile, err)
	}
	if lock.Version == 0 {
		lock.Version = 1
	}
	for _, p := range lock.Packs {
		if err := validatePackName(p.Name); err != nil {
			return nil, fmt.Errorf("%s: %w", packsLockfile, lockedNameError(err))
		}
	}
	return &lock, nil
}

// lockedNameError points at the one command that can replace a lockfile
// entry whose name update and sync refuse.
func lockedNameError(err error) error {
	return fmt.Errorf("%w; reinstall it with `agnostic-ai packs add <source> --name <name>`", err)
}

// writePacksLock atomically rewrites agnostic.packs.lock. When the
// resulting list is empty the file is removed so adding then removing a
// pack leaves no residue.
func writePacksLock(root string, lock *packsLock) error {
	path := filepath.Join(root, packsLockfile)
	if len(lock.Packs) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", packsLockfile, err)
		}
		return nil
	}
	sort.Slice(lock.Packs, func(i, j int) bool { return lock.Packs[i].Name < lock.Packs[j].Name })
	data, err := yaml.Marshal(lock)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", packsLockfile, err)
	}
	snapshot, err := snapshotEditFile(path)
	if err != nil {
		return err
	}
	if snapshot.info == nil {
		_, err = snapshot.replace(path, data, 0o644)
	} else {
		_, err = snapshot.replace(path, data)
	}
	return err
}

func (l *packsLock) has(name string) bool {
	for _, p := range l.Packs {
		if p.Name == name {
			return true
		}
	}
	return false
}

func (l *packsLock) upsert(p packEntry) {
	for i, ex := range l.Packs {
		if ex.Name == p.Name {
			l.Packs[i] = p
			return
		}
	}
	l.Packs = append(l.Packs, p)
}

func (l *packsLock) remove(name string) {
	out := l.Packs[:0]
	for _, p := range l.Packs {
		if p.Name == name {
			continue
		}
		out = append(out, p)
	}
	l.Packs = out
}

// resolvePacksLayers returns one spec.Layer per installed pack, in
// alphabetical order so emit output stays deterministic across runs.
// Each pack is named "pack:<name>" so list/validate output is
// self-explanatory. Returns nil when no packs are installed.
func resolvePacksLayers(projectRoot string) []spec.Layer {
	lock, err := readPacksLock(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "! %v\n", err)
		return nil
	}
	if len(lock.Packs) == 0 {
		return nil
	}
	out := make([]spec.Layer, 0, len(lock.Packs))
	for _, p := range lock.Packs {
		if err := validatePackName(p.Name); err != nil {
			fmt.Fprintf(os.Stderr, "! %v in %s; skipped\n", err, packsLockfile)
			continue
		}
		root := filepath.Join(projectRoot, packsDir, p.Name)
		if !dirExists(root) {
			fmt.Fprintf(os.Stderr, "! pack %q listed in %s but missing on disk; run `agnostic-ai packs update`\n", p.Name, packsLockfile)
			continue
		}
		out = append(out, spec.Layer{
			Name:     layerNamePackPrefix + p.Name,
			Root:     root,
			Sources:  defaultLayerSources(),
			RuleRoot: projectRoot,
		})
	}
	return out
}
