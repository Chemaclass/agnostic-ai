package vibe

import (
	"bytes"
	"fmt"
	"maps"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func encode(doc map[string]any) (string, error) {
	var b bytes.Buffer
	if err := toml.NewEncoder(&b).Encode(doc); err != nil {
		return "", fmt.Errorf("vibe: encode native TOML: %w", err)
	}
	return b.String(), nil
}
func writeDocument(sess *emit.Session, path string, doc map[string]any, has bool, dryRun bool) error {
	if !has {
		return sess.RemoveGenerated(path, dryRun)
	}
	body, err := encode(doc)
	if err != nil {
		return err
	}
	return sess.WriteFile(path, emit.WithHeader(body, emit.FormatTOML), dryRun)
}
func prepareConfig(b spec.Bundle, cfg *config.Config) (map[string]any, []map[string]any, error) {
	doc := map[string]any{}
	for _, s := range b.Settings {
		if m := emit.SettingsModel([]spec.Entry{s}, target); m != "" {
			doc["active_model"] = m
		}
		emit.MergeSettingsCustomKeys(doc, []spec.Entry{s}, target)
	}
	servers, err := (Adapter{}).UserMCPArrayRecords(b.MCPs)
	if err != nil {
		return nil, nil, err
	}
	conf := emit.OutputConfFile(cfg, target, ".vibe/config.toml")
	mcp := emit.OutputMCPFile(cfg, target, conf)
	if conf == mcp && len(servers) > 0 {
		doc["mcp_servers"] = servers
	}
	if err := validateConfigMCPNames(doc); err != nil {
		return nil, nil, err
	}
	return doc, servers, nil
}
func emitConfig(sess *emit.Session, doc map[string]any, servers []map[string]any, cfg *config.Config, dryRun bool) error {
	conf := emit.OutputConfFile(cfg, target, ".vibe/config.toml")
	mcp := emit.OutputMCPFile(cfg, target, conf)
	if conf == mcp {
		return writeDocument(sess, conf, doc, len(doc) > 0, dryRun)
	}
	if err := writeDocument(sess, conf, doc, len(doc) > 0, dryRun); err != nil {
		return err
	}
	return writeDocument(sess, mcp, map[string]any{"mcp_servers": servers}, len(servers) > 0, dryRun)
}

func (Adapter) UserMCPArrayRecords(mcps []spec.Entry) ([]map[string]any, error) {
	var records []map[string]any
	for _, e := range mcps {
		m := emit.ResolveMeta(e.Meta, target)
		d := map[string]any{"name": e.Name}
		transport, _ := m["type"].(string)
		if transport == "" {
			transport = "stdio"
			if url, _ := m["url"].(string); url != "" {
				transport = "http"
			}
		}
		switch transport {
		case "stdio":
			d["transport"] = "stdio"
			for _, k := range []string{"command", "args", "env", "cwd"} {
				if v, ok := m[k]; ok {
					d[k] = v
				}
			}
		case "http", "streamable-http":
			d["transport"] = "streamable-http"
			d["url"] = m["url"]
			if headers, ok := m["headers"]; ok {
				d["auth"] = map[string]any{"type": "static", "headers": headers}
			}
		default:
			emit.NoteFieldNoOp(target, spec.KindMCP, "type", 1, "Vibe supports stdio and streamable HTTP, not "+transport)
			continue
		}
		for _, k := range []string{"disabled", "disabled_tools", "startup_timeout_sec", "tool_timeout_sec"} {
			if v, ok := m[k]; ok {
				d[k] = v
			}
		}
		if _, ok := m["description"]; ok {
			emit.NoteFieldNoOp(target, spec.KindMCP, "description", 1, "Vibe has no server description field")
		}
		maps.Copy(d, native(e.Meta))
		d["name"] = e.Name
		switch d["transport"] {
		case "stdio":
			if cmd, ok := d["command"].(string); (!ok || strings.TrimSpace(cmd) == "") && len(stringsList(d["command"])) == 0 {
				return nil, fmt.Errorf("vibe: MCP %s requires a command", e.Name)
			}
		case "http", "streamable-http":
			if url, _ := d["url"].(string); strings.TrimSpace(url) == "" {
				return nil, fmt.Errorf("vibe: MCP %s requires a URL", e.Name)
			}
		default:
			return nil, fmt.Errorf("vibe: MCP %s has unsupported native transport %v", e.Name, d["transport"])
		}
		records = append(records, d)
	}
	_, err := (Adapter{}).UserMCPArrayIdentities(records)
	return records, err
}

func (Adapter) CanonicalUserArrayRecord(parent string, record map[string]any) any {
	doc := maps.Clone(record)
	if parent == "hooks" {
		if command, ok := doc["command"].(string); ok {
			doc["command"] = DeclaredHookCommand(command)
		}
		if doc["strict"] == false {
			delete(doc, "strict")
		}
		if timeout, ok := doc["timeout"]; ok && fmt.Sprint(timeout) == "60" {
			delete(doc, "timeout")
		}
		return doc
	}
	if parent != "mcp_servers" {
		return doc
	}
	if name, ok := doc["name"].(string); ok {
		doc["name"] = nativeMCPName(name)
	}
	if doc["transport"] == "http" {
		doc["transport"] = "streamable-http"
	}
	if doc["disabled"] == false {
		delete(doc, "disabled")
	}
	if doc["sampling_enabled"] == false {
		delete(doc, "sampling_enabled")
	}
	for _, key := range []string{"args", "disabled_tools"} {
		switch list := doc[key].(type) {
		case []any:
			if len(list) == 0 {
				delete(doc, key)
			}
		case []string:
			if len(list) == 0 {
				delete(doc, key)
			}
		}
	}
	for key, value := range map[string]string{"startup_timeout_sec": "10", "tool_timeout_sec": "60"} {
		if fmt.Sprint(doc[key]) == value {
			delete(doc, key)
		}
	}
	auth, _ := doc["auth"].(map[string]any)
	auth = maps.Clone(auth)
	for _, key := range []string{"headers", "api_key_env", "api_key_header", "api_key_format"} {
		if value, ok := doc[key]; ok {
			if auth == nil {
				auth = map[string]any{"type": "static"}
			}
			auth[key] = value
			delete(doc, key)
		}
	}
	if auth != nil {
		if auth["type"] == "static" {
			delete(auth, "type")
			if auth["api_key_env"] == "" {
				delete(auth, "api_key_env")
			}
			if auth["api_key_header"] == "Authorization" {
				delete(auth, "api_key_header")
			}
			if auth["api_key_format"] == "Bearer {token}" {
				delete(auth, "api_key_format")
			}
			if headers, ok := auth["headers"].(map[string]any); ok && len(headers) == 0 {
				delete(auth, "headers")
			}
		}
		if len(auth) > 0 {
			doc["auth"] = auth
		} else {
			delete(doc, "auth")
		}
	}
	return doc
}
