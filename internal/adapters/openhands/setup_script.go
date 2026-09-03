package openhands

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// emitSetupScript writes OpenHands' repository bootstrap script,
// `.openhands/setup.sh` (docs.openhands.dev/openhands/usage/customization/repository:
// "You can add a `.openhands/setup.sh` file, which will run every time
// OpenHands begins working with your repository... an ideal location
// for installing dependencies, setting environment variables, and
// performing other setup tasks").
//
// Only the spec's `install` field maps here: it is a shell command
// string in both the documented example and Cursor's identical
// `environment.json` schema (the only other consumer of this spec
// kind), so it becomes the script body verbatim. `terminals` describes
// long-running dev processes Cursor's background agent keeps open;
// `.openhands/setup.sh` runs once, synchronously, before OpenHands
// starts working, so there is no OpenHands surface for it and it
// surfaces a coverage note instead of being silently dropped. Any
// other key is Cursor's `environment.json` schema surface (install and
// terminals are the only two fields spec-format.md documents) and has
// no OpenHands equivalent either; it passes through unnoted since the
// vendor names no shape for it to disagree with.
//
// Multiple environment specs merge the same way Cursor's do: last
// spec's `install` wins. Keeps the two targets' documented mental
// model ("one merged environment, last write wins per field")
// identical rather than inventing a second merge policy for the same
// spec kind.
//
// OpenHands chmods the script itself before running it
// (`chmod +x {script} && source {script}` in sandbox-server's
// app_conversation_service_base.go, maybe_run_setup_script), so this
// adapter writes it with the same 0644 mode every other emitted file
// gets; no executable bit to set on our side.
func emitSetupScript(sess *emit.Session, envs []spec.Entry, cfg *config.Config, dryRun bool) error {
	install, terminalsCount := resolveSetupScript(envs)
	emit.NoteFieldNoOp(target, spec.KindEnvironment, "terminals", terminalsCount,
		"OpenHands' setup.sh runs once, synchronously, at repo start; there is no long-running terminal/process surface to route it to")
	if install == "" {
		return nil
	}
	path := emit.OutputSetupFile(cfg, target, defaultSetupFile)
	return sess.WriteFile(path, renderSetupScript(install), dryRun)
}

// resolveSetupScript walks envs in order and returns the last
// non-empty `install` string plus how many entries set `terminals`
// (any value, including an empty list, counts: the author declared
// the field, so the no-op note should fire).
func resolveSetupScript(envs []spec.Entry) (string, int) {
	install := ""
	terminalsCount := 0
	for _, e := range envs {
		m := emit.ResolveMeta(e.Meta, target)
		if v, ok := m["install"].(string); ok && v != "" {
			install = v
		}
		if _, ok := m["terminals"]; ok {
			terminalsCount++
		}
	}
	return install, terminalsCount
}

// renderSetupScript wraps install in the shebang line the vendor's own
// example uses. The provenance header lands on the line after the
// shebang, not before it: `.openhands/setup.sh` is a real shell
// script, and a leading `#!` must be the file's first two bytes for
// any direct-execution fallback to route it to bash. Sourcing (what
// OpenHands actually does) would tolerate either order, but there is
// no reason to give up the guarantee for the case that needs it.
func renderSetupScript(install string) string {
	var sb strings.Builder
	sb.WriteString("#!/bin/bash\n")
	sb.WriteString(emit.HeaderBlock(emit.FormatShell))
	sb.WriteString(strings.TrimRight(install, "\n"))
	sb.WriteString("\n")
	return sb.String()
}
