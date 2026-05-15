package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/lsp"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func newLSPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lsp",
		Short: "Start the agnostic-ai Language Server (LSP) on stdio.",
		Long: "Runs a Language Server Protocol server on stdin/stdout. " +
			"Configure your editor to launch `agnostic-ai lsp` as the language " +
			"server for agnostic-ai spec files (.agnostic-ai/**/*.md, *.mdc). " +
			"The server pushes lint diagnostics whenever a file is opened or saved.",
		Example: `  # Neovim (init.lua)
  vim.lsp.start({
    name = "agnostic-ai",
    cmd = { "agnostic-ai", "lsp" },
    root_dir = vim.fn.getcwd(),
    filetypes = { "markdown" },
  })

  # VS Code (settings.json, requires a generic LSP extension)
  "languageServerExample.serverCommand": "agnostic-ai lsp"`,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			srv := lsp.New(os.Stdin, os.Stdout, lspLinter)
			return srv.Run()
		},
	}
}

// lspLinter runs the full lint suite rooted at root and returns a map of
// absolute file path → LSP diagnostics. Files with no findings are omitted
// so the server can push empty diagnostics to clear stale markers.
func lspLinter(root string) map[string][]lsp.Diagnostic {
	cfg, err := config.Load(root)
	if err != nil {
		return nil
	}
	layers := resolveLayers(root, cfg)
	b, err := spec.LoadLayered(layers)
	if err != nil {
		return nil
	}
	entries := b.All()

	var findings []lintFinding
	findings = append(findings, lintEmptySpecs(entries)...)
	findings = append(findings, lintHookCollisions(b.Hooks)...)
	findings = append(findings, lintDuplicateNames(entries)...)
	findings = append(findings, lintDeadSpecs(entries, cfg.Targets)...)
	findings = append(findings, lintHookMatcherMisuse(b.Hooks)...)

	out := map[string][]lsp.Diagnostic{}
	for _, f := range findings {
		abs, err := filepath.Abs(f.Path)
		if err != nil {
			abs = f.Path
		}
		out[abs] = append(out[abs], lintFindingToDiagnostic(f))
	}
	return out
}

func lintFindingToDiagnostic(f lintFinding) lsp.Diagnostic {
	sev := lsp.SeverityWarning
	if f.Severity == lintError {
		sev = lsp.SeverityError
	}
	msg := f.Message
	if !strings.Contains(msg, f.Code) {
		msg = fmt.Sprintf("[%s] %s", f.Code, msg)
	}
	return lsp.Diagnostic{
		Range:    lsp.Range{Start: lsp.Position{}, End: lsp.Position{Character: 1}},
		Severity: sev,
		Code:     f.Code,
		Source:   "agnostic-ai",
		Message:  msg,
	}
}
