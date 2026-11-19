package approvals

import (
	"os"
	"path"
	"path/filepath"
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

// globRule asks for an unquoted glob that may expand to a credential
// file, a protected path, or a path outside the project. Each segment
// is tried against sample names; only a segment that starts with a dot
// reaches dot files, which hold most of both.
func (g *gate) globRule(word, cwd string) string {
	if pathLike(word) {
		return RuleOutsideProject
	}
	if base := globBase(word); base != "." {
		abs, ok := g.path(base, cwd)
		switch {
		case !ok || !g.inside(abs):
			return RuleOutsideProject
		case g.secret(abs):
			return RuleSecret
		case g.protected(abs) || g.protectedBelow(g.relative(abs)):
			return RuleProtectedPath
		}
	}
	secrets := []string{".env", ".env.local", "id_rsa", "x.pem", "x.key", "x.p12", ".netrc", ".npmrc", "credentials", ".ssh", ".aws", ".gnupg"}
	protected := []string{"agnostic-ai.yaml", "agnostic-ai.local.yaml", "jenkinsfile", "azure-pipelines.yml", "lefthook.yml"}
	segments := strings.Split(word, "/")
	for i, segment := range segments {
		if !strings.ContainsAny(segment, "*?[") {
			continue
		}
		lower := strings.ToLower(segment)
		last := i == len(segments)-1
		for _, sample := range secrets {
			// The shell matches a leading dot only with a literal dot;
			// the other samples are file names.
			dot := strings.HasPrefix(sample, ".")
			if dot && !strings.HasPrefix(segment, ".") || !dot && !last {
				continue
			}
			if ok, err := path.Match(lower, sample); ok || err != nil {
				return RuleSecret
			}
		}
		if strings.HasPrefix(segment, ".") {
			return RuleProtectedPath
		}
		for _, sample := range protected {
			if ok, _ := path.Match(lower, sample); ok && last {
				return RuleProtectedPath
			}
		}
	}
	return ""
}

// fixedProtected are paths an edit always asks for: Git internals, CI,
// hook and tool settings, shell profiles, and agnostic-ai's own config.
var fixedProtected = spec.ProtectGroup{Paths: []string{
	"**/.git",
	".github/workflows", ".github/hooks", ".gitlab-ci.yml", ".circleci", ".buildkite",
	"azure-pipelines.yml", "Jenkinsfile",
	".husky", ".githooks", ".pre-commit-config.yaml", "lefthook.yml", ".lefthook.yml",
	"**/.claude", "**/.codex", "**/.gemini", "**/.cursor", "**/.kiro", "**/.qwen",
	"**/.windsurf", "**/.crush", "**/.crush.json", "**/crush.json", "**/.mcp.json",
	"**/.vscode/settings.json", "**/opencode.json",
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
