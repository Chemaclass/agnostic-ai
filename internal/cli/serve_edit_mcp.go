package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const editMCPPath = "/api/edit/mcp"

type mcpSpecRequest struct {
	Action   string `json:"action"`
	Audience string `json:"audience"`
	Name     string `json:"name"`
	Target   string `json:"target"`
	Path     string `json:"path"`
	Scope    string `json:"scope"`
	Expected string `json:"expected"`
}

type mcpSpecPreview struct {
	File        string   `json:"file"`
	Spec        string   `json:"spec"`
	Variables   []string `json:"variables"`
	Targets     []string `json:"targets"`
	SyncCommand string   `json:"syncCommand"`
	Expected    string   `json:"expected"`
	OK          bool     `json:"ok,omitempty"`
}

func (h *editHandler) serveMCP(w http.ResponseWriter, body []byte) {
	var req mcpSpecRequest
	if json.Unmarshal(body, &req) != nil || (req.Action != "preview" && req.Action != "apply") || req.Name == "" || req.Target == "" || req.Path == "" {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "send action preview or apply, name, target, path, scope, and audience"})
		return
	}
	where, err := (audienceFields{Audience: req.Audience}).audience()
	if err != nil {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: err.Error()})
		return
	}
	proposed, file, err := h.mcpPreview(req, where)
	if err != nil {
		writeMCPError(w, err)
		return
	}
	if req.Action == "preview" {
		writeEditJSON(w, http.StatusOK, proposed)
		return
	}
	if req.Expected == "" || req.Expected != proposed.Expected {
		writeEditJSON(w, http.StatusConflict, editError{Error: "The server or destination changed; review a fresh preview before applying"})
		return
	}
	var prepare func() (func(bool) error, error)
	if where == audiencePersonal {
		prepare = func() (func(bool) error, error) { return h.preparePersonalOutOfGit(file) }
	}
	guard := func() error {
		now, next, err := h.mcpPreview(req, where)
		if err != nil {
			return err
		}
		if next != file || now.Expected != req.Expected {
			return mcpEditConflict{errors.New("the server or destination changed; review a fresh preview before applying")}
		}
		return nil
	}
	_, err = h.applyValidatedWithPrepare(file, func(data []byte) ([]byte, error) {
		if len(data) > 0 {
			return nil, mcpEditConflict{mcpSpecExists(req.Name)}
		}
		return []byte(proposed.Spec), nil
	}, prepare, guard)
	if err != nil {
		writeMCPError(w, err)
		return
	}
	proposed.OK = true
	writeEditJSON(w, http.StatusOK, proposed)
}

type mcpEditConflict struct{ error }

func (e mcpEditConflict) Unwrap() error { return e.error }

type mcpSpecExists string

func (e mcpSpecExists) Error() string {
	return fmt.Sprintf("a spec named %s already exists", string(e))
}

func writeMCPError(w http.ResponseWriter, err error) {
	status := http.StatusUnprocessableEntity
	var conflict mcpEditConflict
	if errors.As(err, &conflict) {
		status = http.StatusConflict
	}
	message := err.Error()
	var exists mcpSpecExists
	if errors.As(err, &exists) {
		message = fmt.Sprintf("A spec named %s already exists", string(exists))
	}
	writeEditJSON(w, status, editError{Error: message})
}

func (h *editHandler) mcpPreview(req mcpSpecRequest, where editAudience) (mcpSpecPreview, string, error) {
	scope, err := loadCheckScope(h.global)
	if err != nil {
		return mcpSpecPreview{}, "", err
	}
	if err := spec.ValidateMCPNames([]string{req.Name}); err != nil {
		return mcpSpecPreview{}, "", err
	}
	exists := func() error { return mcpEditConflict{mcpSpecExists(req.Name)} }
	for _, entry := range scope.bundle.MCPs {
		if strings.EqualFold(spec.MCPFileName(entry.Name), spec.MCPFileName(req.Name)) {
			return mcpSpecPreview{}, "", exists()
		}
	}
	cfg := scope.cfg
	source := scope.source
	if h.global {
		cfg = &config.Config{Targets: scope.targets}
	}
	home, err := globalUserHome()
	if err != nil {
		return mcpSpecPreview{}, "", err
	}
	rows := nativeMCPServers(home, h.root, source, cfg, h.global)
	if !slices.ContainsFunc(rows, func(row overview.NativeMCP) bool {
		return row.Name == req.Name && row.Target == req.Target && row.Path == req.Path && row.Scope == req.Scope
	}) {
		return mcpSpecPreview{}, "", mcpEditConflict{errors.New("this server is no longer listed in the tool's own config; reload to review it")}
	}
	server, path, format, err := h.mcpNativeServer(home, cfg, req)
	if err != nil {
		return mcpSpecPreview{}, "", err
	}
	raw, err := json.Marshal(server)
	if err != nil {
		return mcpSpecPreview{}, "", fmt.Errorf("read %s: %w", path, err)
	}
	data, variables, err := previewMCPSpec(req.Target, req.Name, path, format, server)
	if err != nil {
		return mcpSpecPreview{}, "", err
	}
	if variables == nil {
		variables = []string{}
	}
	dir := filepath.Join(h.root, "mcps")
	if !h.global {
		dir = config.ResolveSourcePath(h.root, cfg.Sources.MCPs)
	}
	if where == audiencePersonal {
		local, _ := h.personalRoot("")
		dir = filepath.Join(local, "mcps")
	}
	file := filepath.Join(dir, spec.MCPFileName(req.Name))
	if err := refuseDataStreamMCP(req.Name, file); err != nil {
		return mcpSpecPreview{}, "", err
	}
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return mcpSpecPreview{}, "", fmt.Errorf("%s: %w", dir, err)
	}
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), spec.MCPFileName(req.Name)) {
			return mcpSpecPreview{}, "", exists()
		}
	}
	if where == audiencePersonal {
		if err := h.checkPersonalTarget(file); err != nil {
			return mcpSpecPreview{}, "", err
		}
	}
	targets, err := mcpPreviewTargets(cfg, h.global, data)
	if err != nil {
		return mcpSpecPreview{}, "", err
	}
	command := "agnostic-ai sync"
	if h.global {
		command += " --global"
	}
	proposed := mcpSpecPreview{File: overviewDisplay(h.root, file), Spec: string(data), Variables: variables, Targets: targets, SyncCommand: command}
	fingerprint, err := json.Marshal(struct {
		Request mcpSpecRequest
		Preview mcpSpecPreview
		Native  json.RawMessage
	}{mcpSpecRequest{Audience: req.Audience, Name: req.Name, Target: req.Target, Path: req.Path, Scope: req.Scope}, proposed, raw})
	if err != nil {
		return mcpSpecPreview{}, "", fmt.Errorf("prepare MCP preview: %w", err)
	}
	sum := sha256.Sum256(fingerprint)
	proposed.Expected = hex.EncodeToString(sum[:])
	return proposed, file, nil
}

func (h *editHandler) mcpNativeServer(home string, cfg *config.Config, req mcpSpecRequest) (map[string]any, string, string, error) {
	var servers map[string]map[string]any
	var path, format string
	if req.Scope == overview.NativeScopeUser {
		g := globalTargets[req.Target]
		path, format = g.mcpPath(home), g.mcp.format
		servers = readNativeMCPServers(path, format, g.mcp.key)
	} else if req.Target == "claude" && req.Path == nativeHomePath(home, globalTargets["claude"].mcpPath(home)) {
		path, format = globalTargets["claude"].mcpPath(home), "json"
		servers = claudeLocalMCPServers(path, h.root)
	} else {
		f := projectNativeMCP[req.Target]
		path, format = filepath.Join(h.root, projectNativeMCPPath(req.Target, f, cfg)), f.format
		servers = readNativeMCPServers(path, format, f.key)
		if req.Target == "claude" && servers[req.Name] != nil {
			selected := map[string]any{req.Name: servers[req.Name]}
			if err := applyClaudeMCPDisabled(h.root, claudeLayoutFor(cfg).dir, selected); err != nil {
				return nil, path, format, err
			}
		}
	}
	server, ok := servers[req.Name]
	if !ok {
		return nil, path, format, mcpEditConflict{errors.New("the server changed; reload to review it")}
	}
	return server, path, format, nil
}
