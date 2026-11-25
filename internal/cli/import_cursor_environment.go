package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// cursorEnvironmentFile is where Cursor Cloud reads its environment
// config (cursor.com/docs/cloud-agent).
var cursorEnvironmentFile = filepath.Join(".cursor", "environment.json")

// cursorEnvironmentSpecName names the environment spec an environment.json
// imports into.
const cursorEnvironmentSpecName = "cursor"

// cursorEnvironmentReservedKeys are the environment spec fields other
// outputs or the loader read for themselves. The cursor emit never
// writes them to environment.json, so a file that sets one would lose it
// on the next sync.
var cursorEnvironmentReservedKeys = []string{
	"name", "scope", "description", "setup", "setup-windows", "cleanup", "dev-commands",
	"setup-worktree", "setup-worktree-unix", "setup-worktree-windows",
	"target", "targets", "target-exclude", "targets-exclude",
	spec.EnvironmentVariablesField, spec.EnvironmentAllowedDomainsField,
}

// importCursorEnvironment reads a hand-written .cursor/environment.json
// into an environment spec that passes every key through, the reverse of
// the cursor emit. `//` comments become YAML comments. A file sync wrote,
// or a spec already at the destination, is left alone.
func importCursorEnvironment(root string, src config.Sources) (int, error) {
	if src.Environments == "" {
		return 0, nil
	}
	for _, p := range readStateFile(root).Outputs {
		if filepath.Clean(filepath.FromSlash(p)) == cursorEnvironmentFile {
			return 0, nil
		}
	}
	path := filepath.Join(root, cursorEnvironmentFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	doc, err := jsoncObjectToYAML(data)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(doc.Content) == 0 {
		return 0, nil
	}
	for i := 0; i < len(doc.Content); i += 2 {
		key := doc.Content[i].Value
		if strings.HasPrefix(key, "x-") || slices.Contains(cursorEnvironmentReservedKeys, key) {
			summaryf("  ! left %s as written: it sets %q, which an environment spec reads for itself, so sync would drop it\n",
				filepath.ToSlash(cursorEnvironmentFile), key)
			return 0, nil
		}
	}

	dstDir := importSourcePath(root, src.Environments)
	out := filepath.Join(dstDir, cursorEnvironmentSpecName+".yaml")
	if fileExists(out) {
		summaryf("  ! skipped %s: %s already exists; move its keys there by hand\n",
			filepath.ToSlash(cursorEnvironmentFile), filepath.ToSlash(src.Environments)+"/"+cursorEnvironmentSpecName+".yaml")
		return 0, nil
	}
	dropGeneratedCursorAllowlist(root, doc)
	if len(doc.Content) == 0 {
		return 0, nil
	}
	name := yaml.Node{Kind: yaml.ScalarNode, Value: cursorEnvironmentSpecName}
	head := []*yaml.Node{{Kind: yaml.ScalarNode, Value: "name"}, &name}
	if scopeImportedEnvironment(root, "cursor") {
		head = append(head, &yaml.Node{Kind: yaml.ScalarNode, Value: "targets"}, importedTargetsNode("cursor"))
	}
	doc.Content = append(head, doc.Content...)
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return 0, fmt.Errorf("marshal %s: %w", out, err)
	}
	if err := importMkdirAll(dstDir, 0o755); err != nil {
		return 0, fmt.Errorf("mkdir %s: %w", dstDir, err)
	}
	if err := importWriteFile(out, raw, 0o644); err != nil {
		return 0, fmt.Errorf("write %s: %w", out, err)
	}
	return 1, nil
}

func dropGeneratedCursorAllowlist(root string, doc *yaml.Node) {
	for i := 0; i < len(doc.Content); i += 2 {
		if doc.Content[i].Value != "egressAllowlist" {
			continue
		}
		var native []string
		if err := doc.Content[i+1].Decode(&native); err != nil {
			return
		}
		_, bundle, err := loadProject(root)
		if err != nil {
			return
		}
		meta := resolvedCloudRequirementMeta(bundle.Environments, "cursor")
		if _, explicit := meta["egressAllowlist"]; explicit {
			return
		}
		requirements, _ := spec.ReadEnvironmentRequirements(meta)
		if requirements.AllowedDomains != nil && slices.Equal(native, requirements.AllowedDomains) {
			doc.Content = slices.Delete(doc.Content, i, i+2)
		}
		return
	}
}
