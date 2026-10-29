package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/preview"
)

// ellipsisEnvironmentDir holds one file per environment
// (ellipsis.dev/docs/environments/schema).
const ellipsisEnvironmentDir = ".ellipsis/environments"

const (
	ellipsisEnvironmentSpecName = "ellipsis"
	ellipsisSandboxRoot         = "/sandbox/"
)

var (
	ellipsisEnvironmentTopKeys = []string{"ellipsis", "repositories", "variables", "hooks", "compute", "mcp_servers"}
	ellipsisServerKeys         = []string{"name", "url", "headers", "command", "args", "env"}
	ellipsisHookNames          = []string{"build_base", "after_checkout", "before_start", "validate"}
	ellipsisIntegrations       = []string{"slack", "linear"}
	ellipsisNamePattern        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
	ellipsisRepoPattern        = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

// ellipsisEnvironmentImport is a native environment file split into the
// specs that carry it.
type ellipsisEnvironmentImport struct {
	install, setup string
	block          map[string]any
	servers        map[string]any
}

// importEllipsisEnvironment reads a hand-written environment file into an
// environment spec, an `x-ellipsis.environment` settings spec, and MCP
// specs, reversing the ellipsis emit:
//
//   - a `build_base` or `after_checkout` script that starts with the
//     `cd /sandbox/<repo>` line sync writes becomes `install` or `setup`;
//   - every other script, the cache inputs, `before_start`, `validate`,
//     variables, compute, and the connected integrations stay in
//     `x-ellipsis` as written, so no behavior is rewritten;
//   - each MCP server becomes a spec scoped to Ellipsis.
//
// A file it cannot express, such as more than one repository or a field the
// schema does not list, is left as written with a note, because sync would
// drop what the specs cannot hold. A file with the provenance header
// imports nothing, since its specs are the source.
func importEllipsisEnvironment(root string, src config.Sources, cfg *config.Config, plan ellipsisEnvironmentPlan) error {
	if plan.leave != "" {
		summaryf("%s", plan.leave)
		return nil
	}
	if plan.imported == nil {
		return nil
	}
	rel, imported := plan.rel, plan.imported
	if err := mkdirAllSources(root, src.Environments, src.Settings, src.MCPs); err != nil {
		return err
	}
	envs, settings, mcps := 0, 0, 0
	if imported.install != "" || imported.setup != "" {
		wrote, err := writeEllipsisEnvironmentSpec(importSourcePath(root, src.Environments), imported)
		if err != nil {
			return err
		}
		envs = wrote
	}
	raw, err := yaml.Marshal(map[string]any{"x-ellipsis": map[string]any{"environment": imported.block}})
	if err != nil {
		return fmt.Errorf("marshal settings from %s: %w", rel, err)
	}
	dst := filepath.Join(importSourcePath(root, src.Settings), "ellipsis-environment.yaml")
	if err := importWriteFile(dst, raw, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	settings = 1
	if len(imported.servers) > 0 {
		mcps, err = writeEllipsisMCPSpecs(imported.servers, importSourcePath(root, src.MCPs))
		if err != nil {
			return err
		}
	}
	if name, _ := imported.block["name"].(string); name != "" && !pathIsConfigured(cfg) && path.Base(rel) != name+".yaml" {
		summaryf("  ! sync writes this environment to %s/%s.yaml; delete %s after import, or set outputs.ellipsis.environment-file to it, so Ellipsis does not see two files named %s\n",
			ellipsisEnvironmentDir, name, rel, name)
	}
	summaryf("imported %d environments, %d settings, %d mcps\n", envs, settings, mcps)
	return nil
}

// ellipsisEnvironmentPlan is the environment file import would write, or
// the note that says why it leaves the files as written.
type ellipsisEnvironmentPlan struct {
	rel      string
	imported *ellipsisEnvironmentImport
	leave    string
}

// planEllipsisEnvironment reads the hand-written environment file before
// anything is written, so the review import knows whether it selects one.
func planEllipsisEnvironment(root string, cfg *config.Config) (ellipsisEnvironmentPlan, error) {
	candidates, err := ellipsisEnvironmentFiles(root, cfg)
	if err != nil {
		return ellipsisEnvironmentPlan{}, err
	}
	if len(candidates) == 0 {
		return ellipsisEnvironmentPlan{}, nil
	}
	if len(candidates) > 1 {
		return ellipsisEnvironmentPlan{leave: fmt.Sprintf("  ! left %s as written: %d environment files, and agnostic-ai writes one; keep one and import again, or list the files under sync.unmanaged\n",
			ellipsisEnvironmentDir, len(candidates))}, nil
	}
	rel, doc := candidates[0].path, candidates[0].doc
	imported, problem := splitEllipsisEnvironment(doc)
	if problem == "" {
		problem = ellipsisLaunchCredential(imported.servers)
	}
	if problem != "" {
		return ellipsisEnvironmentPlan{leave: fmt.Sprintf("  ! left %s as written: %s, which the specs cannot hold, so sync would drop it\n", rel, problem)}, nil
	}
	return ellipsisEnvironmentPlan{rel: rel, imported: imported}, nil
}

func pathIsConfigured(cfg *config.Config) bool {
	o, ok := cfg.Outputs["ellipsis"]
	return ok && o.EnvironmentFile != ""
}

type nativeEllipsisEnvironment struct {
	path string
	doc  map[string]any
}

// ellipsisEnvironmentFiles returns the hand-written environment files:
// the configured one, or every YAML file in the environments folder. A
// file sync wrote, or one of another kind, is not a candidate.
func ellipsisEnvironmentFiles(root string, cfg *config.Config) ([]nativeEllipsisEnvironment, error) {
	var rels []string
	if o, ok := cfg.Outputs["ellipsis"]; ok && o.EnvironmentFile != "" {
		rels = []string{filepath.ToSlash(o.EnvironmentFile)}
	} else {
		for _, ext := range []string{"yaml", "yml"} {
			matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(ellipsisEnvironmentDir), "*."+ext))
			if err != nil {
				return nil, err
			}
			for _, m := range matches {
				rel, err := filepath.Rel(root, m)
				if err != nil {
					return nil, err
				}
				rels = append(rels, filepath.ToSlash(rel))
			}
		}
	}
	slices.Sort(rels)
	var out []nativeEllipsisEnvironment
	for _, rel := range rels {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}
		text := strings.ReplaceAll(strings.TrimPrefix(string(data), "\U0000FEFF"), "\r\n", "\n")
		if header.Has(text) {
			continue
		}
		var doc map[string]any
		if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
			return nil, fmt.Errorf("%s is not valid YAML: %w", rel, err)
		}
		if top, _ := doc["ellipsis"].(map[string]any); top["kind"] != "environment" {
			continue
		}
		out = append(out, nativeEllipsisEnvironment{path: rel, doc: doc})
	}
	return out, nil
}

func writeEllipsisEnvironmentSpec(dir string, imported *ellipsisEnvironmentImport) (int, error) {
	spec := yaml.Node{Kind: yaml.MappingNode}
	addYAMLField(&spec, "name", ellipsisEnvironmentSpecName)
	addYAMLField(&spec, "targets", importedTargetsNode("ellipsis"))
	if imported.install != "" {
		addYAMLField(&spec, "install", imported.install)
	}
	if imported.setup != "" {
		addYAMLField(&spec, "setup", imported.setup)
	}
	raw, err := yaml.Marshal(&spec)
	if err != nil {
		return 0, fmt.Errorf("marshal environment spec: %w", err)
	}
	if err := importMkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("mkdir %s: %w", dir, err)
	}
	out := filepath.Join(dir, ellipsisEnvironmentSpecName+".yaml")
	if err := importWriteFile(out, raw, 0o644); err != nil {
		return 0, fmt.Errorf("write %s: %w", out, err)
	}
	return 1, nil
}

// splitEllipsisEnvironment divides a native file, or returns why the specs
// cannot express it.
func splitEllipsisEnvironment(doc map[string]any) (*ellipsisEnvironmentImport, string) {
	for _, key := range slices.Sorted(maps.Keys(doc)) {
		if !slices.Contains(ellipsisEnvironmentTopKeys, key) {
			return nil, fmt.Sprintf("it sets %q", key)
		}
	}
	imported := &ellipsisEnvironmentImport{block: map[string]any{}, servers: map[string]any{}}
	top, _ := doc["ellipsis"].(map[string]any)
	for _, key := range slices.Sorted(maps.Keys(top)) {
		switch key {
		case "kind":
		case "name":
		case "description", "metadata":
			imported.block[key] = top[key]
		default:
			return nil, fmt.Sprintf("its ellipsis block sets %q", key)
		}
	}
	name, _ := top["name"].(string)
	if !ellipsisNamePattern.MatchString(name) {
		return nil, "it has no usable ellipsis.name"
	}
	imported.block["name"] = name

	repo, problem := ellipsisRepository(doc["repositories"])
	if problem != "" {
		return nil, problem
	}
	imported.block["repository"] = repo
	repoName, _ := repo["name"].(string)

	if problem := ellipsisVariables(doc["variables"], imported.block); problem != "" {
		return nil, problem
	}
	if compute, ok := doc["compute"].(map[string]any); ok {
		for _, key := range slices.Sorted(maps.Keys(compute)) {
			if key != "cpu" && key != "memory" && key != "timeout" {
				return nil, fmt.Sprintf("its compute sets %q", key)
			}
		}
		imported.block["compute"] = compute
	} else if _, set := doc["compute"]; set {
		return nil, "its compute is not a mapping"
	}
	if problem := ellipsisHooks(doc["hooks"], repoName, imported); problem != "" {
		return nil, problem
	}
	if problem := ellipsisServers(doc["mcp_servers"], imported); problem != "" {
		return nil, problem
	}
	return imported, ""
}

func ellipsisRepository(raw any) (map[string]any, string) {
	list, _ := raw.([]any)
	if len(list) != 1 {
		return nil, fmt.Sprintf("it lists %d repositories, and this target checks out one", len(list))
	}
	repo, _ := list[0].(map[string]any)
	for _, key := range slices.Sorted(maps.Keys(repo)) {
		if key != "name" && key != "owner" && key != "ref" {
			return nil, fmt.Sprintf("its repository sets %q", key)
		}
	}
	name, _ := repo["name"].(string)
	if name == "." || name == ".." || !ellipsisRepoPattern.MatchString(name) {
		return nil, "its repository has no usable name"
	}
	return repo, ""
}

func ellipsisVariables(raw any, block map[string]any) string {
	if raw == nil {
		return ""
	}
	list, ok := raw.([]any)
	if !ok {
		return "its variables are not a list"
	}
	for _, item := range list {
		v, _ := item.(map[string]any)
		for _, key := range slices.Sorted(maps.Keys(v)) {
			if key != "name" && key != "value" {
				return fmt.Sprintf("a variable sets %q", key)
			}
		}
		name, _ := v["name"].(string)
		if value, _ := v["value"].(string); value != "" && (preview.CredentialKey(name) || preview.CredentialDetected(value)) {
			return fmt.Sprintf("variable %s holds a credential value, and a spec must not carry it: remove the value so Ellipsis reads the stored secret", name)
		}
	}
	block["variables"] = list
	return ""
}

// ellipsisHooks sends each hook to the field that keeps its behavior: a
// script that starts in the checkout becomes `install` or `setup`, any
// other stays in x-ellipsis as written.
func ellipsisHooks(raw any, repo string, imported *ellipsisEnvironmentImport) string {
	if raw == nil {
		return ""
	}
	hooks, ok := raw.(map[string]any)
	if !ok {
		return "its hooks are not a mapping"
	}
	native := map[string]any{}
	for _, name := range slices.Sorted(maps.Keys(hooks)) {
		if !slices.Contains(ellipsisHookNames, name) {
			return fmt.Sprintf("it has the hook %q", name)
		}
		script, inputs, hasInputs, problem := ellipsisHook(hooks[name], name == "build_base")
		if problem != "" {
			return fmt.Sprintf("hook %s %s", name, problem)
		}
		portable, isPortable := "", false
		if name == "build_base" || name == "after_checkout" {
			portable, isPortable = checkoutScript(script, repo)
		}
		switch name {
		case "build_base":
			entry := map[string]any{}
			if isPortable {
				imported.install = portable
			} else {
				entry["run"] = script
			}
			if hasInputs {
				entry["inputs"] = inputs
			}
			if len(entry) > 0 {
				native[name] = entry
			}
		case "after_checkout":
			if isPortable {
				imported.setup = portable
			} else {
				native[name] = script
			}
		default:
			native[name] = script
		}
	}
	if len(native) > 0 {
		imported.block["hooks"] = native
	}
	return ""
}

// ellipsisHook reads a hook written as a script or as {run, inputs}.
func ellipsisHook(raw any, takesInputs bool) (script string, inputs []any, hasInputs bool, problem string) {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return "", nil, false, "is empty"
		}
		return v, nil, false, ""
	case map[string]any:
		for _, key := range slices.Sorted(maps.Keys(v)) {
			if key != "run" && (key != "inputs" || !takesInputs) {
				return "", nil, false, fmt.Sprintf("sets %q", key)
			}
		}
		script, _ = v["run"].(string)
		if script == "" {
			return "", nil, false, "has no run script"
		}
		if list, ok := v["inputs"].([]any); ok {
			return script, list, true, ""
		} else if _, set := v["inputs"]; set {
			return "", nil, false, "has inputs that are not a list"
		}
		return script, nil, false, ""
	}
	return "", nil, false, "is not a script"
}

// checkoutScript returns script without the `cd /sandbox/<repo>` first
// line that sync writes, when it starts with exactly that line.
func checkoutScript(script, repo string) (string, bool) {
	rest, ok := strings.CutPrefix(script, "cd "+ellipsisSandboxRoot+repo+"\n")
	rest = strings.TrimRight(rest, "\n")
	if !ok || strings.TrimSpace(rest) == "" {
		return "", false
	}
	return rest, true
}

// ellipsisServers sends each integration to x-ellipsis and each server to
// an MCP spec.
func ellipsisServers(raw any, imported *ellipsisEnvironmentImport) string {
	if raw == nil {
		return ""
	}
	list, ok := raw.([]any)
	if !ok {
		return "its mcp_servers are not a list"
	}
	var named []any
	for _, item := range list {
		switch v := item.(type) {
		case string:
			if !slices.Contains(ellipsisIntegrations, v) {
				return fmt.Sprintf("it lists the integration %q", v)
			}
			named = append(named, v)
		case map[string]any:
			name, problem := ellipsisServer(v)
			if problem != "" {
				return problem
			}
			if _, dup := imported.servers[name]; dup {
				return fmt.Sprintf("server %s appears twice", name)
			}
			server := maps.Clone(v)
			delete(server, "name")
			server["targets"] = []any{"ellipsis"}
			imported.servers[name] = server
		default:
			return "an mcp_servers entry is not a name or a mapping"
		}
	}
	if len(named) > 0 {
		imported.block["mcp_servers"] = named
	}
	return ""
}

func ellipsisServer(v map[string]any) (string, string) {
	name, _ := v["name"].(string)
	if !ellipsisNamePattern.MatchString(name) || slices.Contains(ellipsisIntegrations, name) {
		return "", fmt.Sprintf("an mcp server has the name %q, which a spec cannot carry", name)
	}
	for _, key := range slices.Sorted(maps.Keys(v)) {
		if !slices.Contains(ellipsisServerKeys, key) {
			return "", fmt.Sprintf("server %s sets %q", name, key)
		}
	}
	_, hasURL := v["url"]
	_, hasCommand := v["command"]
	if hasURL == hasCommand {
		return "", fmt.Sprintf("server %s needs exactly one of url and command", name)
	}
	return name, ""
}

// ellipsisLaunchCredential says why a server cannot be imported when a
// credential sits in its `url` or `args`: a spec would hold a reference
// there, and Ellipsis reads references in `env` and `headers` only, so sync
// would leave the server out.
func ellipsisLaunchCredential(servers map[string]any) string {
	probe, _ := jsonRoundTrip(servers).(map[string]any)
	for _, r := range referenceMCPLiterals(probe) {
		if r.leftOut || (r.field != "env" && r.field != "headers") {
			return fmt.Sprintf("server %s holds a credential in its %s, and Ellipsis reads references in env and headers only", r.server, r.field)
		}
	}
	return ""
}

// writeEllipsisMCPSpecs writes one MCP spec per server. A credential in
// `env` or `headers` becomes a reference to a secret stored in Ellipsis. A
// plain setting stays as written and is marked `!literal`, because Ellipsis
// reads every `${NAME}` as a stored secret and "a missing secret fails the
// session".
func writeEllipsisMCPSpecs(servers map[string]any, dstDir string) (int, error) {
	for _, raw := range servers {
		if server, ok := raw.(map[string]any); ok {
			adapters.EscapeMCPLiterals("ellipsis", server)
			adapters.ReadMCPEnvRefs("ellipsis", server)
		}
	}
	plain := setAsidePlainValues(servers)
	refs := referenceMCPLiterals(servers)
	for name, fields := range plain {
		server, ok := servers[name].(map[string]any)
		if !ok {
			continue
		}
		for field, values := range fields {
			held, _ := server[field].(map[string]any)
			if held == nil {
				held = map[string]any{}
				server[field] = held
			}
			maps.Copy(held, values)
		}
	}
	count, err := writeMCPSpecs(servers, dstDir)
	if err != nil {
		return count, err
	}
	reportMCPLiteralRefsWithHint(refs, "import does not copy a credential into a spec; store each secret above in Ellipsis under that name, since Ellipsis reads ${NAME} from its stored secrets")
	return count, nil
}

// setAsidePlainValues removes each `env` and `headers` value that is no
// credential from servers and returns it by server, field, and key, so the
// reference rewrite leaves it alone.
func setAsidePlainValues(servers map[string]any) map[string]map[string]map[string]any {
	plain := map[string]map[string]map[string]any{}
	for name, raw := range servers {
		server, _ := raw.(map[string]any)
		for _, field := range mcpSecretFields {
			values, _ := server[field].(map[string]any)
			for key, v := range values {
				value, _ := v.(string)
				if value == "" || strings.Contains(value, "${") || preview.CredentialKey(key) || preview.CredentialDetected(value) {
					continue
				}
				if plain[name] == nil {
					plain[name] = map[string]map[string]any{}
				}
				if plain[name][field] == nil {
					plain[name][field] = map[string]any{}
				}
				plain[name][field][key] = value
				delete(values, key)
			}
		}
	}
	return plain
}
