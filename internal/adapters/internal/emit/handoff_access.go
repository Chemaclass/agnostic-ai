package emit

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func HandoffConfigFile(cfg *config.Config, path, target string) bool {
	return cfg != nil && (slices.Contains(cfg.Builtins, "handoff") || slices.Contains(cfg.Builtins, "handoff-hook")) &&
		mayNameRepoStore(cfg, path, nil, []string{target})
}

func SharedHandoffWriteRoot(cfg *config.Config) string {
	root, dir := sharedHandoffCandidate(cfg)
	if dir == "" || !safeIgnoredHandoffDirectory(root, dir) {
		return ""
	}
	return realPath(dir)
}

func sharedHandoffCandidate(cfg *config.Config) (string, string) {
	if cfg == nil || !slices.Contains(cfg.Builtins, "handoff") && !slices.Contains(cfg.Builtins, "handoff-hook") {
		return "", ""
	}
	root, project, ok := MainCheckout()
	current, err := os.Getwd()
	if !ok || err != nil || SamePath(current, project) {
		return "", ""
	}
	return root, filepath.Join(project, ".agnostic-ai", "local", "handoffs")
}

func safeIgnoredHandoffDirectory(root, path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	canonicalRoot, canonical := realPath(root), realPath(abs)
	rel, err := filepath.Rel(canonicalRoot, canonical)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	for p := abs; !SamePath(p, root); p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil && !os.IsNotExist(err) || err == nil && !info.IsDir() {
			return false
		}
		if err == nil {
			dir, err := os.Open(p)
			if err != nil {
				return false
			}
			if err := dir.Close(); err != nil {
				return false
			}
		}
		if filepath.Dir(p) == p {
			return false
		}
	}
	tracked, err := exec.Command("git", "-C", root, "ls-files", "--", filepath.ToSlash(rel)).Output()
	if err != nil || len(tracked) != 0 {
		return false
	}
	canonical += string(filepath.Separator)
	return exec.Command("git", "-C", root, "check-ignore", "-q", "--", canonical).Run() == nil
}

func (s *Session) HandoffConfigWritable(cfg *config.Config, path, target string) bool {
	if !HandoffConfigFile(cfg, path, target) || s.IsUnmanaged(path) {
		return false
	}
	// The output belongs to the current checkout, not the main checkout.
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return false
	}
	root := strings.TrimSpace(string(out))
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for p := abs; !SamePath(p, root); p = filepath.Dir(p) {
		if info, err := os.Lstat(p); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		if filepath.Dir(p) == p {
			return false
		}
	}
	rel, err := filepath.Rel(realPath(root), realPath(abs))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	tracked, err := exec.Command("git", "-C", root, "ls-files", "--", filepath.ToSlash(rel)).Output()
	if err != nil || len(tracked) != 0 {
		return false
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil {
		return false
	}
	return bytes.Contains(data, []byte(ProvenanceMarker)) || len(priorMergedKeys(path)) > 0 ||
		PriorOutputSum != nil && PriorOutputSum(path) != ""
}

func (s *Session) CreateHandoffStore(dir string, dryRun bool) error {
	if dir == "" || dryRun || s.IsCapturing() {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("%s: %w", dir, err)
	}
	return nil
}

func (s *Session) HandoffAccessRoot(cfg *config.Config, path, target, key string, dryRun bool) (string, error) {
	dir := SharedHandoffWriteRoot(cfg)
	if dir == "" {
		if _, candidate := sharedHandoffCandidate(cfg); candidate != "" {
			NoteProject(fmt.Sprintf("%s: shared handoff store %s is unavailable or not private; use an ignored checkout store", target, filepath.ToSlash(candidate)))
		}
		return "", nil
	}
	if !s.HandoffConfigWritable(cfg, path, target) {
		NoteProject(fmt.Sprintf("%s: %s stays as configured; to save shared handoffs, add %q to %s, keeping its other entries", target, filepath.ToSlash(path), filepath.ToSlash(dir), key))
		return "", nil
	}
	if err := s.CreateHandoffStore(dir, dryRun); err != nil {
		if errors.Is(err, fs.ErrPermission) || errors.Is(err, fs.ErrExist) || errors.Is(err, syscall.ENOTDIR) || errors.Is(err, syscall.EROFS) {
			NoteProject(fmt.Sprintf("%s: shared handoff store is unavailable (%v); use an ignored checkout store", target, err))
			return "", nil
		}
		return "", err
	}
	return filepath.ToSlash(dir), nil
}
