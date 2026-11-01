package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/term"
)

// noTargetsKept reports whether cfg lists no target, in which case sync,
// watch and the drift reports keep every generated file: an empty list is
// more often a half-saved config than a request to delete everything
// (#2952). --allow-empty-targets lifts it.
func noTargetsKept(cfg *config.Config) bool { return cfg != nil && len(cfg.Targets) == 0 }

const emptyTargetsWarning = "targets lists no tool; keeping files until it does (run `agnostic-ai sync --allow-empty-targets` to remove them)"

func warnEmptyTargets(w io.Writer) {
	_, _ = fmt.Fprintf(w, "  %s %s\n", term.Bang(w), emptyTargetsWarning)
}

// warnEmptyTargetsIgnoring is warnEmptyTargets for a run that named
// targets with -t, which an empty list also skips.
func warnEmptyTargetsIgnoring(w io.Writer) {
	_, _ = fmt.Fprintf(w, "  %s %s; ignoring -t\n", term.Bang(w), emptyTargetsWarning)
}

// generatedFilesKept reports whether the last sync recorded a file that is
// still on disk, apart from the project's own .agnostic-ai folder. With an empty targets list sync keeps these files, so
// --check cannot say they are current.
func generatedFilesKept(cfg *config.Config, ledger ledgerSnapshot) bool {
	for _, p := range ledger.state.Outputs {
		if p == ".agnostic-ai" || strings.HasPrefix(p, ".agnostic-ai/") {
			continue
		}
		if _, err := os.Lstat(p); err == nil {
			return true
		}
	}
	if ledger.missing() {
		all := cfg.WithAdditionalTargets(adapters.Names()...)
		rep := unledgeredReport(all, nil, ledger.state, true, func(p string) bool {
			fi, err := os.Lstat(p)
			return err == nil && fi.Mode().IsRegular() && !cfg.IsUnmanaged(p)
		})
		return rep.hasDrift()
	}
	return false
}

func errEmptyTargetsCheck() error {
	return errs.Coded(errs.CodeEmptyTargets, "targets lists no tool and generated files remain; add a tool to targets, or run `agnostic-ai sync --allow-empty-targets` to remove them")
}

// writeEmptyTargetsJSON answers `sync --json` for an empty targets list.
func writeEmptyTargetsJSON(cmd *cobra.Command, command string, failed bool) error {
	out := jsonOutput{Version: "1", Command: command}
	if failed {
		out.addError(errEmptyTargetsCheck())
	}
	return writeIndentedJSON(cmd, syncJSONOutput{
		out.forOutput(),
		syncDrops{
			Warnings: []dropRecord{{Target: "agnostic-ai", Message: emptyTargetsWarning, Suggestion: "agnostic-ai sync --allow-empty-targets"}},
			Notes:    []dropRecord{},
		},
	})
}
