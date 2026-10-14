package ellipsis

import (
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// serverName is what Ellipsis tool names are built from, so a name is
// written only when it stays plain.
var serverName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// scriptExtensions mark an argument as a script file, which a sandbox
// reads from the checkout and never from the working directory of the
// machine that wrote the spec.
var scriptExtensions = []string{".js", ".mjs", ".cjs", ".ts", ".py", ".sh", ".rb", ".php", ".jar"}

// disabledReason explains why a disabled server is left out: Ellipsis has
// no switch, and a listed server is available in every session.
const disabledReason = "Ellipsis lists a server only to make it available and has no way to switch one off, so the server is left out"

// serverProblem is a server the file cannot hold as written.
type serverProblem struct {
	name, why string
}

// MCPLaunchView reads the same x-ellipsis overrides the server builder
// does, so a reference is checked where it is written.
func (Adapter) MCPLaunchView() emit.MCPLaunchView {
	return emit.MCPLaunchView{Resolved: true}
}

// buildServers renders the `mcp_servers` entries of the environment file,
// sorted by name. A server Ellipsis cannot run as written is returned as
// a problem and left out, never weakened. Ellipsis reads `${NAME}` in
// `headers` and `env` only, and sync has already rewritten each reference
// to that form or left the server out.
func buildServers(mcps []spec.Entry, repo string) ([]*yaml.Node, []serverProblem) {
	mcps = emit.DropMCPDisabled(target, mcps, disabledReason)
	slices.SortFunc(mcps, func(a, b spec.Entry) int { return strings.Compare(a.Name, b.Name) })
	var servers []*yaml.Node
	var problems []serverProblem
	var descriptions, timeouts, roots int
	for _, e := range mcps {
		meta := emit.ResolveMeta(e.Meta, target)
		node, why := buildServer(e.Name, meta, repo)
		if why == "" {
			why = overrideLiteral(e.Meta)
		}
		if why != "" {
			problems = append(problems, serverProblem{e.Name, why})
			emit.NoteEntryOmitted(target, spec.KindMCP, e.Name)
			continue
		}
		servers = append(servers, node)
		if _, ok := meta["description"]; ok {
			descriptions++
		}
		if _, ok := meta["timeout"]; ok {
			timeouts++
		}
		if len(emit.BuildRoots(meta)) > 0 {
			roots++
		}
	}
	emit.NoteFieldNoOp(target, spec.KindMCP, "description", descriptions, "an Ellipsis server entry has no description key")
	emit.NoteFieldNoOp(target, spec.KindMCP, "timeout", timeouts, "an Ellipsis server entry has no timeout key")
	emit.NoteFieldNoOp(target, spec.KindMCP, "roots", roots, "an Ellipsis server entry has no roots key")
	return servers, problems
}

// schemeWord is the one word a header value may put before a reference,
// as in `Bearer ${TOKEN}`.
var schemeWord = regexp.MustCompile(`^[A-Za-z]+\s+`)

// overrideLiteral says why an `x-ellipsis` override cannot be written when
// its `env` or `headers` holds a value that is no reference. The `!literal`
// mark that tells a plain setting from a secret exists only on the
// top-level maps, so a literal here could be a token in a committed file.
// It names the key, never the value.
func overrideLiteral(meta map[string]any) string {
	x, _ := meta[emit.XPrefix+target].(map[string]any)
	for _, field := range []string{"env", "headers"} {
		values, _ := x[field].(map[string]any)
		for _, key := range slices.Sorted(maps.Keys(values)) {
			value, ok := values[key].(string)
			if !ok || value == "" {
				continue
			}
			if rest := schemeWord.ReplaceAllString(value, ""); !spec.OnlyEnvRefs(rest) && !spec.OnlyEscapedEnvRefs(rest) {
				return fmt.Sprintf("has a literal value for %s %s under x-ellipsis, which cannot carry the !literal mark; write a ${NAME} reference, or set a plain value in the top-level %s with !literal", field, key, field)
			}
		}
	}
	return ""
}

// buildServer renders one server, or says why it cannot be written.
func buildServer(name string, meta map[string]any, repo string) (*yaml.Node, string) {
	switch {
	case name == "":
		return nil, "has no name"
	case slices.Contains(integrations, name):
		return nil, fmt.Sprintf("is named %s, which Ellipsis reads as the connected %s integration; rename the server, or list the integration in x-ellipsis.environment.mcp_servers", name, name)
	case !serverName.MatchString(name):
		return nil, "has a name Ellipsis tool names cannot carry; use letters, digits, - and _"
	}
	if v, ok := meta["oauth"]; ok && v != nil && v != false {
		return nil, "uses oauth, and an Ellipsis server entry has no oauth or login settings; the connection would fail"
	}
	remote, why := isRemote(meta)
	if why != "" {
		return nil, why
	}
	if remote {
		return remoteServer(name, meta)
	}
	return stdioServer(name, meta, repo)
}

// isRemote reads the transport. Ellipsis connects to a `url` over
// streamable HTTP; a `command` runs in the sandbox.
func isRemote(meta map[string]any) (bool, string) {
	transport, _ := meta["type"].(string)
	switch transport {
	case "http":
		return true, ""
	case "stdio", "local":
		return false, ""
	case "":
		_, hasCommand := meta["command"]
		return !hasCommand && meta["url"] != nil, ""
	case "sse", "ws":
		return false, fmt.Sprintf("uses the %s transport, and Ellipsis connects to remote servers over streamable HTTP only; use type: http, or keep this server to the tools that read it", transport)
	}
	return false, fmt.Sprintf("uses the transport %q, which Ellipsis does not read; use type: http or stdio", transport)
}

func remoteServer(name string, meta map[string]any) (*yaml.Node, string) {
	url, _ := meta["url"].(string)
	if url == "" {
		return nil, "is a remote server with no url"
	}
	node := mapping()
	add(node, "name", textNode(name))
	add(node, "url", textNode(url))
	if headers := emit.StringMap(meta["headers"]); len(headers) > 0 {
		add(node, "headers", orderedMap(stringsAsAny(headers), nil))
	}
	return node, ""
}

func stdioServer(name string, meta map[string]any, repo string) (*yaml.Node, string) {
	command, _ := meta["command"].(string)
	if command == "" {
		return nil, "is a stdio server with no command"
	}
	if cwd, ok := meta["cwd"]; ok && cwd != nil && cwd != "" {
		return nil, fmt.Sprintf("sets cwd, and an Ellipsis server entry has no working directory; write the script path as %s%s/<path> instead", sandboxRoot, repo)
	}
	args := emit.StringSlice(meta["args"])
	for _, value := range append([]string{command}, args...) {
		if why := sandboxPathProblem(value, repo); why != "" {
			return nil, why
		}
	}
	node := mapping()
	add(node, "name", textNode(name))
	add(node, "command", textNode(command))
	if len(args) > 0 {
		list := &yaml.Node{Kind: yaml.SequenceNode}
		for _, a := range args {
			list.Content = append(list.Content, textNode(a))
		}
		add(node, "args", list)
	}
	if env := emit.StringMap(meta["env"]); len(env) > 0 {
		add(node, "env", orderedMap(stringsAsAny(env), nil))
	}
	return node, ""
}

func stringsAsAny(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// sandboxPathProblem checks a command or argument that names a file. The
// server runs inside the sandbox, where the one selected repository is at
// /sandbox/<repo>: a path from the machine that wrote the spec, a path
// relative to an unknown directory, or another checkout would not resolve.
func sandboxPathProblem(value, repo string) string {
	checkout := sandboxRoot + repo + "/"
	// `--flag=/path` carries the path after the equals sign.
	if flag, rest, ok := strings.Cut(value, "="); ok && strings.HasPrefix(flag, "--") {
		value = rest
	}
	switch {
	case strings.HasPrefix(value, sandboxRoot):
		segments := strings.Split(strings.TrimPrefix(value, sandboxRoot), "/")
		switch {
		case slices.Contains(segments, ".."):
			return fmt.Sprintf("uses %s, which climbs out of %s with ..; keep paths inside it", value, checkout)
		case segments[0] != repo:
			return fmt.Sprintf("uses %s, a checkout that is not there: only %s is checked out", value, checkout)
		}
	case looksLikeLocalPath(value):
		return fmt.Sprintf("uses %q, a path that only exists on the machine that wrote the spec; Ellipsis runs the server in a sandbox, so write %s<path>", value, checkout)
	}
	return ""
}

var windowsPath = regexp.MustCompile(`^[A-Za-z]:[\\/]|^\\\\`)

func looksLikeLocalPath(value string) bool {
	switch {
	case strings.HasPrefix(value, "./"), strings.HasPrefix(value, "../"), strings.HasPrefix(value, ".\\"), strings.HasPrefix(value, "..\\"),
		strings.HasPrefix(value, "~"), strings.HasPrefix(value, "file:"), strings.Contains(value, "${"),
		strings.HasPrefix(value, "/Users/"), strings.HasPrefix(value, "/home/"), strings.HasPrefix(value, "/root/"),
		strings.HasPrefix(value, "/Volumes/"), strings.HasPrefix(value, "/private/"):
		return true
	case windowsPath.MatchString(value):
		return true
	case strings.HasPrefix(value, "/") || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "@") || strings.Contains(value, "://"):
		return false
	}
	return slices.Contains(scriptExtensions, strings.ToLower(path.Ext(value)))
}

// reportOmitted says which servers were left out. Under
// `on-unsupported: error` the first one stops the sync, since the file
// would not match the specs.
func reportOmitted(omitted []serverProblem, mode string) error {
	for _, p := range omitted {
		switch mode {
		case emit.OnUnsupportedSilent:
		case emit.OnUnsupportedError:
			return fmt.Errorf("ellipsis: MCP server %q cannot be written to the environment file: %s", p.name, p.why)
		default:
			emit.NoteProject(fmt.Sprintf("ellipsis: MCP server %q left out of the environment file: %s", p.name, p.why))
		}
	}
	return nil
}
