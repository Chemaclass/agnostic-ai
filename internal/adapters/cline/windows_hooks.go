package cline

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func renderWindowsHookFiles(hooks []spec.Entry) []emit.HookScript {
	commands := map[string][]string{}
	var order []string
	for _, hook := range hooks {
		meta := emit.ResolveMeta(hook.Meta, target)
		if meta["runtime"] != "vscode" {
			continue
		}
		event, _ := meta["event"].(string)
		canonical, ok := clineHookEventByKey[strings.ToLower(strings.TrimSpace(event))]
		if !ok || canonical != "TaskStart" {
			emit.NoteSurfaceGap(target, spec.KindHook, 1, "Windows VS Code hooks", "runtime vscode currently writes a native companion only for TaskStart")
			continue
		}
		windows := emit.HookCommands(meta["commandWindows"])
		if len(windows) == 0 {
			emit.NoteSurfaceGap(target, spec.KindHook, 1, "Windows VS Code hooks", "provide commandWindows for a native PowerShell hook")
			continue
		}
		if _, seen := commands[canonical]; !seen {
			order = append(order, canonical)
		}
		commands[canonical] = append(commands[canonical], windows...)
	}
	var files []emit.HookScript
	for _, event := range order {
		var body strings.Builder
		body.WriteString("$ErrorActionPreference = 'Stop'\n")
		body.WriteString("[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)\n")
		body.WriteString("$env:AGNOSTIC_AI_TARGET = 'cline'\n")
		for _, command := range commands[event] {
			body.WriteString("$global:LASTEXITCODE = 0\n& {\n")
			body.WriteString(command)
			body.WriteString("\n}\nif ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }\n")
		}
		// Windows PowerShell 5 reads UTF-8 scripts correctly only with a BOM.
		text := "\U0000FEFF" + emit.WithHeader(body.String(), emit.FormatShell)
		files = append(files, emit.HookScript{Path: event + ".ps1", Body: []byte(text), Mode: 0o644})
	}
	return files
}
