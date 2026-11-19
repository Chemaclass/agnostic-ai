package approvals

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// secretHomeDirs are folders under home that hold credentials.
var secretHomeDirs = []string{
	".ssh", ".aws", ".gnupg", ".config/gh", ".docker", ".kube", ".azure",
	".config/gcloud", ".password-store", "Library/Keychains", ".netrc",
	".git-credentials", ".npmrc", ".pypirc",
}

// secretNames are file names that hold credentials wherever they sit.
var secretNames = []string{
	".env*", "*.pem", "*.key", "*.p12", "*.pfx", "*.keystore", "*.jks",
	"id_rsa*", "id_dsa*", "id_ecdsa*", "id_ed25519*", ".netrc", ".npmrc",
	".pypirc", ".git-credentials", "credentials", "credentials.json",
}

// secretSamples are .env names meant to be committed: they hold no
// real values.
var secretSamples = []string{".env.example", ".env.sample", ".env.template", ".env.dist"}

// secret reports whether abs names a credential file or folder.
func (g *gate) secret(abs string) bool {
	if under(abs, g.home) {
		rel, _ := filepath.Rel(g.home, abs)
		rel = filepath.ToSlash(rel)
		for _, dir := range secretHomeDirs {
			if strings.EqualFold(rel, dir) || hasFoldPrefix(rel, dir+"/") {
				return true
			}
		}
	}
	for segment := range strings.SplitSeq(filepath.ToSlash(abs), "/") {
		if secretName(segment) {
			return true
		}
	}
	return false
}

func hasFoldPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

func secretName(name string) bool {
	lower := strings.ToLower(name)
	if slices.Contains(secretSamples, lower) {
		return false
	}
	for _, pattern := range secretNames {
		if ok, _ := path.Match(pattern, lower); ok {
			return true
		}
	}
	return false
}

// searchGlobRule asks for a search tool's file glob that may match a
// credential file or reach outside the project. Search tools such as
// ripgrep let *, ? and [ match a leading dot, so the glob is matched
// that way against sample names, segment by segment and as a whole.
func (g *gate) searchGlobRule(glob, cwd string) string {
	if glob == "" {
		return ""
	}
	if strings.HasPrefix(glob, "!") {
		// A negated glob searches every other file.
		return RuleSecret
	}
	if pathLike(glob) {
		return RuleOutsideProject
	}
	if base := globBase(glob); base != "." {
		abs, ok := g.path(base, cwd)
		if !ok || !g.inside(abs) {
			return RuleOutsideProject
		}
	}
	re, err := dotGlobRegexp(strings.ToLower(glob))
	if err != nil {
		return RuleSecret
	}
	samples := []string{".env", ".env.local", ".envrc", "id_rsa", "id_ed25519", "x.pem", "x.key", "x.p12", "x.pfx", ".netrc", ".npmrc", ".pypirc", ".git-credentials", "credentials", "credentials.json"}
	// The last segment names the file; matched alone, a glob such as
	// src/.e* or **/id_* is caught whatever folders come before it.
	segments := strings.Split(strings.ToLower(glob), "/")
	last, err := dotGlobRegexp(segments[len(segments)-1])
	if err != nil {
		return RuleSecret
	}
	for _, sample := range samples {
		if re.MatchString(sample) || re.MatchString("dir/"+sample) || last.MatchString(sample) {
			return RuleSecret
		}
	}
	for segment := range strings.SplitSeq(glob, "/") {
		if secretName(segment) {
			return RuleSecret
		}
	}
	return ""
}

// dotGlobRegexp reads a glob as search tools do: * and ? stay inside a
// segment and match a leading dot, ** crosses segments, [...] is a
// class, and a glob with no slash matches a file name at any depth.
func dotGlobRegexp(glob string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	if !strings.Contains(glob, "/") {
		b.WriteString("(?:.*/)?")
	}
	for i := 0; i < len(glob); i++ {
		c := glob[i]
		switch {
		case strings.HasPrefix(glob[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(glob[i:], "**"):
			b.WriteString(".*")
			i++
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		case c == '[':
			end := strings.IndexByte(glob[i:], ']')
			if end < 0 {
				return nil, fmt.Errorf("open class in %q", glob)
			}
			class := glob[i+1 : i+end]
			negate := strings.HasPrefix(class, "!") || strings.HasPrefix(class, "^")
			if negate {
				class = class[1:]
			}
			class = strings.NewReplacer(`\`, `\\`, `[`, `\[`, `^`, `\^`).Replace(class)
			if negate {
				class = "^" + class
			}
			b.WriteString("[" + class + "]")
			i += end
		case c == '{' || c == '}':
			return nil, fmt.Errorf("braces in %q", glob)
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// fixedProtected are paths an edit always asks for: Git internals, CI,
// hook and tool settings, shell profiles, and agnostic-ai's own config.
var fixedProtected = spec.ProtectGroup{Paths: []string{
	"**/.git",
	".github/workflows", ".github/hooks", ".gitlab-ci.yml", ".circleci", ".buildkite",
	"azure-pipelines.yml", "Jenkinsfile",
	".github/actions", ".gitmodules", ".gitattributes",
	".husky", ".githooks", ".pre-commit-config.yaml", "lefthook.yml", "lefthook.yaml", ".lefthook.yml",
	".lefthook.yaml", "lefthook-local.yml", "lefthook-local.yaml", ".lefthook-local.yml", ".lefthook-local.yaml",
	".devcontainer", "mise.toml", ".mise.toml", ".config/mise",
	"**/.claude", "**/.codex", "**/.gemini", "**/.cursor", "**/.kiro", "**/.qwen", "**/.qoder", "**/.codebuddy",
	"**/.kimi", "**/.kilocode", "**/.augment", "**/.factory", "**/.devin", "**/.openhands", "**/.continue",
	"**/.junie", "**/.trae", "**/.goose", "**/.opencode", "**/.zed", "**/.idea", "**/.amp", "**/.roo",
	"**/.windsurf", "**/.crush", "**/.crush.json", "**/crush.json", "**/.mcp.json",
	"**/.vscode", "**/opencode.json", "**/opencode.jsonc",
	".agnostic-ai/hooks", ".agnostic-ai/settings",
	"**/agnostic-ai.yaml", "**/agnostic-ai.local.yaml",
	"**/.bashrc", "**/.bash_profile", "**/.bash_login", "**/.bash_logout", "**/.profile",
	"**/.zshrc", "**/.zshenv", "**/.zprofile", "**/.zlogin", "**/.zlogout",
	"**/.config/fish",
}}

// protected reports whether abs, inside the project, is a path an edit
// always asks for, or one under $AGNOSTIC_AI_HOME.
func (g *gate) protected(abs string) bool {
	if home := g.policy.AgnosticHome; home != "" && filepath.IsAbs(home) {
		if under(abs, filepath.Clean(home)) {
			return true
		}
		if g.policy.Resolve != nil && under(filepath.Clean(g.policy.Resolve(abs)), filepath.Clean(g.policy.Resolve(home))) {
			return true
		}
	}
	if g.protectedRel(g.relative(abs)) {
		return true
	}
	return g.policy.Resolve != nil && g.protectedRel(g.relative(filepath.Clean(g.policy.Resolve(abs))))
}

func (g *gate) protectedRel(rel string) bool {
	if rel == "" || rel == "." {
		return false
	}
	if _, ok := fixedProtected.Match(rel); ok {
		return true
	}
	for _, group := range g.policy.Protected {
		if _, ok := group.Match(rel); ok {
			return true
		}
	}
	return false
}

// ResolveLinks returns p with the symbolic links of its longest
// existing prefix resolved; the rest, which does not exist yet, is kept
// as written. A dangling link is followed to where it points. It is the
// Resolve a hook passes in a Policy.
func ResolveLinks(p string) string {
	return resolveLinks(filepath.Clean(p), 0)
}

// maxLinkHops bounds a chain of dangling links; past it the path is
// read as the file system root, which no project contains.
const maxLinkHops = 40

func resolveLinks(p string, hops int) string {
	if hops > maxLinkHops {
		return string(filepath.Separator)
	}
	var rest []string
	for dir := p; ; {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(append([]string{real}, rest...)...)
		}
		if info, err := os.Lstat(dir); err == nil && info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(dir)
			if err != nil {
				return string(filepath.Separator)
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(dir), target)
			}
			return resolveLinks(filepath.Join(append([]string{target}, rest...)...), hops+1)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return p
		}
		rest = append([]string{filepath.Base(dir)}, rest...)
		dir = parent
	}
}

// protectedBelow reports whether a protected path sits under the
// project folder rel, so a glob from rel may reach it.
func (g *gate) protectedBelow(rel string) bool {
	if rel == "" || rel == "." {
		return false
	}
	prefix := strings.ToLower(rel) + "/"
	groups := append([]spec.ProtectGroup{fixedProtected}, g.policy.Protected...)
	for _, group := range groups {
		for _, p := range group.Paths {
			if strings.HasPrefix(strings.ToLower(p), prefix) {
				return true
			}
		}
	}
	return false
}
