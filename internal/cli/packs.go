package cli

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
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
// tag, or commit; Sha is the commit hash captured at install time and
// used by `update` to detect changes.
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
  agnostic-ai packs add github.com/chemaclass/go-rules@v1.2.0

  # Install from a local directory
  agnostic-ai packs add ./path/to/pack

  # List installed packs
  agnostic-ai packs list

  # Update one or all packs
  agnostic-ai packs update
  agnostic-ai packs update go-rules

  # Remove a pack
  agnostic-ai packs remove go-rules`,
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
	return &cobra.Command{
		Use:   "update [<name>]",
		Short: "Reinstall pack(s) at the lockfile-recorded ref.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			only := ""
			if len(args) == 1 {
				only = args[0]
			}
			return runPacksUpdate(".", only, cmd.OutOrStdout())
		},
	}
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

	dest := filepath.Join(root, packsDir, name)
	if err := os.RemoveAll(dest); err != nil {
		return fmt.Errorf("packs add: clean dest: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("packs add: mkdir: %w", err)
	}

	sha, err := fetchPack(src, ref, dest)
	if err != nil {
		return fmt.Errorf("packs add: %w", err)
	}

	lock, err := readPacksLock(root)
	if err != nil {
		return err
	}
	lock.upsert(packEntry{Name: name, Source: src, Ref: ref, Sha: sha})
	if err := writePacksLock(root, lock); err != nil {
		return err
	}
	fmt.Fprintf(out, "✓ added %s\n", name)
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
	dest := filepath.Join(root, packsDir, name)
	if err := os.RemoveAll(dest); err != nil {
		return fmt.Errorf("packs remove: %w", err)
	}
	lock.remove(name)
	if err := writePacksLock(root, lock); err != nil {
		return err
	}
	fmt.Fprintf(out, "✓ removed %s\n", name)
	return nil
}

func runPacksList(root string, out io.Writer) error {
	lock, err := readPacksLock(root)
	if err != nil {
		return err
	}
	if len(lock.Packs) == 0 {
		fmt.Fprintln(out, "no packs installed.")
		return nil
	}
	for _, p := range lock.Packs {
		ref := p.Ref
		if ref == "" {
			ref = "HEAD"
		}
		fmt.Fprintf(out, "%s\t%s@%s", p.Name, p.Source, ref)
		if p.Sha != "" {
			fmt.Fprintf(out, " (%s)", short(p.Sha))
		}
		fmt.Fprintln(out)
	}
	return nil
}

func runPacksUpdate(root, only string, out io.Writer) error {
	lock, err := readPacksLock(root)
	if err != nil {
		return err
	}
	if len(lock.Packs) == 0 {
		fmt.Fprintln(out, "no packs installed.")
		return nil
	}
	updated := 0
	for i, p := range lock.Packs {
		if only != "" && p.Name != only {
			continue
		}
		dest := filepath.Join(root, packsDir, p.Name)
		if err := os.RemoveAll(dest); err != nil {
			return fmt.Errorf("packs update %s: %w", p.Name, err)
		}
		sha, err := fetchPack(p.Source, p.Ref, dest)
		if err != nil {
			return fmt.Errorf("packs update %s: %w", p.Name, err)
		}
		lock.Packs[i].Sha = sha
		fmt.Fprintf(out, "✓ updated %s\n", p.Name)
		updated++
	}
	if only != "" && updated == 0 {
		return fmt.Errorf("packs update: %q not installed", only)
	}
	return writePacksLock(root, lock)
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
	if strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") || strings.HasPrefix(s, "/") {
		return true
	}
	return false
}

func localPath(s string) string {
	if strings.HasPrefix(s, "file://") {
		return strings.TrimPrefix(s, "file://")
	}
	return s
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
	cloneURL := source
	if !strings.Contains(cloneURL, "://") && !strings.HasPrefix(cloneURL, "git@") {
		cloneURL = "https://" + cloneURL
	}
	args := []string{"clone", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, cloneURL, dest)
	if err := runCmd("git", args...); err != nil {
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
// Strategy: take the last path component, drop a `.git` suffix.
func derivePackName(source string) string {
	s := source
	if strings.HasPrefix(s, "file://") {
		s = strings.TrimPrefix(s, "file://")
	}
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
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("packs: invalid pack name %q", name)
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
	return &lock, nil
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
	return os.WriteFile(path, data, 0o644)
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
		root := filepath.Join(projectRoot, packsDir, p.Name)
		if !dirExists(root) {
			fmt.Fprintf(os.Stderr, "! pack %q listed in %s but missing on disk; run `agnostic-ai packs update`\n", p.Name, packsLockfile)
			continue
		}
		out = append(out, spec.Layer{
			Name:    layerNamePackPrefix + p.Name,
			Root:    root,
			Sources: defaultLayerSources(),
		})
	}
	return out
}
