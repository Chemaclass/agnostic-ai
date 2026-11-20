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

// secret reports whether abs names a credential file or folder, as
// written or with its links resolved.
func (g *gate) secret(abs string) bool {
	if g.secretPath(abs) {
		return true
	}
	return g.policy.Resolve != nil && g.secretPath(filepath.Clean(g.policy.Resolve(abs)))
}

func (g *gate) secretPath(abs string) bool {
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
	if strings.HasPrefix(glob, "!") || strings.ContainsAny(glob, " ,") {
		// A negated glob searches every other file; a space or comma may
		// split one glob into several.
		return RuleSecret
	}
	// A backslash escapes the next rune; dropping it only widens the glob.
	glob = strings.ReplaceAll(glob, `\`, "")
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
	if gitGlob(glob) {
		return RuleProtectedPath
	}
	return ""
}

// gitGlob reports a search glob that may match a Git folder or config
// file, read the way the search tool reads it.
var gitSamples = []string{".git/config", ".git/head", ".git/index", ".git/hooks/x", ".git/fetch_head", ".git/packed-refs",
	".git/modules/x/config", ".git/worktrees/x/config.worktree", ".gitconfig", ".config/git/config"}

func gitGlob(glob string) bool {
	lower := strings.ToLower(glob)
	for segment := range strings.SplitSeq(lower, "/") {
		if segment == ".git" {
			return true
		}
	}
	re, err := dotGlobRegexp(lower)
	if err != nil {
		return true
	}
	// Each sample is tried at every depth the glob reaches, since a
	// segment of stars alone matches any folder above it.
	depth := strings.Count(lower, "/") + 1
	for _, sample := range gitSamples {
		for prefix := ""; strings.Count(prefix, "/") <= depth; prefix += "d/" {
			if re.MatchString(prefix + sample) {
				return true
			}
		}
	}
	// Each segment on its own: a wildcard one deeper in the glob, such
	// as sub/.gi?/FETCH_HEAD, may still name a Git folder. A segment of
	// stars alone matches every name; the samples above cover it.
	segments := strings.Split(lower, "/")
	matches := func(segment, name string) bool {
		if strings.Trim(segment, "*") == "" {
			return false
		}
		seg, err := dotGlobRegexp("/" + segment)
		return err != nil || seg.MatchString("/"+name)
	}
	for i, segment := range segments {
		if matches(segment, ".git") || matches(segment, ".gitconfig") {
			return true
		}
		if i+1 < len(segments) && matches(segment, ".config") && matches(segments[i+1], "git") {
			return true
		}
	}
	return false
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
// Agent specs are among them: an x-claude hooks or permissionMode field
// in one becomes a hook or a permission mode after the next sync.
var fixedProtected = spec.ProtectGroup{Paths: []string{
	"**/.git", "**/.gitconfig", "**/.config/git",
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
	".agnostic-ai/agents", ".agnostic-ai/hooks", ".agnostic-ai/settings", ".agnostic-ai/mcps", ".agnostic-ai/overlays", ".agnostic-ai/scripts",
	".agnostic-ai/local/agents", ".agnostic-ai/local/hooks", ".agnostic-ai/local/settings", ".agnostic-ai/local/mcps",
	".agnostic-ai/local/overlays", ".agnostic-ai/local/scripts",
	".agnostic-ai/packs",
	"**/agnostic.packs.lock",
	"**/agnostic-ai.yaml", "**/agnostic-ai.local.yaml", "**/agnostic.config.yaml",
	"**/.bashrc", "**/.bash_profile", "**/.bash_login", "**/.bash_logout", "**/.profile",
	"**/.zshrc", "**/.zshenv", "**/.zprofile", "**/.zlogin", "**/.zlogout",
	"**/.config/fish",
}}

// homeProtected are paths under the home folder an edit always asks
// for, beside fixedProtected read from the home folder: programs on the
// PATH and login agents run without the person.
var homeProtected = spec.ProtectGroup{Paths: []string{
	"Library/LaunchAgents", "Library/LaunchDaemons", ".config/autostart", ".config/systemd",
	"bin", ".local/bin", "go/bin", ".cargo/bin", ".bun/bin", ".deno/bin", ".npm-global/bin",
}}

// homeTopProtected is fixedProtected anchored at the home folder: a
// **/ pattern there means a folder right under home, so a tool folder
// inside some project above the session does not protect it.
var homeTopProtected = func() spec.ProtectGroup {
	var group spec.ProtectGroup
	for _, p := range fixedProtected.Paths {
		group.Paths = append(group.Paths, strings.TrimPrefix(p, "**/"))
	}
	return group
}()

// protected reports whether abs, inside the project, is a path an edit
// always asks for, or one under $AGNOSTIC_AI_HOME. Read from the home
// folder too, so a project inside a tool folder such as ~/.claude does
// not make its files plain project files.
func (g *gate) protected(abs string) bool {
	if g.underFolder(abs, g.policy.AgnosticHome) {
		return true
	}
	if g.protectedFromHome(abs) {
		return true
	}
	if g.protectedRel(g.relative(abs)) {
		return true
	}
	return g.policy.Resolve != nil && g.protectedRel(g.relative(filepath.Clean(g.policy.Resolve(abs))))
}

// underFolder reports whether abs sits under the absolute folder dir,
// as written or with links resolved.
func (g *gate) underFolder(abs, dir string) bool {
	if dir == "" || !filepath.IsAbs(dir) {
		return false
	}
	if underFold(abs, filepath.Clean(dir)) {
		return true
	}
	return g.policy.Resolve != nil && underFold(filepath.Clean(g.policy.Resolve(abs)), filepath.Clean(g.policy.Resolve(dir)))
}

// underFold is under with case ignored, since macOS and Windows disks
// ignore it, as the protected path globs do.
func underFold(p, dir string) bool {
	return under(strings.ToLower(p), strings.ToLower(dir))
}

// specRule asks for a write to abs under, or above, a folder that a
// project config file moves specs to. It reads the configs in every
// folder above abs and above the session folder, each as written and
// with links resolved, up to the file system root, so a config below the
// session folder, above the Git root, or one whose source points
// outside its own folder counts. A config that does not read asks.
func (g *gate) specRule(abs string) string {
	if g.policy.SpecSources == nil {
		return ""
	}
	starts := []string{filepath.Dir(abs), g.cwd}
	if g.policy.Resolve != nil {
		starts = append(starts, filepath.Dir(filepath.Clean(g.policy.Resolve(abs))), filepath.Clean(g.policy.Resolve(g.cwd)))
	}
	for _, start := range starts {
		for dir := start; ; dir = filepath.Dir(dir) {
			sources, err := g.specSourcesIn(dir)
			if err != nil {
				return RuleProjectConfig
			}
			for _, source := range sources {
				if g.underFolder(abs, source) || underFold(source, abs) {
					return RuleProtectedPath
				}
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	return ""
}

// specSourcesIn is SpecSources for dir, read once per request.
func (g *gate) specSourcesIn(dir string) ([]string, error) {
	if found, ok := g.specCache[dir]; ok {
		return found.dirs, found.err
	}
	dirs, err := g.policy.SpecSources(dir)
	if g.specCache == nil {
		g.specCache = map[string]specFolders{}
	}
	g.specCache[dir] = specFolders{dirs: dirs, err: err}
	return dirs, err
}

type specFolders struct {
	dirs []string
	err  error
}

func (g *gate) protectedFromHome(abs string) bool {
	paths := []string{abs}
	if g.policy.Resolve != nil {
		paths = append(paths, filepath.Clean(g.policy.Resolve(abs)))
	}
	for _, home := range g.homes() {
		for _, p := range paths {
			if p == home || !underFold(p, home) {
				continue
			}
			rel, err := filepath.Rel(strings.ToLower(home), strings.ToLower(p))
			if err != nil {
				continue
			}
			rel = filepath.ToSlash(rel)
			if segments := strings.Split(rel, "/"); len(segments) > 2 && segments[1] == "worktrees" &&
				strings.HasPrefix(segments[0], ".") && segments[0] != ".git" {
				// A tool's own worktrees, such as ~/.codex/worktrees, hold
				// checkouts, not the tool's settings.
				continue
			}
			if _, ok := homeTopProtected.Match(rel); ok {
				return true
			}
			if _, ok := homeProtected.Match(rel); ok {
				return true
			}
		}
	}
	return false
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
