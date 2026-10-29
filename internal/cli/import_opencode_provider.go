package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/preview"
)

const (
	opencodeProviderKey      = "provider"
	opencodeProviderSpecFile = "opencode-provider.yaml"
)

// opencodeReference matches OpenCode's own `{env:NAME}` and `{file:path}`
// substitutions, which name where a value lives rather than hold it.
var opencodeReference = regexp.MustCompile(`\{env:[A-Za-z_][A-Za-z0-9_]*\}|\{file:[^}]+\}`)

// planOpencodeProvider returns the `provider` block of `opencode.json` to
// keep as `x-opencode.provider`, or nil to leave it in that file, which
// sync keeps when no spec owns the key. It runs before import writes.
func planOpencodeProvider(root string, sources config.Sources) (map[string]any, error) {
	path := filepath.Join(root, opencodeMCPFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	data, _ = adapters.StripJSONC(data)
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var doc map[string]any
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	raw, held := doc[opencodeProviderKey]
	if !held {
		return nil, nil
	}
	provider, ok := raw.(map[string]any)
	if !ok {
		keptf("warning: opencode: left out provider in %s: it is not an object\n", opencodeMCPFile)
		return nil, nil
	}
	if len(provider) == 0 {
		return nil, nil
	}
	bundle, err := loadOpencodeOwners(root, sources)
	if err != nil {
		return nil, err
	}
	provider = yamlNumbers(provider).(map[string]any)
	if owner, value := opencodeSettingsOwner(bundle, root, sources, opencodeProviderKey, opencodeProviderSpecFile); owner != "" {
		if sameJSON(provider, value) {
			return nil, nil
		}
		return nil, fmt.Errorf("import %s: provider differs from settings spec %s, which sets x-opencode.provider", path, owner)
	}
	credential := opencodeProviderCredential(provider)
	if credential == "" {
		return provider, nil
	}
	existing := filepath.Join(importSourcePath(root, sources.Settings), opencodeProviderSpecFile)
	if _, err := os.Stat(existing); err == nil {
		return nil, fmt.Errorf("import %s: %s holds a credential import cannot copy, and sync would write the older %s over it; remove that spec or use an {env:NAME} reference", path, credential, existing)
	}
	keptf("warning: opencode: left out provider in %s: %s holds a credential; sync keeps the block in that file as written\n", opencodeMCPFile, credential)
	return nil, nil
}

// sameJSON reports whether a and b encode to the same JSON, so a whole
// number decoded as int64 from JSON matches the int YAML decodes.
func sameJSON(a, b any) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	return err == nil && bytes.Equal(left, right)
}

func writeOpencodeProvider(provider map[string]any, dstDir string) error {
	return writeSettingsSpec(dstDir, opencodeProviderSpecFile, map[string]any{
		"target":     "opencode",
		"x-opencode": map[string]any{opencodeProviderKey: provider},
	})
}

// opencodeProviderCredential returns the dotted path of the first value in
// provider that holds a literal credential, or "".
func opencodeProviderCredential(provider map[string]any) string {
	_, hidden := preview.RedactTree(provider, nil, func(path []any, value any) bool {
		text, ok := value.(string)
		if !ok || strings.TrimSpace(opencodeReference.ReplaceAllString(text, "")) == "" {
			return false
		}
		key := ""
		for _, step := range path {
			if name, ok := step.(string); ok {
				key = name
			}
		}
		return credentialSettingValue(key, opencodeNativeCredentialView(text).(string))
	})
	if len(hidden) == 0 {
		return ""
	}
	parts := []string{opencodeProviderKey}
	for _, step := range hidden[0] {
		parts = append(parts, fmt.Sprint(step))
	}
	return strings.Join(parts, ".")
}

// yamlNumbers turns each decoded JSON number in v into a Go integer when
// it is whole. A float64 such as 1000000 reaches YAML as `1e+06`.
func yamlNumbers(v any) any {
	switch v := v.(type) {
	case map[string]any:
		for key, item := range v {
			v[key] = yamlNumbers(item)
		}
	case []any:
		for i, item := range v {
			v[i] = yamlNumbers(item)
		}
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return n
		}
		if n, err := strconv.ParseUint(v.String(), 10, 64); err == nil {
			return n
		}
		if f, err := v.Float64(); err == nil {
			return f
		}
		return v.String()
	}
	return v
}
