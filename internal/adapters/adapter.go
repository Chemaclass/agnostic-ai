// Package adapters exposes the per-target Adapter interface and the
// registry mapping target names to implementations.
package adapters

import (
	"errors"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"

	"github.com/chemaclass/agnostic-ai/internal/adapters/aider"
	"github.com/chemaclass/agnostic-ai/internal/adapters/amp"
	"github.com/chemaclass/agnostic-ai/internal/adapters/antigravity"
	"github.com/chemaclass/agnostic-ai/internal/adapters/augment"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claude"
	"github.com/chemaclass/agnostic-ai/internal/adapters/cline"
	"github.com/chemaclass/agnostic-ai/internal/adapters/codebuddy"
	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
	"github.com/chemaclass/agnostic-ai/internal/adapters/continueai"
	"github.com/chemaclass/agnostic-ai/internal/adapters/copilot"
	"github.com/chemaclass/agnostic-ai/internal/adapters/crush"
	"github.com/chemaclass/agnostic-ai/internal/adapters/cursor"
	"github.com/chemaclass/agnostic-ai/internal/adapters/ellipsis"
	"github.com/chemaclass/agnostic-ai/internal/adapters/external"
	"github.com/chemaclass/agnostic-ai/internal/adapters/factory"
	"github.com/chemaclass/agnostic-ai/internal/adapters/gemini"
	"github.com/chemaclass/agnostic-ai/internal/adapters/goose"
	"github.com/chemaclass/agnostic-ai/internal/adapters/grok"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/adapters/jules"
	"github.com/chemaclass/agnostic-ai/internal/adapters/junie"
	"github.com/chemaclass/agnostic-ai/internal/adapters/kilo"
	"github.com/chemaclass/agnostic-ai/internal/adapters/kimi"
	"github.com/chemaclass/agnostic-ai/internal/adapters/kiro"
	"github.com/chemaclass/agnostic-ai/internal/adapters/opencode"
	"github.com/chemaclass/agnostic-ai/internal/adapters/openhands"
	"github.com/chemaclass/agnostic-ai/internal/adapters/pi"
	"github.com/chemaclass/agnostic-ai/internal/adapters/qoder"
	"github.com/chemaclass/agnostic-ai/internal/adapters/qwen"
	"github.com/chemaclass/agnostic-ai/internal/adapters/trae"
	"github.com/chemaclass/agnostic-ai/internal/adapters/vibe"
	"github.com/chemaclass/agnostic-ai/internal/adapters/warp"
	"github.com/chemaclass/agnostic-ai/internal/adapters/windsurf"
	"github.com/chemaclass/agnostic-ai/internal/adapters/zed"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/suggest"
)

// Adapter is the contract every target implementation satisfies.
type Adapter interface {
	// Name returns the target identifier used in config and CLI flags.
	Name() string
	// Capabilities returns the portable spec kinds the target emits natively.
	// The playground and documentation use this declaration directly, so a
	// target-audit fix has one capability source to update.
	Capabilities() []spec.Kind
	// Emit renders the bundle as files for this target, routing every
	// write through sess so the caller owns the capture / recording /
	// backup / transaction buffers. dryRun prints rather than writing.
	Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error
}

// TargetCapabilities is the public, read-only capability view for one
// registered target.
type TargetCapabilities struct {
	Name     string
	Supports []spec.Kind
}

// CapabilityMatrix returns every built-in target and its declared native
// spec kinds sorted by target name. Returned slices are copies so callers cannot
// mutate adapter declarations.
func CapabilityMatrix() []TargetCapabilities {
	names := Names()
	matrix := make([]TargetCapabilities, 0, len(names))
	for _, name := range names {
		a := registry[name]
		matrix = append(matrix, TargetCapabilities{
			Name:     name,
			Supports: append([]spec.Kind(nil), a.Capabilities()...),
		})
	}
	return matrix
}

func SkillDirectoryWriters(cfg *config.Config) map[string][]string {
	writers := map[string][]string{}
	if cfg == nil {
		return writers
	}
	for _, target := range cfg.Targets {
		if dir := emit.VarsFor(cfg, target)[emit.VarSkillsDir]; dir != "" {
			writers[filepath.Clean(dir)] = append(writers[filepath.Clean(dir)], target)
		}
	}
	return writers
}

// EmitWithProvenance wraps adapter.Emit with the per-target provenance
// header toggle and per-target spec scoping. Filters the bundle through
// `spec.Bundle.For(target)` so adapters only see entries whose
// `target:` / `targets:` / `target-exclude:` / `targets-exclude:`
// frontmatter allows the named target (closes #292). Reads
// `outputs.<target>.provenance-header` from cfg (default true) and
// installs the toggle for the duration of the emit so adapter calls to
// emit.WithHeader honor it. CLI dispatch sites (sync, check, status,
// revert, render, collision) should prefer this wrapper over a bare
// adapter.Emit. Installs the user-owned path set from `sync.unmanaged`
// on sess so every adapter, current and future, honors it without
// per-adapter code.
func EmitWithProvenance(sess *Session, a Adapter, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	defer emit.ProvenanceFor(cfg, a.Name())()
	if cfg != nil {
		sess.SetUnmanaged(cfg.Sync.Unmanaged)
		codexSkills := ""
		if slices.Contains(cfg.Targets, "codex") {
			codexSkills = emit.OutputSkillsDir(cfg, "codex", emit.CodexSkillsRoot)
		}
		sess.SetCodexSkillsDir(codexSkills)
		sess.SetSkillsDirWriters(SkillDirectoryWriters(cfg))
	}
	own, err := targetSkills(b.For(a.Name()), cfg, a.Name())
	if err != nil {
		return err
	}
	mode := ""
	if cfg != nil {
		mode = cfg.OnUnsupported
	}
	own, err = emit.WithoutUnsupportedDelete(own, a.Name(), mode)
	if err != nil {
		return err
	}
	own = expandBundleVars(own, cfg, a.Name())
	own.Hooks = emit.TargetHooks(a.Name(), own.Hooks)
	if slices.Contains(a.Capabilities(), spec.KindHook) {
		NotePortableHookGaps(a.Name(), b.Hooks)
	}
	prepared, files, err := emit.PrepareScopedDocuments(own, cfg, a.Name(), ReviewSections(b, cfg, a.Name()))
	if err != nil {
		return err
	}
	prepared.Rules = withoutEntryPointRules(sess, cfg, b, a.Name(), prepared.Rules)
	if err := checkProtectedPaths(a, prepared.Settings); err != nil {
		return err
	}
	if slices.Contains(a.Capabilities(), spec.KindMCP) {
		if err := spec.CheckMCPJSONValues(prepared.MCPs, a.Name()); err != nil {
			return err
		}
		// An external adapter receives the spec's own `${NAME}` form.
		if _, inTree := registry[a.Name()]; inTree {
			prepared.MCPs = rewriteMCPRefs(a, prepared.MCPs)
		}
	}
	prepared = withAgentMCPServers(a, prepared, cfg)
	if err := a.Emit(sess, prepared, cfg, dryRun); err != nil {
		return err
	}
	if slices.Contains(a.Capabilities(), spec.KindEnvironment) {
		emit.RecordEnvironmentFields(a.Name(), prepared.Environments)
	}
	for _, f := range files {
		if err := sess.WriteFile(f.Path, f.Content, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// NotePortableHookGaps notes the portable hooks that reach target but
// have no exact native form there, one note per reason.
func NotePortableHookGaps(target string, hooks []spec.Entry) {
	counts := map[string]int{}
	var reasons []string
	for _, h := range hooks {
		if !h.EmitsTo(target) {
			continue
		}
		if _, reason := h.NativeHook(target); reason != "" {
			if counts[reason] == 0 {
				reasons = append(reasons, reason)
			}
			counts[reason]++
		}
	}
	for _, reason := range reasons {
		emit.NoteCoverageGap(target, spec.KindHook, counts[reason], reason)
	}
}

var registry = map[string]Adapter{
	"claude":      claude.New(),
	"codex":       codex.New(),
	"gemini":      gemini.New(),
	"qwen":        qwen.New(),
	"cursor":      cursor.New(),
	"copilot":     copilot.New(),
	"aider":       aider.New(),
	"cline":       cline.New(),
	"windsurf":    windsurf.New(),
	"continue":    continueai.New(),
	"amp":         amp.New(),
	"zed":         zed.New(),
	"warp":        warp.New(),
	"opencode":    opencode.New(),
	"antigravity": antigravity.New(),
	"junie":       junie.New(),
	"kiro":        kiro.New(),
	"crush":       crush.New(),
	"trae":        trae.New(),
	"jules":       jules.New(),
	"goose":       goose.New(),
	"grok":        grok.New(),
	"augment":     augment.New(),
	"qoder":       qoder.New(),
	"vibe":        vibe.New(),
	"kimi":        kimi.New(),
	"pi":          pi.New(),
	"openhands":   openhands.New(),
	"factory":     factory.New(),
	"kilo":        kilo.New(),
	"ellipsis":    ellipsis.New(),
	"codebuddy":   codebuddy.New(),
}

// Get returns the adapter registered under name. Lookup is restricted to
// the in-tree registry; for the full lookup including external adapters
// discovered on PATH, use Resolve.
func Get(name string) (Adapter, bool) {
	a, ok := registry[name]
	return a, ok
}

// Resolve returns the adapter for name. The in-tree registry is checked
// first; if no built-in matches, an external adapter named
// `agnostic-ai-adapter-<name>` is looked up on PATH. Callers should use
// Resolve rather than Get so opt-in external targets work.
func Resolve(name string) (Adapter, error) {
	if a, ok := registry[name]; ok {
		return a, nil
	}
	a, err := external.New(name)
	if err == nil {
		return a, nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		if s := suggest.Name(name, Names()); s != "" {
			return nil, errs.Coded(errs.CodeSyncTargetUnknown,
				"unknown target: %s (did you mean %s? no %s%s on PATH)",
				name, s, external.BinaryPrefix, name)
		}
		return nil, errs.Coded(errs.CodeSyncTargetUnknown,
			"unknown target: %s (no built-in adapter and no %s%s on PATH)",
			name, external.BinaryPrefix, name)
	}
	return nil, errs.Coded(errs.CodeSyncTargetUnknown,
		"unknown target: %s: %w", name, err)
}

// Names returns every registered target name.
func Names() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
