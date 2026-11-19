package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/spf13/cobra"
)

// syncJSONOutput is the `sync --json` output: the shared schema plus the
// capability warnings and coverage notes a plain sync prints.
type syncJSONOutput struct {
	jsonOutput
	syncDrops
}

type syncDrops struct {
	Warnings []dropRecord `json:"warnings"`
	Notes    []dropRecord `json:"notes"`
}

// pendingSyncDrops returns the buffered capability warnings and coverage
// notes, ordered by target, without clearing them.
func pendingSyncDrops() syncDrops {
	return syncDrops{
		Warnings: dropRecords(adapters.PendingCapabilityWarnings()),
		Notes:    dropRecords(adapters.PendingCoverageNotes()),
	}
}

func dropRecords(records []adapters.DropRecord) []dropRecord {
	out := make([]dropRecord, 0, len(records))
	for _, r := range records {
		out = append(out, dropRecord{Target: r.Target, Kind: string(r.Kind), Count: r.Count, Message: r.Message, Suggestion: r.Suggestion})
	}
	return out
}

// printSyncPlan prints a human-readable per-target summary of what sync
// would change: how many files are missing (added) or stale (changed).
func printSyncPlan(cmd *cobra.Command, reports []driftReport) {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	for _, r := range reports {
		if !r.hasDrift() {
			_, _ = fmt.Fprintf(w, "[%s]\tno changes\n", r.Target)
			continue
		}
		_, _ = fmt.Fprintf(w, "[%s]\tadded: %d\tchanged: %d", r.Target, len(r.Missing), len(r.changed()))
		if len(r.Orphaned) > 0 {
			_, _ = fmt.Fprintf(w, "\torphaned: %d", len(r.Orphaned))
		}
		switch {
		case len(r.Leftover) > 0 && r.Unledgered:
			_, _ = fmt.Fprintf(w, "\tleftover: %d", len(r.Leftover))
		case len(r.Leftover) > 0:
			_, _ = fmt.Fprintf(w, "\tremoved: %d", len(r.Leftover))
		}
		_, _ = fmt.Fprintln(w)
	}
	_ = w.Flush()
}

// printSyncPlanJSON emits what a sync would do in the `sync --json`
// schema, with the create, update, and delete actions a real run reports,
// and kept orphans and leftovers in skipped. A dry run also lists each
// unchanged file as a skip, so every planned output appears once.
func printSyncPlanJSON(cmd *cobra.Command, command string, reports []driftReport, withCurrent bool, drops syncDrops, notesErr error) error {
	out := jsonOutput{Version: "1", Command: command}
	out.addError(notesErr)
	for _, r := range reports {
		for _, f := range r.Missing {
			out.Writes = append(out.Writes, fileRecord{Target: r.Target, Path: f.Path, Action: "create", Bytes: len(f.Content)})
		}
		for _, f := range r.changed() {
			out.Writes = append(out.Writes, fileRecord{Target: r.Target, Path: f.Path, Action: "update", Bytes: len(f.Content)})
		}
		for _, p := range r.Leftover {
			if r.Unledgered {
				out.Skipped = append(out.Skipped, fileRecord{Target: r.Target, Path: p, Action: "leftover"})
				continue
			}
			out.Writes = append(out.Writes, fileRecord{Target: r.Target, Path: p, Action: "delete"})
		}
		for _, p := range r.Orphaned {
			out.Skipped = append(out.Skipped, fileRecord{Target: r.Target, Path: p, Action: "orphan"})
		}
		if withCurrent {
			for _, f := range r.Current {
				out.Skipped = append(out.Skipped, fileRecord{Target: r.Target, Path: f.Path, Action: "skip", Bytes: len(f.Content)})
			}
		}
	}
	if err := writeIndentedJSON(cmd, syncJSONOutput{out.forOutput(), drops}); err != nil {
		return err
	}
	return notesErr
}

// printSyncCheckJSON emits a JSON result for `sync --check`. Every drifted
// file appears in writes, with the actions driftRecords assigns.
func printSyncCheckJSON(cmd *cobra.Command, reports []driftReport, drops syncDrops, notesErr error) error {
	out := jsonOutput{Version: "1", Command: "sync --check", Writes: driftRecords(reports)}
	out.addError(notesErr)
	hasDrift := len(out.Writes) > 0
	if err := writeIndentedJSON(cmd, syncJSONOutput{out.forOutput(), drops}); err != nil {
		return err
	}
	if hasDrift {
		return errors.Join(errDriftDetected(), notesErr)
	}
	return notesErr
}

// checkFormatHuman is the default `--check` report: a per-target table.
// checkFormatGitHub emits GitHub Actions annotations. JSON output stays
// behind the separate `--json` flag, which takes precedence over --format.
const (
	checkFormatHuman  = "human"
	checkFormatGitHub = "github"
)

// validateCheckFormat rejects an unknown `--format` value so a typo fails
// fast instead of silently falling back to the human table.
func validateCheckFormat(format string) error {
	switch format {
	case checkFormatHuman, checkFormatGitHub:
		return nil
	}
	return fmt.Errorf("--format: expected %q or %q, got %q", checkFormatHuman, checkFormatGitHub, format)
}

// errDriftDetected is the failure a drifting `sync --check` returns. It names
// `agnostic-ai doctor` so a bare CI line still points at the full diagnosis;
// the reconcile command prints separately as a stderr hint.
func errDriftDetected() error {
	return fmt.Errorf("drift detected; run `agnostic-ai doctor` for a full diagnosis")
}

// reportCheckDrift renders a `sync --check` result in the requested text
// format and returns a non-zero error when any target drifts. The report
// goes to stdout; the fix hint goes to stderr, so a machine reading stdout (a
// captured log, a github matcher) sees only the drift lines. Presentation
// only: it never writes files or changes what counts as drift.
func reportCheckDrift(cmd *cobra.Command, reports []driftReport, format string, diff bool) error {
	var drift bool
	switch format {
	case checkFormatGitHub:
		drift = printDriftGitHub(cmd, reports)
	default:
		drift = printDrift(reports)
	}
	if diff {
		printDriftDiffs(cmd, reports)
	}
	if !drift {
		return nil
	}
	if n, ref := lineEndingCount(foldSharedDrift(reports)); n > 0 {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%d file(s) differ only in line endings: %s\n", n, lineEndingAdvice(ref))
	}
	if hint := reconcileHint(reports); hint != "" {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), hint)
	}
	return errDriftDetected()
}

// reconcileHint names what settles the drift in reports. doctor --fix
// removes an unledgered leftover and writes the rest; a scope document
// no ledger proves sync wrote is left to the user, so it gets a manual
// step instead of a command that would leave it in place.
func reconcileHint(reports []driftReport) string {
	var syncDrift, removable, scoped bool
	var adopt []string
	for _, r := range reports {
		for _, f := range r.Unmanaged {
			if !slices.Contains(adopt, f.Target) {
				adopt = append(adopt, f.Target)
			}
		}
		switch {
		case !r.hasDrift():
		case r.against != "" && len(r.Leftover)+len(r.Orphaned) == 0:
			// The --against step, not sync, settles it.
		case len(r.Unmanaged) > 0:
		case r.Unledgered:
			removable = removable || len(r.Leftover) > 0
			scoped = scoped || len(r.Orphaned) > 0
		default:
			syncDrift = true
		}
	}
	const manual = "delete the files that look generated, listed above, by hand if stale, or list them under sync.unmanaged"
	fix := ""
	switch {
	case removable:
		fix = "doctor --fix"
	case syncDrift:
		fix = "sync"
	}
	var hint string
	switch {
	case fix == "" && !scoped:
	case fix == "":
		hint = "to reconcile, " + manual
	case scoped:
		hint = "to reconcile, run: agnostic-ai " + fix + ", then " + manual
	default:
		hint = "to reconcile, run: agnostic-ai " + fix
	}
	if len(adopt) == 0 {
		return hint
	}
	imports := make([]string, len(adopt))
	for i, t := range adopt {
		imports[i] = "agnostic-ai import " + t
	}
	move := "to move the hand-written files into .agnostic-ai/, run: " + strings.Join(imports, ", ") + ", then git rm --cached them"
	if hint == "" {
		return move
	}
	return hint + "; " + move
}

// printDriftGitHub emits one GitHub Actions error annotation per drifted file
// so drift surfaces inline on the pull request. Missing files carry no line;
// stale and edited files point at the first changed line. Returns true if any target
// drifted, so an in-sync run emits nothing and stays silent.
func printDriftGitHub(cmd *cobra.Command, reports []driftReport) bool {
	out := cmd.OutOrStdout()
	drift := false
	for _, r := range foldSharedDrift(reports) {
		missing, stale := "run agnostic-ai sync to generate it", "run agnostic-ai sync to reconcile"
		if r.against != "" {
			where, step := againstPlace(r.against)
			missing = "is not " + where + "; " + step
			stale = "does not match the specs " + where + "; " + step
		} else {
			missing, stale = "is missing; "+missing, "drifted from specs; "+stale
		}
		for _, f := range r.Missing {
			drift = true
			_, _ = fmt.Fprintf(out, "::error file=%s::%s %s%s\n",
				githubProp(f.Path), githubData(filepath.ToSlash(f.Path)), missing, githubData(r.sharedNote(f.Path)))
		}
		for _, f := range r.Stale {
			drift = true
			reason := stale
			if r.lineEndings[f.Path] {
				reason = "differs only in line endings; " + lineEndingAdvice(r.against)
			}
			_, _ = fmt.Fprintf(out, "::error file=%s,line=%d::%s %s%s\n",
				githubProp(f.Path), firstChangedLine(f.Path, f.Content), githubData(filepath.ToSlash(f.Path)), githubData(reason), githubData(r.sharedNote(f.Path)))
		}
		for _, f := range r.Edited {
			drift = true
			effect := "move the edit into .agnostic-ai/, then run agnostic-ai sync"
			if f.Merged {
				effect = "sync keeps the keys it did not write and resets the ones it writes; move those edits into .agnostic-ai/, then run agnostic-ai sync"
			}
			_, _ = fmt.Fprintf(out, "::error file=%s,line=%d::%s was edited since the last sync; %s%s\n",
				githubProp(f.Path), firstChangedLine(f.Path, f.Content), githubData(filepath.ToSlash(f.Path)), effect, githubData(r.sharedNote(f.Path)))
		}
		orphanHint := "is no longer generated and its ownership could not be proven; run agnostic-ai doctor --fix to choose removal, or list it under sync.unmanaged"
		if r.Unledgered {
			orphanHint = "looks generated, with no ledger to prove sync wrote it; delete it by hand if stale, or list it under sync.unmanaged"
		}
		for _, p := range r.Orphaned {
			drift = true
			_, _ = fmt.Fprintf(out, "::error file=%s::%s %s%s\n", githubProp(p), githubData(filepath.ToSlash(p)), orphanHint, githubData(r.sharedNote(p)))
		}
		for _, p := range r.Leftover {
			drift = true
			_, _ = fmt.Fprintf(out, "::error file=%s::%s is no longer generated but still loaded; run agnostic-ai %s to remove it%s\n",
				githubProp(p), githubData(filepath.ToSlash(p)), r.leftoverFix(), githubData(r.sharedNote(p)))
		}
		for _, f := range r.Unmanaged {
			drift = true
			_, _ = fmt.Fprintf(out, "::error file=%s::%s is hand-written in a generated folder, so only %s reads it; adopt it with agnostic-ai import %s, then git rm --cached it\n",
				githubProp(f.Path), githubData(f.Path), githubData(f.Target), githubData(f.Target))
		}
	}
	return drift
}

// diffBodyMax caps the changed lines shown per file so a large rewrite does
// not flood CI logs. Past it, a summary line reports the remainder.
const diffBodyMax = 200

// printDriftDiffs prints a unified diff for every drifted file: the on-disk
// content against what sync would write. Missing files show a concise create
// line rather than a full body. Only drifted files produce output, so an
// in-sync run stays silent. Presentation only; nothing is written.
func printDriftDiffs(cmd *cobra.Command, reports []driftReport) {
	out := cmd.OutOrStdout()
	for _, r := range foldSharedDrift(reports) {
		for _, f := range r.Missing {
			_, _ = fmt.Fprintf(out, "would create %s (%d bytes)\n", filepath.ToSlash(f.Path), len(f.Content))
		}
		for _, f := range r.changed() {
			disk, err := os.ReadFile(f.Path)
			if err != nil {
				_, _ = fmt.Fprintf(out, "%s: %v\n", f.Path, err)
				continue
			}
			_, _ = fmt.Fprint(out, unifiedDiff(f.Path, string(disk), f.Content, diffBodyMax))
		}
	}
}

// printDriftRemovals names each generated file a sync would delete,
// the removals a dry-run diff has no hunk for.
func printDriftRemovals(cmd *cobra.Command, reports []driftReport) {
	out := cmd.OutOrStdout()
	for _, r := range foldSharedDrift(reports) {
		if r.Unledgered {
			continue
		}
		for _, path := range r.Leftover {
			_, _ = fmt.Fprintf(out, "would remove %s\n", filepath.ToSlash(path))
		}
	}
}

// unifiedDiff renders a minimal unified diff between the on-disk content
// (have) and what sync would write (want), keyed by path. It trims the shared
// leading and trailing lines and prints the differing middle as one hunk:
// on-disk lines with `-`, would-write lines with `+`. Output past maxLines is
// dropped with a summary so CI logs stay lean. A display helper over line
// slices, not a general diff engine.
func unifiedDiff(path, have, want string, maxLines int) string {
	slash := filepath.ToSlash(path)
	return previewDiff(path, slash+" (on disk)", slash+" (agnostic-ai sync)", have, want, false, maxLines)
}

func previewDiff(path, haveLabel, wantLabel, have, want string, create bool, maxLines int) string {
	before, after := preview.Display(path, have), preview.Display(path, want)
	if have != want && before.Text == after.Text && (before.Hidden || after.Hidden) {
		return fmt.Sprintf("--- %s\n+++ %s\nsensitive value changed; contents hidden\n", haveLabel, wantLabel)
	}
	var haveLines []string
	if !create {
		haveLines = splitLines(before.Text)
	}
	return labeledDiff(haveLabel, wantLabel, haveLines, splitLines(after.Text), maxLines)
}

// labeledDiff is unifiedDiff over line slices with caller-chosen `---` and
// `+++` labels. A nil haveLines renders a file creation.
func labeledDiff(haveLabel, wantLabel string, haveLines, wantLines []string, maxLines int) string {
	p := commonPrefix(haveLines, wantLines)
	s := 0
	for s < len(haveLines)-p && s < len(wantLines)-p &&
		haveLines[len(haveLines)-1-s] == wantLines[len(wantLines)-1-s] {
		s++
	}
	removed := haveLines[p : len(haveLines)-s]
	added := wantLines[p : len(wantLines)-s]

	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n", haveLabel)
	fmt.Fprintf(&b, "+++ %s\n", wantLabel)
	fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", p+1, len(removed), p+1, len(added))

	shown := 0
	body := len(removed) + len(added)
	write := func(mark string, lines []string) {
		for _, ln := range lines {
			if shown >= maxLines {
				return
			}
			fmt.Fprintf(&b, "%s%s\n", mark, ln)
			shown++
		}
	}
	write("-", removed)
	write("+", added)
	if body > maxLines {
		fmt.Fprintf(&b, "... %d more changed line(s) truncated\n", body-maxLines)
	}
	return b.String()
}

// firstChangedLine returns the 1-based line where the on-disk file at path
// first differs from want, or 1 when the file is unreadable. Only used to aim
// a github annotation; never load-bearing.
func firstChangedLine(path, want string) int {
	disk, err := os.ReadFile(path)
	if err != nil {
		return 1
	}
	return commonPrefix(splitLines(string(disk)), splitLines(want)) + 1
}

// commonPrefix returns the count of leading lines a and b share.
func commonPrefix(a, b []string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

// githubData escapes a value for the message body of a github workflow
// command. `%` is escaped first so later escapes are not re-escaped.
func githubData(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	s = strings.ReplaceAll(s, "\n", "%0A")
	return s
}

// githubProp escapes a value used as a github workflow command property
// (file=...). Properties additionally escape `:` and `,`.
func githubProp(s string) string {
	s = githubData(filepath.ToSlash(s))
	s = strings.ReplaceAll(s, ":", "%3A")
	s = strings.ReplaceAll(s, ",", "%2C")
	return s
}
