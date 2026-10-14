// Package ellipsis emits the code-review file Ellipsis reads from the
// default branch of a repository.
//
// Ellipsis runs one review per push from `.ellipsis/code-review.yaml`:
// "A repository holds one file at `.ellipsis/code-review.yaml` on its
// default branch", with "5 top-level blocks: `ellipsis`, optional
// `pull_requests`, `reviewers`, optional `output`, and optional `budget`"
// (ellipsis.dev/docs/code-review/schema). Unknown fields fail the sync.
//
// Review specs feed one reviewer. Root reviews open its prompt; each
// scoped review follows as a section that applies only when the pull
// request changes a file under that directory. The reviewer itself, its
// engine and model, the pull request filters, the posted review, and the
// budget are Ellipsis settings, so they come from an `x-ellipsis` block
// on a settings spec and are copied as written. Nothing is inferred: no
// reviewer means no file, and no `output` block means Ellipsis' own
// default, which never approves.
package ellipsis

import (
	"fmt"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target            = "ellipsis"
	defaultReviewFile = ".ellipsis/code-review.yaml"
)

// Settings is claimed for the `x-ellipsis` block alone: every other
// settings field has no key in a review file.
var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindSettings, spec.KindReview},
}

// Adapter emits the Ellipsis code-review file.
type Adapter struct{}

// New returns an Ellipsis adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

func (Adapter) Capabilities() []spec.Kind { return caps.Supports }

// Emit writes `.ellipsis/code-review.yaml` when review specs and a
// complete reviewer setting both reach the target. A review with no
// usable reviewer is reported, never guessed.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	noteIgnoredSettings(b.Settings)
	prompt := reviewPrompt(b.Reviews)
	if prompt == "" {
		return nil
	}
	file, problems := readFile(b.Settings)
	if len(problems) > 0 {
		return reportProblems(problems, cfg.OnUnsupported)
	}
	doc, err := render(file, prompt)
	if err != nil {
		return err
	}
	return sess.WriteFile(emit.OutputReviewFile(cfg, target, defaultReviewFile), emit.WithHeader(doc, emit.FormatYAML), dryRun)
}

// reportProblems turns what stops the file into a note, or an error under
// `on-unsupported: error`. Silent mode skips it like every other note.
func reportProblems(problems []string, mode string) error {
	switch mode {
	case emit.OnUnsupportedSilent:
		return nil
	case emit.OnUnsupportedError:
		return fmt.Errorf("ellipsis: no code-review file written: %s", strings.Join(problems, "; "))
	}
	for _, p := range problems {
		emit.NoteProject("ellipsis: no code-review file written: " + p)
	}
	return nil
}

// noteIgnoredSettings reports the portable settings fields a review file
// has no key for. Reviewer model and GitHub access live under x-ellipsis.
func noteIgnoredSettings(settings []spec.Entry) {
	var models, permissions, protected int
	for _, entry := range settings {
		if emit.SettingsModel([]spec.Entry{entry}, target) != "" {
			models++
		}
		if emit.SettingsPermissions([]spec.Entry{entry}) != nil {
			permissions++
		}
		if _, ok := entry.Meta["protected"]; ok {
			protected++
		}
	}
	emit.NoteFieldNoOp(target, spec.KindSettings, "model", models, "the reviewer's model is x-ellipsis.reviewer.<engine>.model; no portable model reaches it")
	emit.NoteFieldNoOp(target, spec.KindSettings, "permissions", permissions, "a review file has no command permissions; GitHub access is x-ellipsis.reviewer.permissions")
	emit.NoteFieldNoOp(target, spec.KindSettings, "protected", protected, "a review file cannot block edits")
}
