package codex

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
)

type HookTrustFinding struct {
	Path    string `json:"path"`
	Event   string `json:"event"`
	Group   int    `json:"group"`
	Handler int    `json:"handler"`
	Hook    string `json:"hook"`
	Status  string `json:"status"`
	Problem string `json:"problem,omitempty"`
}

func (f HookTrustFinding) String() string {
	if f.Problem != "" {
		return fmt.Sprintf("codex: %s: cannot check hook trust: %s; open /hooks in Codex to review hook status", f.Path, f.Problem)
	}
	action := "review and trust"
	if f.Status == "disabled" {
		action = "review and enable"
	}
	return fmt.Sprintf("codex: %s: %s hook %q is %s; open /hooks in Codex to %s it", f.Path, f.Event, f.Hook, f.Status, action)
}

// UserHooksPath uses the same user config directory as Codex's runtime.
func UserHooksPath() (string, error) {
	home, err := codexHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "hooks.json"), nil
}

func codexHome() (string, error) {
	if configured := os.Getenv("CODEX_HOME"); configured != "" {
		info, err := os.Stat(configured)
		if err != nil {
			return "", fmt.Errorf("CODEX_HOME %s: %w", configured, err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("CODEX_HOME %s: must be a directory", configured)
		}
		resolved, err := filepath.EvalSymlinks(configured)
		if err != nil {
			return "", fmt.Errorf("CODEX_HOME %s: %w", configured, err)
		}
		return filepath.Abs(resolved)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("codex user home: %w", err)
	}
	return filepath.Abs(filepath.Join(home, ".codex"))
}

func HookTrustFindings(path string, body []byte) ([]HookTrustFinding, error) {
	home, err := codexHome()
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(home, "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("%s: %w", configPath, err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if configured := os.Getenv("CODEX_HOME"); configured != "" {
		configured, err = filepath.Abs(configured)
		if err != nil {
			return nil, fmt.Errorf("CODEX_HOME: %w", err)
		}
		if absolute == filepath.Join(configured, "hooks.json") {
			absolute = filepath.Join(home, "hooks.json")
		}
	}
	findings, err := inspectHookTrust(absolute, body, data, runtime.GOOS)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", configPath, err)
	}
	return findings, nil
}

func NoteHookTrust(path string, body []byte) {
	findings, err := HookTrustFindings(path, body)
	if err != nil {
		emit.NoteProject(fmt.Sprintf("codex: %s: cannot check hook trust: %v; open /hooks in Codex to review hook status", path, err))
		return
	}
	for _, finding := range findings {
		emit.NoteProject(finding.String())
	}
}

type trustHandlers []map[string]any

func (handlers *trustHandlers) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("hooks must be an array")
	}
	type entries trustHandlers
	var next entries
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&next); err != nil {
		return err
	}
	*handlers = trustHandlers(next)
	return nil
}

func inspectHookTrust(path string, body, userConfig []byte, platform string) ([]HookTrustFinding, error) {
	var config map[string]any
	if _, err := toml.Decode(string(userConfig), &config); err != nil {
		return nil, fmt.Errorf("parse user config: %w", err)
	}
	hooks, _ := config["hooks"].(map[string]any)
	states, _ := hooks["state"].(map[string]any)
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if doc == nil {
		return nil, fmt.Errorf("parse %s: hooks file must be an object", path)
	}
	for key := range doc {
		if key != "description" && key != "hooks" {
			return nil, fmt.Errorf("parse %s: unknown field %q", path, key)
		}
	}
	if description, exists := doc["description"]; exists {
		var text *string
		if err := json.Unmarshal(description, &text); err != nil {
			return nil, fmt.Errorf("parse %s description: %w", path, err)
		}
	}
	events := map[string]json.RawMessage{}
	if raw, exists := doc["hooks"]; exists {
		if string(raw) == "null" {
			return nil, fmt.Errorf("parse %s: hooks must be an object", path)
		}
		if err := json.Unmarshal(raw, &events); err != nil {
			return nil, fmt.Errorf("parse %s hooks: %w", path, err)
		}
	}
	var findings []HookTrustFinding
	for _, event := range hookTrustEvents {
		raw, exists := events[event.name]
		if !exists {
			continue
		}
		var groups []*struct {
			Matcher *string       `json:"matcher"`
			Hooks   trustHandlers `json:"hooks"`
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if string(raw) == "null" {
			return nil, fmt.Errorf("parse %s: %s must be an array", path, event.name)
		}
		if err := decoder.Decode(&groups); err != nil {
			return nil, fmt.Errorf("parse %s %s: %w", path, event.name, err)
		}
		for gi, group := range groups {
			if group == nil {
				return nil, fmt.Errorf("parse %s: %s group %d must be an object", path, event.name, gi)
			}
			matcher := group.Matcher
			if slices.Contains([]string{"UserPromptSubmit", "Stop", "Interrupt"}, event.name) {
				matcher = nil
			}
			if matcher != nil && *matcher != "*" {
				if _, err := regexp.Compile(*matcher); err != nil {
					return nil, fmt.Errorf("%s: %s matcher: %w", path, event.name, err)
				}
			}
			for hi, handler := range group.Hooks {
				normalized, label, err := normalizeTrustHandler(event.name, handler, platform)
				if err != nil {
					return nil, fmt.Errorf("%s: %s hook %d:%d: %w", path, event.name, gi, hi, err)
				}
				if normalized == nil {
					continue
				}
				identity := map[string]any{"event_name": event.key, "hooks": []any{normalized}}
				if matcher != nil {
					identity["matcher"] = *matcher
				}
				hash, err := hookTrustHash(identity)
				if err != nil {
					return nil, fmt.Errorf("%s: hash hook: %w", path, err)
				}
				key := fmt.Sprintf("%s:%s:%d:%d", path, event.key, gi, hi)
				state, _ := states[key].(map[string]any)
				// The runtime ignores malformed state entries instead of accepting partial trust.
				if value, exists := state["enabled"]; exists {
					if _, ok := value.(bool); !ok {
						state = nil
					}
				}
				if value, exists := state["trusted_hash"]; exists {
					if _, ok := value.(string); !ok {
						state = nil
					}
				}
				status := "untrusted"
				if enabled, ok := state["enabled"].(bool); ok && !enabled {
					status = "disabled"
				} else if trusted, ok := state["trusted_hash"].(string); ok {
					if trusted == hash {
						continue
					}
					status = "modified"
				}
				findings = append(findings, HookTrustFinding{Path: path, Event: event.name, Group: gi, Handler: hi, Hook: label, Status: status})
			}
		}
	}
	return findings, nil
}

var hookTrustEvents = []struct{ name, key string }{
	{"SessionStart", "session_start"}, {"SubagentStart", "subagent_start"}, {"UserPromptSubmit", "user_prompt_submit"},
	{"PreToolUse", "pre_tool_use"}, {"PermissionRequest", "permission_request"}, {"PostToolUse", "post_tool_use"},
	{"PreCompact", "pre_compact"}, {"PostCompact", "post_compact"}, {"Stop", "stop"}, {"SubagentStop", "subagent_stop"},
	{"SessionEnd", "session_end"}, {"Interrupt", "interrupt"},
}

func normalizeTrustHandler(event string, raw map[string]any, platform string) (map[string]any, string, error) {
	kind, _ := raw["type"].(string)
	if kind == "prompt" || kind == "agent" {
		return nil, "", nil
	}
	if kind != "command" && kind != "mcp_tool" {
		return nil, "", fmt.Errorf("unsupported hook type %q", kind)
	}
	normalized := map[string]any{"type": kind}
	timeout := uint64(600)
	if event == "SessionEnd" || event == "Interrupt" {
		timeout = 1
	}
	if value := raw["timeout"]; value != nil {
		number, ok := value.(json.Number)
		if !ok {
			return nil, "", fmt.Errorf("timeout must be a nonnegative integer")
		}
		parsed, err := strconv.ParseUint(string(number), 10, 64)
		if err != nil {
			return nil, "", fmt.Errorf("timeout: %w", err)
		}
		timeout = max(parsed, 1)
	}
	if event == "SessionEnd" || event == "Interrupt" {
		timeout = min(timeout, 3)
	}
	if timeout > math.MaxInt64 {
		return nil, "", fmt.Errorf("timeout cannot be represented in Codex hook identity")
	}
	normalized["timeout"] = timeout
	if value := raw["statusMessage"]; value != nil {
		if _, ok := value.(string); !ok {
			return nil, "", fmt.Errorf("statusMessage must be a string")
		}
		normalized["statusMessage"] = value
	}
	if kind == "mcp_tool" {
		if event == "SessionEnd" {
			return nil, "", nil
		}
		server, _ := raw["server"].(string)
		tool, _ := raw["tool"].(string)
		if strings.TrimSpace(server) == "" || strings.TrimSpace(tool) == "" {
			return nil, "", fmt.Errorf("server and tool must be nonempty strings")
		}
		normalized["server"], normalized["tool"] = server, tool
		input := map[string]any{}
		if value, exists := raw["input"]; exists {
			var ok bool
			input, ok = value.(map[string]any)
			if !ok {
				return nil, "", fmt.Errorf("input must be an object")
			}
			if err := validateTrustInput(input); err != nil {
				return nil, "", err
			}
		}
		normalized["input"] = input
		return normalized, server + "/" + tool, nil
	}
	command, ok := raw["command"].(string)
	if !ok {
		return nil, "", fmt.Errorf("command must be a string")
	}
	windows := raw["commandWindows"]
	if windows == nil {
		windows = raw["command_windows"]
	}
	if windows != nil {
		alternate, ok := windows.(string)
		if !ok {
			return nil, "", fmt.Errorf("commandWindows must be a string")
		}
		if platform == "windows" {
			command = alternate
		}
	}
	if strings.TrimSpace(command) == "" {
		return nil, "", fmt.Errorf("command must be nonempty")
	}
	normalized["command"] = command
	async := false
	if value, exists := raw["async"]; exists {
		var ok bool
		async, ok = value.(bool)
		if !ok {
			return nil, "", fmt.Errorf("async must be a boolean")
		}
	}
	normalized["async"] = async
	if value := raw["additionalContextLimit"]; value != nil {
		number, ok := value.(json.Number)
		if !ok {
			return nil, "", fmt.Errorf("additionalContextLimit must be a nonnegative integer")
		}
		limit, err := strconv.ParseUint(string(number), 10, 64)
		if err != nil {
			return nil, "", fmt.Errorf("additionalContextLimit: %w", err)
		}
		if limit != 2500 && slices.Contains([]string{"PreToolUse", "PostToolUse", "SessionStart", "UserPromptSubmit", "SubagentStart"}, event) {
			if limit > math.MaxInt64 {
				return nil, "", fmt.Errorf("additionalContextLimit cannot be represented in Codex hook identity")
			}
			normalized["additionalContextLimit"] = limit
		}
	}
	return normalized, command, nil
}

func validateTrustInput(value any) error {
	switch v := value.(type) {
	case nil:
		return fmt.Errorf("input cannot contain null")
	case map[string]any:
		for _, child := range v {
			if err := validateTrustInput(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range v {
			if err := validateTrustInput(child); err != nil {
				return err
			}
		}
	case json.Number:
		if !strings.ContainsAny(string(v), ".eE") {
			if _, err := strconv.ParseInt(string(v), 10, 64); err != nil {
				return fmt.Errorf("input integer: %w", err)
			}
		}
	}
	return nil
}

func hookTrustHash(identity map[string]any) (string, error) {
	var data bytes.Buffer
	if err := writeTrustJSON(&data, identity); err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data.Bytes())), nil
}

// Rust's JSON serializer writes Unicode and HTML characters without Go's extra escaping.
func writeTrustJSON(out *bytes.Buffer, value any) error {
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		out.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := writeTrustJSON(out, key); err != nil {
				return err
			}
			out.WriteByte(':')
			if err := writeTrustJSON(out, v[key]); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	case []any:
		out.WriteByte('[')
		for i, child := range v {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := writeTrustJSON(out, child); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case string:
		out.WriteByte('"')
		for _, char := range v {
			switch char {
			case '"', '\\':
				out.WriteByte('\\')
				out.WriteRune(char)
			case '\b':
				out.WriteString(`\b`)
			case '\f':
				out.WriteString(`\f`)
			case '\n':
				out.WriteString(`\n`)
			case '\r':
				out.WriteString(`\r`)
			case '\t':
				out.WriteString(`\t`)
			default:
				if char < 32 {
					fmt.Fprintf(out, `\u%04x`, char)
				} else {
					out.WriteRune(char)
				}
			}
		}
		out.WriteByte('"')
	case json.Number:
		if strings.ContainsAny(string(v), ".eE") {
			number, err := strconv.ParseFloat(string(v), 64)
			if err != nil {
				return err
			}
			out.WriteString(trustFloat(number))
		} else {
			number, err := strconv.ParseInt(string(v), 10, 64)
			if err != nil {
				return err
			}
			out.WriteString(strconv.FormatInt(number, 10))
		}
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		out.Write(data)
	}
	return nil
}

func trustFloat(number float64) string {
	scientific := strconv.FormatFloat(number, 'e', -1, 64)
	significand, exponentText, _ := strings.Cut(scientific, "e")
	exponent, _ := strconv.Atoi(exponentText)
	if exponent >= -5 && exponent <= 15 {
		rendered := strconv.FormatFloat(number, 'f', -1, 64)
		if !strings.Contains(rendered, ".") {
			rendered += ".0"
		}
		return rendered
	}
	sign := "+"
	if exponent < 0 {
		sign = "-"
		exponent = -exponent
	}
	return significand + "e" + sign + strconv.Itoa(exponent)
}
