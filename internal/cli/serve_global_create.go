package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// globalCreatePath makes the global setup folder from a project server.
const globalCreatePath = "/api/global/create"

// globalSetupEntries are the names that make a global home count as set up.
// A folder holding none of them is treated as missing.
var globalSetupEntries = []string{
	"agnostic-ai.yaml", "AGNOSTIC_AI.md", "agents", "rules", "hooks",
	"settings", "mcps", "skills", "local", "state",
}

type globalStarterFile struct {
	name    string
	content string
}

var globalStarterFiles = []globalStarterFile{
	{"AGNOSTIC_AI.md", "# My instructions\n\nWrite what every project should know about how you work.\n"},
	{"agnostic-ai.yaml", "version: 1\n"},
	{".gitignore", "/local/\n"},
}

// globalHomeMissing reports whether root holds no global setup. A symlink
// whose target is gone is an error, so create never writes through it.
func globalHomeMissing(root string) (bool, error) {
	info, err := os.Lstat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("%s: %w", root, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if _, err := os.Stat(root); err != nil {
			target, _ := os.Readlink(root)
			return false, fmt.Errorf("%s is a symlink to %s, which does not exist; fix or remove the link", root, target)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return false, fmt.Errorf("%s: %w", root, err)
	}
	for _, entry := range entries {
		if slices.Contains(globalSetupEntries, entry.Name()) {
			return false, nil
		}
	}
	return true, nil
}

type globalCreateRequest struct {
	Git bool `json:"git"`
}

type globalCreateReply struct {
	Root    string   `json:"root"`
	Created []string `json:"created"`
	Git     string   `json:"git"`
}

// globalCreateHandler writes the starter files into the one folder fixed
// when the server started. No request names a path.
type globalCreateHandler struct {
	root string
	mu   sync.Mutex
}

func newGlobalCreateHandler(root string) *globalCreateHandler {
	return &globalCreateHandler{root: root}
}

func (h *globalCreateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if status, msg := editRequestGuard(r); status != 0 {
		writeEditJSON(w, status, editError{Error: msg})
		return
	}
	var req globalCreateRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "send a JSON body such as {\"git\": true}"})
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	missing, err := globalHomeMissing(h.root)
	if err != nil {
		writeEditJSON(w, http.StatusConflict, editError{Error: err.Error()})
		return
	}
	if !missing {
		writeEditJSON(w, http.StatusConflict, editError{Error: "a global setup already exists in " + h.root})
		return
	}
	created, err := writeGlobalStarter(h.root)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, fs.ErrExist) {
			status = http.StatusConflict
		}
		writeEditJSON(w, status, editError{Error: err.Error()})
		return
	}
	gitResult := "skipped: not requested"
	if req.Git {
		gitResult = initGlobalGit(h.root)
	}
	writeEditJSON(w, http.StatusOK, globalCreateReply{Root: h.root, Created: created, Git: gitResult})
}

// writeGlobalStarter creates each file exclusively and removes what it
// made when a later file fails, so a retry starts clean.
func writeGlobalStarter(root string) ([]string, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("%s: %w", root, err)
	}
	var created []string
	for _, f := range globalStarterFiles {
		if err := writeExclusive(filepath.Join(root, f.name), f.content); err != nil {
			for _, name := range created {
				_ = os.Remove(filepath.Join(root, name))
			}
			return nil, err
		}
		created = append(created, f.name)
	}
	return created, nil
}

func writeExclusive(path, content string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// initGlobalGit never fails create: git is a convenience. It drops the
// variables a git hook sets, which would point init at another repository.
func initGlobalGit(root string) string {
	if _, err := os.Lstat(filepath.Join(root, ".git")); err == nil {
		return "skipped: already a git repository"
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return "skipped: git not found"
	}
	cmd := exec.Command(git, "init", "--quiet")
	cmd.Dir = root
	cmd.Env = slices.DeleteFunc(os.Environ(), func(kv string) bool {
		return strings.HasPrefix(kv, "GIT_DIR=") || strings.HasPrefix(kv, "GIT_WORK_TREE=") || strings.HasPrefix(kv, "GIT_INDEX_FILE=")
	})
	if out, err := cmd.CombinedOutput(); err != nil {
		return "skipped: git init failed: " + strings.TrimSpace(string(out))
	}
	return "initialized"
}
