package copilot

import (
	"encoding/hex"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const NativeSetupStepsPath = ".github/workflows/copilot-setup-steps.yml"
const setupStepsKey = "setup-steps"
const checkoutCommit = "3d3c42e5aac5ba805825da76410c181273ba90b1"

// checkoutVersion is the release tag of checkoutCommit, written as the
// trailing comment that Renovate, Dependabot and pin policies read.
const checkoutVersion = "v7"

// versionComment splits `<ref> # <version>`, the form a quoted spec value
// uses to keep the comment that YAML would otherwise drop.
var versionComment = regexp.MustCompile(`^(\S+)\s+#\s*(\S.*)$`)

// splitVersionComment returns the ref and its version comment, if any.
func splitVersionComment(value string) (string, string, bool) {
	if !strings.Contains(value, "#") {
		return value, "", true
	}
	m := versionComment.FindStringSubmatch(strings.TrimSpace(value))
	if m == nil {
		return value, "", false
	}
	return m[1], strings.TrimSpace(m[2]), true
}

func SetupStepsPath(cfg *config.Config) string {
	return emit.OutputSetupFile(cfg, target, NativeSetupStepsPath)
}

// SharedSetupStepsSources drops private overrides before target routing.
func SharedSetupStepsSources(envs []spec.Entry) []spec.Entry {
	var shared []spec.Entry
	for _, env := range envs {
		for env.Base != nil {
			env = *env.Base
		}
		if env.Layer != "local" && env.Layer != "user-local" {
			shared = append(shared, env)
		}
	}
	return shared
}

func setupStepsOverride(e spec.Entry) (any, bool) {
	meta, _ := e.Meta["x-copilot"].(map[string]any)
	v, set := meta[setupStepsKey]
	return v, set
}

func SetupStepsIssues(e spec.Entry) []string {
	raw, _ := setupStepsOverride(e)
	options, mapped := raw.(map[string]any)
	_, toggle := raw.(bool)
	if raw != nil && !mapped && !toggle {
		return []string{"x-copilot.setup-steps must be a boolean or a mapping"}
	}
	var issues []string
	for _, key := range slices.Sorted(maps.Keys(options)) {
		v := options[key]
		var problem string
		switch key {
		case "runs-on":
			if !validRunner(v) {
				problem = "must be a label, a list of labels, or a mapping with group or labels"
			}
		case "permissions":
			if !validPermissions(v) {
				problem = "must be read-all, write-all, or a mapping of permissions to read, write, or none"
			}
		case "services":
			if _, ok := v.(map[string]any); !ok && !isActionsExpression(v) {
				problem = "must be a service mapping or a GitHub Actions expression"
			}
		case "snapshot":
			image, _ := v.(map[string]any)
			if !nonemptyText(v) && !nonemptyText(image["image-name"]) {
				problem = "must be an image name or a mapping with image-name"
			}
		case "timeout-minutes":
			timeout, ok := v.(int)
			if !ok || timeout < 1 || timeout > 59 {
				problem = "must be an integer from 1 to 59"
			}
		case "steps":
			steps, ok := v.([]any)
			if !ok {
				problem = "must be a list of workflow steps"
			}
			for _, step := range steps {
				fields, ok := step.(map[string]any)
				if !ok {
					problem = "must contain only workflow step mappings"
					break
				}
				uses, _ := fields["uses"].(string)
				if _, _, ok := splitVersionComment(uses); !ok {
					problem = "uses must be an action reference, optionally followed by ` # <version>`"
					break
				}
			}
		case "checkout":
			text, _ := v.(string)
			ref, _, ok := splitVersionComment(text)
			if _, err := hex.DecodeString(ref); !ok || len(ref) != 40 || err != nil {
				problem = "must be the full 40-character actions/checkout commit hash, optionally followed by ` # <version>`"
			}
		default:
			problem = "is not a supported Copilot setup job option"
		}
		if problem != "" {
			issues = append(issues, "x-copilot.setup-steps."+key+" "+problem)
		}
	}
	return issues
}

func validRunner(v any) bool {
	options, mapped := v.(map[string]any)
	if !mapped {
		return validRunnerLabels(v)
	}
	if len(options) == 0 {
		return false
	}
	for key, value := range options {
		switch key {
		case "group":
			if !nonemptyText(value) {
				return false
			}
		case "labels":
			if !validRunnerLabels(value) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func validRunnerLabels(v any) bool {
	labels, list := v.([]any)
	if !list {
		return nonemptyText(v)
	}
	for _, label := range labels {
		if !nonemptyText(label) {
			return false
		}
	}
	return len(labels) > 0
}

func validPermissions(v any) bool {
	switch permissions := v.(type) {
	case string:
		return permissions == "read-all" || permissions == "write-all"
	case map[string]any:
		for _, access := range permissions {
			switch access {
			case "read", "write", "none":
			default:
				return false
			}
		}
		return true
	default:
		return false
	}
}

func isActionsExpression(v any) bool {
	text, _ := v.(string)
	text = strings.TrimSpace(text)
	return strings.HasPrefix(text, "${{") && strings.HasSuffix(text, "}}")
}

func nonemptyText(v any) bool {
	text, _ := v.(string)
	return strings.TrimSpace(text) != ""
}

func emitSetupSteps(sess *emit.Session, envs []spec.Entry, cfg *config.Config, dryRun bool) error {
	var install string
	var optIn any
	fieldCounts := map[string]int{}
	for _, e := range envs {
		if issues := SetupStepsIssues(e); len(issues) > 0 {
			return fmt.Errorf("%s: %s", e.Path, strings.Join(issues, "; "))
		}
		if raw, set := setupStepsOverride(e); set {
			optIn = raw
		}
		for field, value := range emit.ResolveMeta(e.Meta, target) {
			switch field {
			case "install":
				install, _ = value.(string)
			case "name", "description", "scope", setupStepsKey:
			default:
				fieldCounts[field]++
			}
		}
	}
	for _, field := range slices.Sorted(maps.Keys(fieldCounts)) {
		emit.NoteFieldNoOp(target, spec.KindEnvironment, field, fieldCounts[field], "the Copilot setup workflow reads install")
	}
	if strings.TrimSpace(install) == "" {
		return nil
	}
	options, hasOptions := optIn.(map[string]any)
	if optIn != true && !hasOptions {
		emit.NoteFieldNoOp(target, spec.KindEnvironment, "install", 1, "set x-copilot.setup-steps: true to write the Copilot setup workflow")
		return nil
	}
	path := SetupStepsPath(cfg)
	raw, err := renderSetupWorkflow(setupWorkflow(install, options))
	if err != nil {
		return fmt.Errorf("render %s: %w", path, err)
	}
	return sess.WriteGeneratedFile(path, emit.WithHeader(string(raw), emit.FormatYAML), dryRun)
}

type setupStepsWorkflow struct {
	Name string
	On   map[string]any
	Jobs map[string]map[string]any
}

func setupWorkflow(install string, options map[string]any) setupStepsWorkflow {
	job := map[string]any{
		"permissions": map[string]string{"contents": "read"},
		"runs-on":     "ubuntu-latest",
	}
	ref := checkoutCommit + " # " + checkoutVersion
	var extraSteps []any
	for key, v := range options {
		switch key {
		case "checkout":
			ref, _ = v.(string)
		case "steps":
			extraSteps, _ = v.([]any)
		default:
			job[key] = v
		}
	}
	steps := []any{map[string]any{
		"uses": "actions/checkout@" + ref,
		"with": map[string]bool{"persist-credentials": false},
	}}
	steps = append(steps, extraSteps...)
	job["steps"] = append(steps, map[string]string{"name": "Install dependencies", "run": install})
	return setupStepsWorkflow{
		Name: "Copilot Setup Steps",
		On:   map[string]any{"workflow_dispatch": map[string]any{}},
		Jobs: map[string]map[string]any{"copilot-setup-steps": job},
	}
}

// renderSetupWorkflow marshals the workflow and turns each step's
// `uses: <ref> # <version>` value into the ref with a line comment.
func renderSetupWorkflow(workflow setupStepsWorkflow) ([]byte, error) {
	var doc yaml.Node
	if err := doc.Encode(workflow); err != nil {
		return nil, err
	}
	for _, job := range mappingValues(mappingValue(&doc, "jobs")) {
		steps := mappingValue(job, "steps")
		if steps == nil || steps.Kind != yaml.SequenceNode {
			continue
		}
		for _, step := range steps.Content {
			uses := mappingValue(step, "uses")
			if uses == nil || uses.Kind != yaml.ScalarNode {
				continue
			}
			if ref, version, ok := splitVersionComment(uses.Value); ok && version != "" {
				uses.Value, uses.LineComment, uses.Style = ref, "# "+version, 0
			}
		}
	}
	return yaml.Marshal(&doc)
}

func mappingValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func mappingValues(n *yaml.Node) []*yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	var values []*yaml.Node
	for i := 1; i < len(n.Content); i += 2 {
		values = append(values, n.Content[i])
	}
	return values
}
