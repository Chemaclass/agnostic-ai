package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// ellipsisReviewFile is the one file per repository Ellipsis reads from
// the default branch (ellipsis.dev/docs/code-review/schema).
const ellipsisReviewFile = ".ellipsis/code-review.yaml"

// ellipsisFileKeys are the top-level blocks of a code review file.
var ellipsisFileKeys = []string{"ellipsis", "pull_requests", "reviewers", "output", "budget"}

// ellipsisEngines name the session block that holds a reviewer's prompt.
var ellipsisEngines = []string{"claude_code", "codex", "gemini"}

// importFromEllipsis reads a hand-written `.ellipsis/code-review.yaml`
// into a review spec and a settings spec, reversing the ellipsis emit:
//
//   - the one reviewer's prompt becomes the root review `review`;
//   - every other key, `ellipsis.enabled` and `ellipsis.metadata` included,
//     becomes `x-ellipsis` on `settings/ellipsis.yaml`, where the adapter
//     copies it back as written.
//
// A file it cannot express stops the import with a message and writes
// nothing: more than one reviewer, another file kind, a field the schema
// does not list, or a reviewer without a prompt. A file carrying the
// provenance header imports nothing, since its specs are the source.
// Comments and key order in the hand-written file are not kept.
func importFromEllipsis(root string, src config.Sources, cfg *config.Config) error {
	path := ellipsisReviewFile
	if o, ok := cfg.Outputs["ellipsis"]; ok && o.ReviewFile != "" {
		path = o.ReviewFile
	}
	environment, err := planEllipsisEnvironment(root, cfg)
	if err != nil {
		return err
	}
	reviews, settings, err := importEllipsisFile(root, path, src, environment.imported != nil)
	if err != nil {
		return err
	}
	summaryf("imported %d reviews, %d settings\n", reviews, settings)
	if err := importEllipsisEnvironment(root, src, cfg, environment); err != nil {
		return err
	}
	printImportNextSteps(root, "ellipsis")
	return nil
}

// environmentSelected is true when the import also selects an environment.
// A reviewer without one then gets a null environment, so sync keeps it in
// Ellipsis' basic sandbox instead of linking it to the selection.
func importEllipsisFile(root, rel string, src config.Sources, environmentSelected bool) (reviews, settings int, err error) {
	native := filepath.Join(root, filepath.FromSlash(rel))
	data, err := os.ReadFile(native)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("read %s: %w", rel, err)
	}
	text := strings.ReplaceAll(strings.TrimPrefix(string(data), "\U0000FEFF"), "\r\n", "\n")
	if header.Has(text) {
		return 0, 0, nil
	}
	prompt, block, err := splitEllipsisFile(rel, text)
	if err != nil {
		return 0, 0, err
	}
	if reviewer, _ := block["reviewer"].(map[string]any); environmentSelected {
		if _, named := reviewer["environment"]; !named {
			reviewer["environment"] = nil
		}
	}
	if err := mkdirAllSources(root, src.Reviews, src.Settings); err != nil {
		return 0, 0, err
	}
	wrote, err := writeReviewSpec(importSourcePath(root, src.Reviews), rootReviewSpecName, "", prompt)
	if err != nil {
		return 0, 0, err
	}
	if wrote {
		reviews++
	}
	raw, err := yaml.Marshal(map[string]any{"x-ellipsis": block})
	if err != nil {
		return reviews, 0, fmt.Errorf("marshal settings from %s: %w", rel, err)
	}
	dst := filepath.Join(importSourcePath(root, src.Settings), "ellipsis.yaml")
	if err := importWriteFile(dst, raw, 0o644); err != nil {
		return reviews, 0, fmt.Errorf("write %s: %w", dst, err)
	}
	return reviews, 1, nil
}

// splitEllipsisFile returns the reviewer's prompt and the x-ellipsis
// block holding everything else, or says why the file cannot be imported.
func splitEllipsisFile(rel, text string) (string, map[string]any, error) {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return "", nil, fmt.Errorf("%s is not valid YAML: %w", rel, err)
	}
	if doc == nil {
		return "", nil, fmt.Errorf("%s holds no Ellipsis code review: expected the blocks %s", rel, strings.Join(ellipsisFileKeys, ", "))
	}
	var unknown []string
	for key := range doc {
		if !contains(ellipsisFileKeys, key) {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return "", nil, fmt.Errorf("%s has the field %s, which a code review file does not take; Ellipsis rejects unknown fields, so remove it and import again", rel, strings.Join(unknown, ", "))
	}
	block := map[string]any{}
	if top, ok := doc["ellipsis"].(map[string]any); ok {
		if kind, set := top["kind"]; set && kind != "code_review" {
			return "", nil, fmt.Errorf("%s is not a code_review file (ellipsis.kind is %v); agnostic-ai imports only code_review files", rel, kind)
		}
		kept := map[string]any{}
		for key, value := range top {
			if key != "kind" {
				kept[key] = value
			}
		}
		if len(kept) > 0 {
			block["ellipsis"] = kept
		}
	}
	for _, key := range []string{"pull_requests", "output", "budget"} {
		if v, ok := doc[key]; ok {
			block[key] = v
		}
	}
	reviewers, _ := doc["reviewers"].([]any)
	switch len(reviewers) {
	case 0:
		return "", nil, fmt.Errorf("%s has no reviewers; add one, with a prompt and a model, and import again", rel)
	case 1:
	default:
		return "", nil, fmt.Errorf("%s has %d reviewers, and agnostic-ai writes one; keep one reviewer in the file and import again, or leave the file hand-written and list it under sync.unmanaged", rel, len(reviewers))
	}
	reviewer, ok := reviewers[0].(map[string]any)
	if !ok {
		return "", nil, fmt.Errorf("%s: reviewers[0] must be a mapping with a name, an engine block, and a prompt", rel)
	}
	prompt, err := takeEllipsisPrompt(rel, reviewer)
	if err != nil {
		return "", nil, err
	}
	block["reviewer"] = reviewer
	return prompt, block, nil
}

// takeEllipsisPrompt removes the prompt from the reviewer's engine block
// and returns it. The review spec owns the prompt, so it must not stay in
// the settings too.
func takeEllipsisPrompt(rel string, reviewer map[string]any) (string, error) {
	for _, engine := range ellipsisEngines {
		session, ok := reviewer[engine].(map[string]any)
		if !ok {
			continue
		}
		prompt, _ := session["prompt"].(string)
		if strings.TrimSpace(prompt) == "" {
			return "", fmt.Errorf("%s: reviewer %v has no prompt in %s; agnostic-ai keeps the prompt as a review spec, so add one and import again", rel, reviewer["name"], engine)
		}
		delete(session, "prompt")
		return strings.TrimSpace(prompt), nil
	}
	return "", fmt.Errorf("%s: reviewer %v has no claude_code, codex, or gemini block with a prompt", rel, reviewer["name"])
}
