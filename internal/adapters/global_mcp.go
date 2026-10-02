package adapters

import (
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
	return r.UserMCPServers(userMCPs(target, mcps)), true
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
	return r.UserMCPServerTables(userMCPs(target, mcps)), true
}

func userMCPs(target string, mcps []spec.Entry) []spec.Entry {
	return emit.RewriteMCPEnvRefs(target, (spec.Bundle{MCPs: mcps}).For(target).MCPs)
}

// ReadMCPEnvRefs rewrites target's own environment reference form in a
// native MCP server's `env`, `headers`, `url`, and `args` values back to
// the spec's `${NAME}`, in place.
func ReadMCPEnvRefs(target string, server map[string]any) {
	emit.ReadMCPEnvRefs(target, server)
}
