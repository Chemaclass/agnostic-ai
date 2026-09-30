package codex

import (
	"bytes"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// projectIgnoredKeys are the top-level keys Codex removes from a project
// `.codex/config.toml`, printing a startup warning for each. The list is
// PROJECT_LOCAL_CONFIG_DENYLIST in openai/codex
// codex-rs/config/src/loader/mod.rs, which is longer than the list on
// learn.chatgpt.com/docs/config-file/config-advanced.
var projectIgnoredKeys = []string{
	"openai_base_url",
	"chatgpt_base_url",
	"apps_mcp_product_sku",
	"responses_api_metadata",
	"model_provider",
	"model_providers",
	"notify",
	"profile",
	"profiles",
	"experimental_realtime_webrtc_call_base_url",
	"experimental_realtime_ws_base_url",
	"otel",
}

const (
	userConfigHome    = "~/.codex/config.toml"
	userProfileHome   = "~/.codex/<name>.config.toml"
	projectConfigName = "a project .codex/config.toml"
)

func isProfileKey(key string) bool { return key == "profile" || key == "profiles" }

// noteIgnoredConfigFields names each `outputs.codex.config` field that
// maps to a key Codex ignores in a project config.toml. The renderer
// never writes those fields.
func noteIgnoredConfigFields(cfg *config.CodexConfig) {
	if cfg == nil {
		return
	}
	if len(cfg.Notify) > 0 {
		emit.NoteProject("codex: outputs.codex.config.notify is not written, since Codex ignores notify in " + projectConfigName + "; set it in " + userConfigHome)
	}
	if len(cfg.ModelProviders) > 0 {
		emit.NoteProject("codex: outputs.codex.config.model-providers is not written, since Codex ignores model_providers in " + projectConfigName + "; set them in " + userConfigHome)
	}
	if len(cfg.Profiles) > 0 {
		emit.NoteProject("codex: outputs.codex.config.profiles is not written, since Codex ignores profiles in " + projectConfigName + " and reads no [profiles.*] table since 0.134.0; put each profile in " + userProfileHome + " and select it with --profile <name>")
	}
}

// dropIgnoredOverlayKeys returns body without the top-level keys Codex
// ignores in a project config.toml, plus the names it dropped. doc is
// body decoded. The overlay file itself keeps them: it is the user's own
// content, and the note says where each one works.
//
// A line filter keeps comments and layout. When its result does not
// decode to doc minus the dropped keys, the filtered table is encoded
// instead, which is correct but loses comments.
func dropIgnoredOverlayKeys(body string, doc map[string]any) (string, []string, error) {
	var dropped []string
	for _, key := range projectIgnoredKeys {
		if _, ok := doc[key]; ok {
			dropped = append(dropped, key)
		}
	}
	if len(dropped) == 0 {
		return body, nil, nil
	}
	slices.Sort(dropped)
	want := maps.Clone(doc)
	for _, key := range dropped {
		delete(want, key)
	}
	filtered := strings.TrimSpace(cutTopLevelKeys(body, dropped))
	if filtered != "" {
		filtered += "\n"
	}
	got := map[string]any{}
	if _, err := toml.Decode(filtered, &got); err != nil || !reflect.DeepEqual(got, want) {
		filtered = ""
		if len(want) > 0 {
			var buf bytes.Buffer
			if err := toml.NewEncoder(&buf).Encode(want); err != nil {
				return "", nil, err
			}
			filtered = buf.String()
		}
	}
	noteIgnoredOverlayKeys(dropped)
	return filtered, dropped, nil
}

func noteIgnoredOverlayKeys(dropped []string) {
	var user, profile []string
	for _, key := range dropped {
		if isProfileKey(key) {
			profile = append(profile, key)
		} else {
			user = append(user, key)
		}
	}
	if len(user) > 0 {
		them := pronoun(user)
		emit.NoteProject("codex: " + configOverlayPath + " sets " + strings.Join(user, ", ") + ", which Codex ignores in " + projectConfigName + ", so sync leaves " + them + " out; move " + them + " to " + userConfigHome)
	}
	if len(profile) > 0 {
		emit.NoteProject("codex: " + configOverlayPath + " sets " + strings.Join(profile, ", ") + ", which Codex ignores in " + projectConfigName + ", so sync leaves " + pronoun(profile) + " out; move each profile to " + userProfileHome + " and select it with --profile <name>")
	}
}

func pronoun(keys []string) string {
	if len(keys) == 1 {
		return "it"
	}
	return "them"
}

// cutTopLevelKeys removes from raw every table whose header starts with
// one of keys, and every assignment before the first header whose key
// starts with one of keys, including a value that spans lines.
func cutTopLevelKeys(raw string, keys []string) string {
	drop := func(key string) bool { return slices.Contains(keys, firstKeySegment(key)) }
	var out []string
	inTable, skipping := false, false
	depth := 0
	multiline := ""
	for _, line := range strings.Split(raw, "\n") {
		if depth > 0 {
			depth = max(depth+bracketDepth(line), 0)
			continue
		}
		if multiline != "" {
			if strings.Contains(line, multiline) {
				multiline = ""
			}
			if !skipping {
				out = append(out, line)
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if header, ok := tableHeader(trimmed); ok {
			inTable = true
			skipping = drop(header)
		}
		if !skipping && !inTable && !strings.HasPrefix(trimmed, "#") {
			if name, _, found := strings.Cut(trimmed, "="); found && drop(name) {
				depth = max(bracketDepth(line), 0)
				continue
			}
		}
		multiline = openMultilineDelimiter(line)
		if !skipping {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// tableHeader returns the key path of a `[table]` or `[[array]]` header
// line, ignoring a trailing comment.
func tableHeader(trimmed string) (string, bool) {
	if !strings.HasPrefix(trimmed, "[") {
		return "", false
	}
	if i := strings.LastIndex(trimmed, "]"); i >= 0 {
		trimmed = trimmed[:i+1]
	}
	if !strings.HasSuffix(trimmed, "]") {
		return "", false
	}
	return strings.TrimSpace(strings.Trim(trimmed, "[]")), true
}

// firstKeySegment returns the first segment of a dotted TOML key, with
// quotes removed.
func firstKeySegment(key string) string {
	key = strings.TrimSpace(key)
	if key != "" && (key[0] == '"' || key[0] == '\'') {
		if end := strings.IndexByte(key[1:], key[0]); end >= 0 {
			return key[1 : end+1]
		}
	}
	first, _, _ := strings.Cut(key, ".")
	return strings.TrimSpace(first)
}

// bracketDepth returns how many `[` and `{` in line stay open, skipping
// single-line strings and a trailing comment.
func bracketDepth(line string) int {
	depth := 0
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#':
			return depth
		case c == '[' || c == '{':
			depth++
		case c == ']' || c == '}':
			depth--
		}
	}
	return depth
}

// openMultilineDelimiter returns the triple quote that opens a string
// line does not close, or "".
func openMultilineDelimiter(line string) string {
	for _, delim := range []string{`"""`, `'''`} {
		if strings.Count(line, delim)%2 == 1 {
			return delim
		}
	}
	return ""
}
