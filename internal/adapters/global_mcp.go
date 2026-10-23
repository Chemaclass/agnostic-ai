package adapters

import (
	"runtime"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// UserMCPRenderer is implemented by a target whose user MCP file is JSON
// with a `mcpServers` map.
type UserMCPRenderer interface {
	UserMCPServers(mcps []spec.Entry) map[string]any
}

// UserMCPTableRenderer is implemented by a target whose user MCP servers
// are TOML tables.
type UserMCPTableRenderer interface {
	UserMCPServerTables(mcps []spec.Entry) map[string]string
}

// UserMCPServers renders the MCP specs that emit to target as its user
// `mcpServers` map. ok is false when target has no such renderer.
func UserMCPServers(target string, mcps []spec.Entry) (map[string]any, bool) {
	a, err := Resolve(target)
	if err != nil {
		return nil, false
	}
	r, ok := a.(UserMCPRenderer)
	if !ok {
		return nil, false
	}
	return r.UserMCPServers(userMCPs(a, mcps)), true
}

// UserMCPServerTables renders the MCP specs that emit to target as TOML
// table text per server name. ok is false when target has no such
// renderer.
func UserMCPServerTables(target string, mcps []spec.Entry) (map[string]string, bool) {
	a, err := Resolve(target)
	if err != nil {
		return nil, false
	}
	r, ok := a.(UserMCPTableRenderer)
	if !ok {
		return nil, false
	}
	return r.UserMCPServerTables(userMCPs(a, mcps)), true
}

// userMCPs leaves agent-only servers out: a user MCP file is live in
// every session, and sync --global writes no server into an agent file.
func userMCPs(a Adapter, mcps []spec.Entry) []spec.Entry {
	shared, only := spec.SplitAgentOnly((spec.Bundle{MCPs: mcps}).For(a.Name()).MCPs)
	for _, e := range only {
		emit.NoteEntryOmitted(a.Name(), spec.KindMCP, e.Name)
	}
	noteAgentOnlyGap(a.Name(), len(only), "sync --global writes no server into an agent file")
	return rewriteMCPRefs(a, shared)
}

// EscapeMCPLiterals writes each `${NAME}` in a native server field
// target never expands as the spec's `$${NAME}` escape. Call it before
// ReadMCPEnvRefs.
func EscapeMCPLiterals(target string, server map[string]any) {
	emit.EscapeMCPLiterals(target, server)
}

// ReadMCPEnvRefs rewrites target's own environment reference form in a
// native MCP server's `env`, `headers`, `url`, and `args` values back to
// the spec's `${NAME}`, in place.
func ReadMCPEnvRefs(target string, server map[string]any) {
	emit.ReadMCPEnvRefs(target, server)
}

// UserMCPArrayRenderer emits native TOML array records with their native identity.
type UserMCPArrayRenderer interface {
	UserMCPArrayRecords([]spec.Entry) ([]map[string]any, error)
}

func UserMCPArrayRecords(target string, mcps []spec.Entry) ([]map[string]any, error) {
	a, err := Resolve(target)
	if err != nil {
		return nil, err
	}
	r, ok := a.(UserMCPArrayRenderer)
	if !ok {
		return nil, nil
	}
	return r.UserMCPArrayRecords(userMCPs(a, mcps))
}

// UserHookArrayRenderer emits native hook records after script relocation.
type UserHookArrayRenderer interface {
	UserHookRecords([]spec.Entry, string) ([]map[string]any, error)
}

func UserHookArrayRecords(target string, hooks []spec.Entry, scriptsDir string) ([]map[string]any, error) {
	return UserHookArrayRecordsForPlatform(target, hooks, scriptsDir, runtime.GOOS)
}

type userHookPlatformRenderer interface {
	UserHookRecordsForPlatform([]spec.Entry, string, string) ([]map[string]any, error)
}

func UserHookArrayRecordsForPlatform(target string, hooks []spec.Entry, scriptsDir, platform string) ([]map[string]any, error) {
	a, err := Resolve(target)
	if err != nil {
		return nil, err
	}
	r, ok := a.(UserHookArrayRenderer)
	if !ok {
		return nil, nil
	}
	selected := TargetHooks(target, (spec.Bundle{Hooks: hooks}).For(target).Hooks)
	if native, ok := a.(userHookPlatformRenderer); ok {
		return native.UserHookRecordsForPlatform(selected, scriptsDir, platform)
	}
	return r.UserHookRecords(selected, scriptsDir)
}

type userArrayCanonicalizer interface {
	CanonicalUserArrayRecord(string, map[string]any) any
}

func CanonicalUserArrayRecord(target, parent string, record map[string]any) any {
	a, err := Resolve(target)
	if err != nil {
		return record
	}
	if c, ok := a.(userArrayCanonicalizer); ok {
		return c.CanonicalUserArrayRecord(parent, record)
	}
	return record
}
