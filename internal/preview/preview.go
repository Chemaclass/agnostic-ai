package preview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

type Result struct {
	Text     string
	Hidden   bool
	Withheld bool
}

const redacted = "<redacted>"

const referenceForms = `\$\{\{\s*secrets\.[A-Za-z_][A-Za-z0-9_]*\s*\}\}|\$\{(?:env:)?[A-Za-z_][A-Za-z0-9_]*\}|\{env:[A-Za-z_][A-Za-z0-9_]*\}|\$[A-Za-z_][A-Za-z0-9_]*|%[A-Za-z_][A-Za-z0-9_]*%`

var pureReference = regexp.MustCompile(`^(?i:Bearer )?(?:` + referenceForms + `)+$`)
var referenceTokens = regexp.MustCompile(referenceForms)
var yamlMapping = regexp.MustCompile(`(?m)^\s*(?:-\s+)?(?:[A-Za-z_][A-Za-z0-9_.-]*|"(?:\\.|[^"\\])*"|'(?:''|[^'])*'):\s`)
var tomlAssignment = regexp.MustCompile(`(?m)^\s*(?:[A-Za-z_][A-Za-z0-9_.-]*|"(?:\\.|[^"\\])*"|'[^']*')\s*=`)

func PureReference(value string) bool { return pureReference.MatchString(strings.TrimSpace(value)) }

func withheld() Result {
	return Result{Text: "[preview withheld: content could not be safely shown]\n", Hidden: true, Withheld: true}
}

func Display(path, body string) Result {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return Result{Text: body}
	}
	if strings.HasPrefix(trimmed, "#!/") || (filepath.Ext(path) == ".sh" || filepath.Ext(path) == ".bash") {
		if sensitiveShell(body) {
			return withheld()
		}
		return Result{Text: body}
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return structured("yaml", body)
	}
	if strings.TrimSuffix(strings.SplitN(body, "\n", 2)[0], "\r") == "---" {
		return frontmatter(path, body)
	}
	stripped, _ := StripJSONC([]byte(body))
	jsonStart := strings.TrimSpace(string(stripped))
	if strings.HasPrefix(jsonStart, "{") || strings.HasPrefix(jsonStart, "[") && !tomlAssignment.MatchString(body) {
		return structured("json", body)
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json", ".jsonc":
		return structured("json", body)
	case ".toml":
		return structured("toml", body)
	}
	if filepath.Ext(path) == ".md" || strings.Contains(body, "```") || strings.Contains(body, "~~~") {
		return markdown(body)
	}
	if tomlAssignment.MatchString(body) {
		return structured("toml", body)
	}
	if yamlMapping.MatchString(body) {
		return structured("yaml", body)
	}
	return markdown(body)
}

func structured(format, body string) Result {
	var value any
	aliases := false
	switch format {
	case "json":
		data, _ := StripJSONC([]byte(body))
		if !json.Valid(data) {
			return withheld()
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if decoder.Decode(&value) != nil {
			return withheld()
		}
	case "yaml":
		decoder := yaml.NewDecoder(strings.NewReader(body))
		var documents []any
		for {
			var node yaml.Node
			err := decoder.Decode(&node)
			if err == io.EOF {
				break
			}
			if err != nil {
				return withheld()
			}
			var document any
			if node.Decode(&document) != nil {
				return withheld()
			}
			var hasAlias func(*yaml.Node)
			hasAlias = func(n *yaml.Node) {
				if n.Kind == yaml.AliasNode {
					aliases = true
				}
				for _, c := range n.Content {
					hasAlias(c)
				}
			}
			hasAlias(&node)
			documents = append(documents, document)
		}
		if len(documents) == 1 {
			value = documents[0]
		} else {
			_, changed := sanitize(documents, false)
			if changed {
				return withheld()
			}
			return Result{Text: body}
		}
	case "toml":
		var fields map[string]any
		if _, err := toml.Decode(body, &fields); err != nil {
			return withheld()
		}
		value = fields
	}
	value, changed := sanitize(value, false)
	if !changed {
		return Result{Text: body}
	}
	if aliases {
		return withheld()
	}
	var data []byte
	var err error
	switch format {
	case "json":
		data, err = json.MarshalIndent(value, "", "  ")
	case "yaml":
		data, err = yaml.Marshal(value)
	case "toml":
		var buffer bytes.Buffer
		err = toml.NewEncoder(&buffer).Encode(value)
		data = buffer.Bytes()
	}
	if err != nil {
		return withheld()
	}
	return Result{Text: strings.TrimSuffix(string(data), "\n") + "\n", Hidden: true}
}

func sanitize(value any, sensitive bool) (any, bool) {
	switch v := value.(type) {
	case map[string]any:
		changed := false
		for key, item := range v {
			normal := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
			container := normal == "env" || normal == "environment" || normal == "headers" || normal == "httpheaders"
			if text, ok := item.(string); ok && !sensitive && !CredentialKey(key) {
				hidden := false
				if normal == "url" {
					hidden = sensitiveURL(text)
				} else if normal == "apikeyhelper" {
					hidden = sensitiveHelper(text)
				} else if executionField(normal) {
					hidden = sensitiveShell(text)
				}
				if normal == "url" || executionField(normal) {
					if hidden {
						v[key] = redacted
						changed = true
					}
					continue
				}
			}
			if !sensitive && (normal == "args" || normal == "command") {
				if args, ok := item.([]any); ok {
					var c bool
					v[key], c = sanitizeArgs(args)
					changed = changed || c
					continue
				}
			}
			next, c := sanitize(item, sensitive || container || CredentialKey(key))
			v[key] = next
			changed = changed || c
		}
		return v, changed
	case map[any]any:
		converted := make(map[string]any, len(v))
		for k, item := range v {
			converted[fmt.Sprint(k)] = item
		}
		return sanitize(converted, sensitive)
	case []any:
		changed := false
		for i, item := range v {
			next, c := sanitize(item, sensitive)
			v[i] = next
			changed = changed || c
		}
		return v, changed
	case []map[string]any:
		changed := false
		for i, item := range v {
			next, c := sanitize(item, sensitive)
			fields, ok := next.(map[string]any)
			if !ok {
				return redacted, true
			}
			v[i] = fields
			changed = changed || c
		}
		return v, changed
	case string:
		if v != "" && sensitive && !PureReference(v) || sensitiveURL(v) || sensitiveCommand(v) {
			return redacted, true
		}
	default:
		if sensitive && value != nil {
			return redacted, true
		}
	}
	return value, false
}

func sanitizeArgs(args []any) ([]any, bool) {
	changed, nextSensitive, header := false, false, false
	for i, item := range args {
		text, ok := item.(string)
		if !ok {
			next, c := sanitize(item, nextSensitive)
			args[i] = next
			changed = changed || c
			nextSensitive = false
			continue
		}
		flagName, _, hasValue := strings.Cut(strings.TrimPrefix(text, "--"), "=")
		flag := strings.HasPrefix(text, "--") && CredentialName(flagName)
		headerValue, headerFlag := attachedHeader(text)
		if nextSensitive && !PureReference(text) && (!header || !pureArgument(text)) || headerFlag && !pureArgument(headerValue) || flag && hasValue && !PureReference(strings.SplitN(text, "=", 2)[1]) || sensitiveURL(text) || (!headerFlag || !pureArgument(headerValue)) && sensitiveCommand(text) {
			args[i] = redacted
			changed = true
		}
		header = text == "-H" || text == "--header"
		nextSensitive = flag && !hasValue || header
	}
	return args, changed
}

func sensitiveURL(value string) bool {
	if !strings.Contains(value, "://") && !strings.ContainsAny(value, "?#") {
		return false
	}
	sections := []string{}
	if start := strings.IndexAny(value, "?#"); start >= 0 {
		sections = strings.FieldsFunc(value[start+1:], func(r rune) bool { return r == '?' || r == '#' })
	}
	for _, section := range sections {
		for _, parameter := range strings.FieldsFunc(section, func(r rune) bool { return r == '&' || r == ';' }) {
			key, val, ok := strings.Cut(parameter, "=")
			if !ok || !CredentialParam(key) || val == "" || PureReference(val) {
				continue
			}
			decoded, err := url.QueryUnescape(val)
			if err != nil || !PureReference(decoded) {
				return true
			}
		}
	}
	u, err := url.Parse(value)
	if err != nil {
		return strings.Contains(value, "@")
	}
	if u.User != nil {
		password, present := u.User.Password()
		if !PureReference(u.User.Username()) || present && !PureReference(password) {
			return true
		}
	}
	return false
}

func executionField(key string) bool {
	switch key {
	case "command", "bash", "powershell", "commandwindows", "apikeyhelper":
		return true
	default:
		return false
	}
}
