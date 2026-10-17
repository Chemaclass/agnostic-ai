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
//
// Ellipsis starts a session from an environment file under
// `.ellipsis/environments/`: "the repositories, setup scripts,
// environment variables, and machine size a session starts with", and
// its MCP servers, since "a repository's own `.mcp.json` is ignored"
// (ellipsis.dev/docs/environments). One `x-ellipsis.environment` block
// selects the one environment and its one repository. Environment specs
// fill its setup scripts, MCP specs its servers, and the review file
// names it. Without the block no environment file is written.
package ellipsis

import (
	"fmt"
	"os"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target            = "ellipsis"
	defaultReviewFile = ".ellipsis/code-review.yaml"
)

// Settings is claimed for the `x-ellipsis` block alone: every other
// settings field has no key in a review or environment file.
var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindSettings, spec.KindReview, spec.KindEnvironment, spec.KindMCP},
}

// Adapter emits the Ellipsis code-review file.
type Adapter struct{}

// New returns an Ellipsis adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

func (Adapter) Capabilities() []spec.Kind { return caps.Supports }

// Emit writes `.ellipsis/code-review.yaml` when review specs and a
// complete reviewer setting both reach the target, and the selected
// environment file when x-ellipsis.environment names one. A file with a
// problem is reported, never guessed, and no file is written when either
// one fails under `on-unsupported: error`.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	noteIgnoredSettings(b.Settings)
	environment, environmentName, err := planEnvironment(b, cfg)
	if err != nil {
		return err
	}
	review, err := planReview(b, cfg, environmentName)
	if err != nil {
		return err
	}
	for _, f := range []*plannedFile{environment, review} {
		if f == nil {
			continue
		}
		if err := sess.WriteFile(f.path, f.content, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// planReview renders the review file. environmentName is the environment
// the review runs in, or "" when none is selected.
func planReview(b spec.Bundle, cfg *config.Config, environmentName string) (*plannedFile, error) {
	prompt := reviewPrompt(b.Reviews)
	if prompt == "" {
		return nil, nil
	}
	file, problems := readFile(b.Settings, environmentName)
	path := emit.OutputReviewFile(cfg, target, defaultReviewFile)
	if len(problems) > 0 {
		// A reviewer that is set but broken must not take the deployed
		// review down with it. Without a reviewer key the sweep removes it.
		var kept *plannedFile
		if _, set := emit.SettingsCustomKeys(b.Settings, target)["reviewer"]; set {
			kept = keepDeployed(path)
		}
		return kept, reportProblems(problems, cfg.OnUnsupported, "code-review file")
	}
	doc, err := render(file, prompt)
	if err != nil {
		return nil, err
	}
	return &plannedFile{path: path, content: emit.WithHeader(doc, emit.FormatYAML)}, nil
}

// keepDeployed returns the generated file already on disk as it is, so the
// sync keeps it instead of sweeping it as a leftover. A file without the
// generated header is not ours and stays untouched.
func keepDeployed(path string) *plannedFile {
	data, err := os.ReadFile(path)
	if err != nil || !header.Has(string(data)) {
		return nil
	}
	return &plannedFile{path: path, content: string(data)}
}

// reportProblems turns what stops the file into a note, or an error under
// `on-unsupported: error`. Silent mode skips it like every other note.
func reportProblems(problems []string, mode, what string) error {
	switch mode {
	case emit.OnUnsupportedSilent:
		return nil
	case emit.OnUnsupportedError:
		return fmt.Errorf("ellipsis: no %s written: %s", what, strings.Join(problems, "; "))
	}
	for _, p := range problems {
		emit.NoteProject(fmt.Sprintf("ellipsis: no %s written: %s", what, p))
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
