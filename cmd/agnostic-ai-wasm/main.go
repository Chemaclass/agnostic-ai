// Command agnostic-ai-wasm is the WebAssembly entry point used by the
// in-browser playground. It exposes a single global function,
// `agnosticAIRender`, that takes one spec body plus a list of targets
// and returns each adapter's emitted output as a JSON-friendly object.
//
// The binary is built with `GOOS=js GOARCH=wasm` and shipped alongside
// the static playground at docs/playground/. The full adapter registry
// is linked in, so the playground reflects every target the released
// CLI supports.
//
//go:build js && wasm

package main

import (
	"fmt"
	"strings"
	"syscall/js"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func main() {
	js.Global().Set("agnosticAIRender", js.FuncOf(render))
	js.Global().Set("agnosticAITargets", js.FuncOf(listTargets))
	// Park the goroutine; closing it would invalidate the exported
	// JS callbacks and break the playground after the first call.
	select {}
}

// render is the JS-callable entry point.
//
// Args (positional):
//
//	0: kind (string)        — one of agent, skill, rule, hook, mcp.
//	1: body (string)        — full spec source (frontmatter + body).
//	2: targets (Array<string>) — adapter names to emit.
//
// Returns a JS object:
//
//	{
//	  files: [{ target, path, content }],
//	  errors: [{ target, message }]
//	}
func render(_ js.Value, args []js.Value) any {
	if len(args) != 3 {
		return jsError(fmt.Errorf("expected 3 args (kind, body, targets), got %d", len(args)))
	}
	kindRaw := args[0].String()
	body := args[1].String()
	targetsArg := args[2]

	kind, err := normalizeKind(kindRaw)
	if err != nil {
		return jsError(err)
	}
	targets := jsStringArray(targetsArg)
	if len(targets) == 0 {
		return jsError(fmt.Errorf("at least one target required"))
	}

	entry, err := parseEntry(kind, body)
	if err != nil {
		return jsError(err)
	}
	if entry.Name == "" {
		// Adapters use Name as a slug for per-file outputs; falling
		// back here keeps the playground useful even when the user is
		// still drafting frontmatter.
		entry.Name = "untitled"
		if entry.Meta == nil {
			entry.Meta = map[string]any{}
		}
		entry.Meta["name"] = entry.Name
	}

	bundle := singleEntryBundleWASM(entry)
	cfg := defaultPlaygroundConfig(targets)

	files := []any{}
	errs := []any{}
	for _, t := range targets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			errs = append(errs, jsErrorEntry(t, err))
			continue
		}
		adapters.StartCapture()
		err = adapter.Emit(bundle, cfg, false)
		captured := adapters.StopCapture()
		if err != nil {
			errs = append(errs, jsErrorEntry(t, err))
			continue
		}
		for _, f := range captured {
			files = append(files, map[string]any{
				"target":  t,
				"path":    f.Path,
				"content": f.Content,
			})
		}
	}
	return js.ValueOf(map[string]any{
		"files":  files,
		"errors": errs,
	})
}

func listTargets(_ js.Value, _ []js.Value) any {
	names := adapters.Names()
	out := make([]any, 0, len(names))
	for _, n := range names {
		out = append(out, n)
	}
	return js.ValueOf(out)
}

func parseEntry(kind spec.Kind, body string) (spec.Entry, error) {
	switch kind {
	case spec.KindHook, spec.KindMCP:
		return spec.ParseYAMLBytes(kind, []byte(body))
	default:
		return spec.ParseMarkdownBytes(kind, []byte(body))
	}
}

func normalizeKind(raw string) (spec.Kind, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "agent":
		return spec.KindAgent, nil
	case "skill":
		return spec.KindSkill, nil
	case "rule":
		return spec.KindRule, nil
	case "hook":
		return spec.KindHook, nil
	case "mcp":
		return spec.KindMCP, nil
	}
	return "", fmt.Errorf("unknown kind %q (expected agent, skill, rule, hook, mcp)", raw)
}

func singleEntryBundleWASM(e spec.Entry) spec.Bundle {
	var b spec.Bundle
	switch e.Kind {
	case spec.KindAgent:
		b.Agents = []spec.Entry{e}
	case spec.KindSkill:
		b.Skills = []spec.Entry{e}
	case spec.KindRule:
		b.Rules = []spec.Entry{e}
	case spec.KindHook:
		b.Hooks = []spec.Entry{e}
	case spec.KindMCP:
		b.MCPs = []spec.Entry{e}
	}
	return b
}

// defaultPlaygroundConfig fills in source paths and target list using
// the same defaults the CLI uses on a fresh `init`. Adapters need a
// non-nil cfg with these fields populated.
func defaultPlaygroundConfig(targets []string) *config.Config {
	return &config.Config{
		Version: 1,
		Sources: config.Sources{
			Agents: ".agnostic-ai/agents",
			Skills: ".agnostic-ai/skills",
			Rules:  ".agnostic-ai/rules",
			Hooks:  ".agnostic-ai/hooks",
			MCPs:   ".agnostic-ai/mcps",
		},
		Targets:       targets,
		OnUnsupported: "warn",
	}
}

func jsStringArray(v js.Value) []string {
	if v.Type() != js.TypeObject {
		return nil
	}
	n := v.Length()
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = v.Index(i).String()
	}
	return out
}

func jsError(err error) any {
	return js.ValueOf(map[string]any{
		"files":  []any{},
		"errors": []any{map[string]any{"target": "", "message": err.Error()}},
	})
}

func jsErrorEntry(target string, err error) any {
	return map[string]any{"target": target, "message": err.Error()}
}
