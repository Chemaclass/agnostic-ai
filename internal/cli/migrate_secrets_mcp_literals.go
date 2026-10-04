package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// secretsMCPLiteralsMigration marks each plain MCP `env` and `headers`
// value `!literal`, which sync drops, so the output stays the same. A
// value import's detector reads as a credential becomes a `${NAME}`
// reference instead, the one change to synced output the issue allows
// (#1753); the note names each variable to set.
var secretsMCPLiteralsMigration = specMigration{
	ID:      "secrets-mcp-literals",
	Group:   "secrets",
	Release: "0.79.0",
	Summary: "mark a plain MCP env or headers value !literal, and turn a credential into a ${NAME} reference",
	// The plan reads the project config and layers; global MCP specs
	// are not planned yet.
	ProjectOnly: true,
	Plan:        planSecretsMCPLiterals,
	Note:        secretsMCPLiteralsNote,
}

// mcpLiteralsPlan is the migration's plan plus the variables its
// references read.
type mcpLiteralsPlan struct {
	changes   []migrationChange
	skips     []migrationSkip
	variables []string
}

// mcpLiteralsFile is one spec file's unmarked values.
type mcpLiteralsFile struct {
	entry    spec.Entry
	literals []mcpUnmarked
	// credential maps the index of each literal the detector reads as a
	// credential to its place in the shared credential list.
	credential map[int]int
}

func planSecretsMCPLiterals(s migrationScope) ([]migrationChange, []migrationSkip, error) {
	p, err := planMCPLiterals(s.root)
	return p.changes, p.skips, err
}

func secretsMCPLiteralsNote(s migrationScope) string {
	p, err := planMCPLiterals(s.root)
	if err != nil || len(p.variables) == 0 {
		return ""
	}
	return "the rewritten credentials read a variable now; set " + andList(p.variables) + " in the shell that starts your tools"
}

// planMCPLiterals reads every MCP spec file of the project and its
// local/ layer on its own, so a local/ spec that extends a shared one
// marks only the values it sets. Variable names follow import: an `env`
// value reads its key, a header reads `<SERVER>_<HEADER>` and keeps a
// `Bearer ` prefix, and a name already in use gets the server prefix.
func planMCPLiterals(root string) (mcpLiteralsPlan, error) {
	var plan mcpLiteralsPlan
	cfg, _, err := loadProject(root)
	if err != nil {
		return plan, err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return plan, err
	}
	referenced := map[string]bool{}
	var files []mcpLiteralsFile
	var credentials []mcpLiteral
	for _, layer := range resolveLayers(root, cfg) {
		lb, err := spec.LoadLayered([]spec.Layer{layer})
		if err != nil {
			return plan, err
		}
		for _, e := range lb.MCPs {
			recordMCPReferences(e.Meta, referenced)
			literals := mcpUnmarkedLiterals(e)
			if len(literals) == 0 {
				continue
			}
			if pack, ok := strings.CutPrefix(layer.Name, layerNamePackPrefix); ok {
				plan.skips = append(plan.skips, packSkip(e.Path, pack))
				continue
			}
			if real, err := filepath.EvalSymlinks(e.Path); err != nil || !pathWithin(realRoot, real) {
				plan.skips = append(plan.skips, migrationSkip{Path: e.Path, Reason: "resolves outside the project"})
				continue
			}
			f := mcpLiteralsFile{entry: e, credential: map[int]int{}}
			for _, l := range literals {
				if !mcpCredentialSetting(l.key, l.value) {
					// A credential name with a value of no credential
					// shape, such as `API_KEY: sk-live-abc`, may still be
					// one, so the user decides.
					if mcpCredentialKey(l.key) {
						plan.skips = append(plan.skips, migrationSkip{Path: e.Path, Actionable: true,
							Reason: l.field + " " + l.key + " has a credential name; write a ${NAME} reference, or mark it !literal by hand"})
						continue
					}
					f.literals = append(f.literals, l)
					continue
				}
				if spec.HasEnvRef(l.value) {
					plan.skips = append(plan.skips, migrationSkip{Path: e.Path, Actionable: true,
						Reason: l.field + " " + l.key + " holds a credential around a reference; move it into one ${NAME} by hand"})
					continue
				}
				c := mcpLiteral{server: e.Name, field: l.field, key: l.key, secret: l.value}
				if token, ok := strings.CutPrefix(l.value, "Bearer "); ok && l.field == "headers" && token != "" {
					c.prefix, c.secret = "Bearer ", token
				}
				f.credential[len(f.literals)] = len(credentials)
				f.literals = append(f.literals, l)
				credentials = append(credentials, c)
			}
			files = append(files, f)
		}
	}
	names := mcpLiteralNames(credentials, referenced)
	for _, f := range files {
		var edits []yamlValueEdit
		var variables []string
		for i, l := range f.literals {
			c, isCredential := f.credential[i]
			if !isCredential {
				edits = append(edits, yamlValueEdit{Field: l.field, Key: l.key, Tag: spec.LiteralTag})
				continue
			}
			edits = append(edits, yamlValueEdit{Field: l.field, Key: l.key, Value: credentials[c].prefix + spec.EnvRef(names[c])})
			variables = append(variables, names[c])
		}
		if len(edits) == 0 {
			continue
		}
		body, err := os.ReadFile(f.entry.Path)
		if err != nil {
			return plan, err
		}
		after, err := editNestedYAMLValues(string(body), edits)
		if err != nil {
			plan.skips = append(plan.skips, migrationSkip{Path: f.entry.Path, Actionable: true, Reason: "cannot rewrite in place: " + err.Error()})
			continue
		}
		plan.changes = append(plan.changes, migrationChange{Path: f.entry.Path, Before: string(body), After: after})
		for _, v := range variables {
			if !slices.Contains(plan.variables, v) {
				plan.variables = append(plan.variables, v)
			}
		}
	}
	return plan, nil
}

// recordMCPReferences adds each `${NAME}` in v, at any depth, to names.
func recordMCPReferences(v any, names map[string]bool) {
	switch v := v.(type) {
	case string:
		for _, t := range spec.EnvRefTokens(v) {
			if t.Known() {
				names[t.Name] = true
			}
		}
	case map[string]any:
		for _, x := range v {
			recordMCPReferences(x, names)
		}
	case []any:
		for _, x := range v {
			recordMCPReferences(x, names)
		}
	}
}
