package emit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"

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
	return handoffConfigPathWritable(path, target)
}

func handoffConfigPathWritable(path, target string) bool {
	if !handoffConfigAncestorsWritable(path) {
		return false
	}
	if tracked, known := handoffConfigTracked(path); tracked || !known {
		return false
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil {
		return false
	}
	if bytes.Contains(data, []byte(ProvenanceMarker)) {
		return true
	}
	if target == "opencode" || target == "windsurf" {
		// These files also hold MCP servers and permission specs, so a
		// merged key says nothing about who wrote the file. A file sync
		// created, or a grant it wrote itself, does.
		if PriorMergedCreated != nil && PriorMergedCreated(path) {
			return true
		}
		if target == "opencode" {
			// An edited grant is still the file's own claim.
			for _, pattern := range PriorClaimedEntries(path, []string{"permission", "external_directory"}) {
				if strings.HasSuffix(pattern, "/.agnostic-ai/local/handoffs/**") {
					return true
				}
			}
			return false
		}
		return priorOwnedHandoffRoot(path, target, data, false, "")
	}
	return len(priorMergedKeys(path)) > 0 ||
		PriorOutputSum != nil && PriorOutputSum(path) != ""
}

// handoffConfigTracked reports whether Git tracks the file at path, and
// whether that could be told.
func handoffConfigTracked(path string) (tracked, known bool) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return false, false
	}
	root := strings.TrimSpace(string(out))
	abs, err := filepath.Abs(path)
	if err != nil {
		return false, false
	}
	rel, err := filepath.Rel(realPath(root), realPath(abs))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, false
	}
	files, err := exec.Command("git", "-C", root, "ls-files", "--", filepath.ToSlash(rel)).Output()
	return err == nil && len(files) != 0, err == nil
}

func handoffConfigAncestorsWritable(path string) bool {
	if CheckScopePath(path) != nil {
		return false
	}
	root, err := os.Getwd()
	if err != nil {
		return false
	}
	root = filepath.Clean(root)
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		root = realPath(root)
		rel, err = filepath.Rel(root, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return false
		}
	}
	for p := filepath.Join(root, rel); p != root; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil && !os.IsNotExist(err) || err == nil && info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	return true
}

// PreserveHandoffConfig holds back a whole config file that carries only
// the handoff grant. OpenCode and Devin are not held: their file also
// carries MCP servers and permission specs, and HandoffAccessRoot skips
// just the grant.
func (s *Session) PreserveHandoffConfig(cfg *config.Config, path, target string) (bool, error) {
	if target == "opencode" || target == "windsurf" {
		return false, nil
	}
	claims := priorMergedKeys(path)
	sum := ""
	if PriorOutputSum != nil {
		sum = PriorOutputSum(path)
	}
	if !HandoffConfigFile(cfg, path, target) && len(claims) == 0 && sum == "" {
		return false, nil
	}
	var data []byte
	var err error
	info, statErr := os.Lstat(path)
	missing := os.IsNotExist(statErr)
	if missing && handoffConfigAncestorsWritable(path) {
		return false, nil
	}
	link := missing || statErr == nil && info.Mode()&os.ModeSymlink != 0
	if link {
		if destination, statErr := os.Stat(path); statErr != nil {
			err = statErr
		} else if !destination.Mode().IsRegular() {
			err = fs.ErrInvalid
		}
	}
	if err == nil {
		data, err = os.ReadFile(path)
	}
	priorHandoff := priorOwnedHandoffRoot(path, target, data, err != nil, sum)
	if !HandoffConfigFile(cfg, path, target) && !priorHandoff {
		return false, nil
	}
	if s.IsUnmanaged(path) {
		return true, nil
	}
	if handoffConfigPathWritable(path, target) {
		return false, nil
	}
	NoteProject(fmt.Sprintf("%s: %s stays as configured; its handoff permissions are not writable", target, filepath.ToSlash(path)))
	if len(claims) == 0 && sum == "" {
		return true, nil
	}
	if err != nil && !link {
		return true, fmt.Errorf("read %s: %w", path, err)
	}
	// Keep the output and its claims in the ledger without following a refused write.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.capturing && err == nil {
		s.captured = append(s.captured, CapturedFile{Path: path, Content: string(data), Merged: len(claims) > 0, Keys: claims})
	}
	if s.recording {
		s.recorded = append(s.recorded, path)
	}
	if s.detailing {
		s.detailed = append(s.detailed, WrittenFile{Path: path, Bytes: len(data), Action: "skip", Sum: sum, Merged: len(claims) > 0, Keys: claims})
	}
	return true, nil
}

func priorOwnedHandoffRoot(path, target string, data []byte, unreadable bool, sum string) bool {
	if target == "codex" {
		if sum == "" {
			return false
		}
		if unreadable {
			return true
		}
		var doc struct {
			Sandbox struct {
				WritableRoots []string `toml:"writable_roots"`
			} `toml:"sandbox_workspace_write"`
		}
		if _, err := toml.Decode(string(data), &doc); err != nil {
			return false
		}
		for _, dir := range doc.Sandbox.WritableRoots {
			if strings.HasSuffix(filepath.ToSlash(dir), "/.agnostic-ai/local/handoffs") {
				return true
			}
		}
		return false
	}
	if target == "opencode" {
		entries := PriorClaimedEntries(path, []string{"permission", "external_directory"})
		if unreadable {
			return len(entries) > 0
		}
		data, _ = StripJSONC(data)
		var doc struct {
			Permission struct {
				ExternalDirectory map[string]string `json:"external_directory"`
			} `json:"permission"`
		}
		if json.Unmarshal(data, &doc) != nil {
			return false
		}
		for _, pattern := range entries {
			if strings.HasSuffix(pattern, "/.agnostic-ai/local/handoffs/**") && doc.Permission.ExternalDirectory[pattern] == "allow" {
				return true
			}
		}
		return false
	}
	if target == "windsurf" {
		items := PriorClaimedItems(path, []string{"permissions", "allow"})
		if unreadable {
			return len(items) > 0
		}
		data, _ = StripJSONC(data)
		var doc struct {
			Permissions struct {
				Allow []string `json:"allow"`
			} `json:"permissions"`
		}
		if json.Unmarshal(data, &doc) != nil {
			return false
		}
		for _, rule := range doc.Permissions.Allow {
			if strings.HasPrefix(rule, "Write(") && strings.HasSuffix(rule, "/.agnostic-ai/local/handoffs/**)") && slices.Contains(items, ContentSum(canonicalJSON(rule))) {
				return true
			}
		}
		return false
	}
	items := PriorClaimedItems(path, []string{"permissions", "additionalDirectories"})
	if len(items) == 0 {
		return false
	}
	if unreadable {
		return true
	}
	data, _ = StripJSONC(data)
	var doc struct {
		Permissions struct {
			AdditionalDirectories []string `json:"additionalDirectories"`
		} `json:"permissions"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return false
	}
	for _, dir := range doc.Permissions.AdditionalDirectories {
		if strings.HasSuffix(filepath.ToSlash(dir), "/.agnostic-ai/local/handoffs") && slices.Contains(items, ContentSum(canonicalJSON(dir))) {
			return true
		}
	}
	return false
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
	grant, _, err := s.HandoffAccess(cfg, path, target, key, dryRun)
	return grant, err
}

// HandoffAccess returns the store sync grants, and the store a refused
// config already grants by hand. The second belongs to the user: a
// target that rewrites the whole permission value carries it over
// without claiming it, and no note asks for what is already there.
func (s *Session) HandoffAccess(cfg *config.Config, path, target, key string, dryRun bool) (grant, kept string, err error) {
	dir := SharedHandoffWriteRoot(cfg)
	if dir == "" {
		if _, candidate := sharedHandoffCandidate(cfg); candidate != "" {
			NoteProject(fmt.Sprintf("%s: shared handoff store %s is unavailable or not private; use an ignored checkout store", target, filepath.ToSlash(candidate)))
		}
		return "", "", nil
	}
	if unsafe := handoffGlobCharacters(target); unsafe != "" && strings.ContainsAny(dir, unsafe) {
		NoteProject(fmt.Sprintf("%s: shared handoff store %s contains glob characters and cannot be granted safely; use an ignored checkout store", target, filepath.ToSlash(dir)))
		return "", "", nil
	}
	if !s.HandoffConfigWritable(cfg, path, target) {
		if handoffGrantOnDisk(path, target, filepath.ToSlash(dir)) {
			return "", filepath.ToSlash(dir), nil
		}
		if tracked, _ := handoffConfigTracked(path); tracked {
			NoteProject(fmt.Sprintf("%s: %s is tracked by Git, so sync leaves its handoff permission alone; run `git rm --cached %s` so sync can manage it, since the store path is local to this machine", target, filepath.ToSlash(path), filepath.ToSlash(path)))
			return "", "", nil
		}
		NoteProject(fmt.Sprintf("%s: %s stays as configured; to save shared handoffs, add %s to %s, keeping its other entries", target, filepath.ToSlash(path), handoffNativeEntry(target, filepath.ToSlash(dir)), key))
		return "", "", nil
	}
	if err := s.CreateHandoffStore(dir, dryRun); err != nil {
		if errors.Is(err, fs.ErrPermission) || errors.Is(err, fs.ErrExist) || errors.Is(err, syscall.ENOTDIR) || errors.Is(err, syscall.EROFS) {
			NoteProject(fmt.Sprintf("%s: shared handoff store is unavailable (%v); use an ignored checkout store", target, err))
			return "", "", nil
		}
		return "", "", err
	}
	return filepath.ToSlash(dir), "", nil
}

// PersonalMemoryGrantable reports whether dir can go into target's write
// grant for personal memory. A path with a glob character would grant
// more than the folder, so it gets no grant and a note.
func PersonalMemoryGrantable(target, dir string) bool {
	unsafe := handoffGlobCharacters(target)
	if unsafe == "" || !strings.ContainsAny(dir, unsafe) {
		return true
	}
	NoteProject(fmt.Sprintf("%s: personal memory folder %s contains glob characters and cannot be granted safely; move AGNOSTIC_AI_HOME or set memory.personal: checkout", target, filepath.ToSlash(dir)))
	return false
}

// handoffGlobCharacters lists the characters that would turn the main
// checkout path into a broader pattern in a target whose grant is a glob.
// Devin also reads `)` as the end of its `Write(` rule.
func handoffGlobCharacters(target string) string {
	switch target {
	case "opencode":
		return "*?[]{}"
	case "windsurf":
		return "*?[]{}()"
	}
	return ""
}

// handoffNativeEntry spells the grant the way the target's config holds it.
func handoffNativeEntry(target, dir string) string {
	switch target {
	case "opencode":
		return fmt.Sprintf("%q: \"allow\"", dir+"/**")
	case "windsurf":
		return "Write(" + dir + "/**)"
	}
	return fmt.Sprintf("%q", dir)
}

// handoffGrantOnDisk reports whether the config at path already grants
// dir in the target's native spelling.
func handoffGrantOnDisk(path, target, dir string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	data, _ = StripJSONC(data)
	switch target {
	case "opencode":
		var doc struct {
			Permission struct {
				ExternalDirectory map[string]json.RawMessage `json:"external_directory"`
			} `json:"permission"`
		}
		if json.Unmarshal(data, &doc) != nil {
			return false
		}
		_, ok := doc.Permission.ExternalDirectory[dir+"/**"]
		return ok
	case "windsurf":
		var doc struct {
			Permissions struct {
				Allow []string `json:"allow"`
			} `json:"permissions"`
		}
		return json.Unmarshal(data, &doc) == nil && slices.Contains(doc.Permissions.Allow, "Write("+dir+"/**)")
	}
	return false
}
