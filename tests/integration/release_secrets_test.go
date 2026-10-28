package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	uiBundleJob   = "ui-bundle"
	uiSmokeJob    = "ui-smoke"
	goreleaserJob = "goreleaser"
	// npmPublishJob publishes to npm, which needs Node, the npm token, and
	// id-token for provenance. It runs only this repository's scripts and
	// an npm CLI checked against a committed hash, never a lockfile.
	npmPublishJob = "npm"
)

var (
	runsNodeRE = regexp.MustCompile(`(^|[\s;&|(])(npm|npx|node|pnpm|yarn|bun|corepack)(\s|$)|node_modules/\.bin`)
	npmCIRE    = regexp.MustCompile(`npm[^\n]*\sci(\s|$)`)
	// A token passed as a value. `secrets.X != ''` only records presence.
	tokenValueRE = regexp.MustCompile(`\$\{\{\s*(secrets\.[A-Za-z0-9_]+|github\.token)\s*\}\}`)
)

func stepRunsNode(s workflowStep) bool {
	return runsNodeRE.MatchString(s.Run) || strings.HasPrefix(s.Uses, "actions/setup-node@")
}

// TestGoreleaser_RunsNoNodeHooks keeps third-party UI code out of the
// GoReleaser process.
//
// GoReleaser hooks inherit its environment, which holds GITHUB_TOKEN, the
// package-manager tokens, and the OIDC request token. `npm ci` runs
// dependency install scripts and the bundle check runs Vite and every
// plugin in the lockfile, so a bad dependency bump would run with all of
// them at the next tag.
func TestGoreleaser_RunsNoNodeHooks(t *testing.T) {
	data, err := os.ReadFile(goreleaserConfigPath)
	if err != nil {
		t.Fatalf("read %s: %v", goreleaserConfigPath, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", goreleaserConfigPath, err)
	}
	for _, hook := range hookCommands(&doc, false) {
		if runsNodeRE.MatchString(hook) {
			t.Errorf("a GoReleaser hook runs %q, where the publish tokens are in the environment", hook)
		}
	}
}

// hookCommands returns every string under a `hooks` key at any depth:
// `before`, `after`, and per-build `pre` and `post` hooks alike.
func hookCommands(n *yaml.Node, inHooks bool) []string {
	if n.Kind == yaml.ScalarNode {
		if inHooks {
			return []string{n.Value}
		}
		return nil
	}
	var out []string
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			out = append(out, hookCommands(n.Content[i+1], inHooks || n.Content[i].Value == "hooks")...)
		}
		return out
	}
	for _, c := range n.Content {
		out = append(out, hookCommands(c, inHooks)...)
	}
	return out
}

// TestReleaseWorkflow_KeepsNodeAwayFromPublishCredentials checks that no
// step running npm or Node shares a job with the release credentials.
//
// A job with `id-token: write` hands the OIDC request token to every
// step, and a token in a job or step env reaches anything that step
// starts, so the only boundary that holds is a job without them.
func TestReleaseWorkflow_KeepsNodeAwayFromPublishCredentials(t *testing.T) {
	jobs := workflowJobs(t, releaseWorkflowPath)
	for _, required := range []string{goreleaserJob, uiBundleJob, uiSmokeJob} {
		if _, ok := jobs[required]; !ok {
			t.Errorf("release.yml has no %s job", required)
		}
	}
	for name, job := range jobs {
		if name == npmPublishJob {
			continue
		}
		var nodeSteps []workflowStep
		for _, s := range job.Steps {
			if stepRunsNode(s) {
				nodeSteps = append(nodeSteps, s)
			}
		}
		if len(nodeSteps) == 0 {
			continue
		}
		for scope, access := range job.Permissions {
			if access == "write" {
				t.Errorf("the %s job runs npm or Node and has %s: write", name, scope)
			}
		}
		for key, value := range job.Env {
			if tokenValueRE.MatchString(value) {
				t.Errorf("the %s job runs npm or Node and passes a token in its env %s", name, key)
			}
		}
		for _, s := range nodeSteps {
			for key, value := range s.Env {
				if tokenValueRE.MatchString(value) {
					t.Errorf("the %s job step %q runs npm or Node with a token in %s", name, s.Name, key)
				}
			}
		}
	}
}

// TestReleaseWorkflow_BuildsFromACleanGoCache keeps a cache entry saved
// by an earlier job in the same tag run out of the signed build.
func TestReleaseWorkflow_BuildsFromACleanGoCache(t *testing.T) {
	for _, s := range workflowJobs(t, releaseWorkflowPath)[goreleaserJob].Steps {
		if !strings.HasPrefix(s.Uses, "actions/setup-go@") {
			continue
		}
		if cache, ok := s.With["cache"].(bool); !ok || cache {
			t.Errorf("the %s job step %q must set cache: false", goreleaserJob, s.Name)
		}
	}
}

// TestReleaseWorkflow_ChecksTheBundleBeforePublishing keeps the release
// failing when the committed UI bundle does not match its sources, now
// that the check no longer runs inside GoReleaser.
func TestReleaseWorkflow_ChecksTheBundleBeforePublishing(t *testing.T) {
	if needs := jobNeeds(t, releaseWorkflowPath, goreleaserJob); !slices.Contains(needs, uiBundleJob) {
		t.Errorf("the %s job does not need %s, so a stale bundle would ship, got needs %v", goreleaserJob, uiBundleJob, needs)
	}
	var install, check bool
	for _, s := range workflowJobs(t, releaseWorkflowPath)[uiBundleJob].Steps {
		if npmCIRE.MatchString(s.Run) {
			install = true
			if !strings.Contains(s.Run, "--ignore-scripts") {
				t.Errorf("step %q installs the UI dependencies with their lifecycle scripts:\n%s", s.Name, s.Run)
			}
		}
		if strings.Contains(s.Run, "bundle:check") {
			check = true
		}
	}
	if !install || !check {
		t.Errorf("the %s job must install the UI dependencies and run bundle:check (install=%v, check=%v)", uiBundleJob, install, check)
	}
}

// jobNeeds returns the jobs the named job waits for. `needs` is a string
// or a list.
func jobNeeds(t *testing.T, path, job string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc struct {
		Jobs map[string]struct {
			Needs yaml.Node `yaml:"needs"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	node := doc.Jobs[job].Needs
	if node.Kind == yaml.ScalarNode {
		return []string{node.Value}
	}
	var needs []string
	if err := node.Decode(&needs); err != nil && node.Kind != 0 {
		t.Fatalf("decode needs of %s: %v", job, err)
	}
	return needs
}

// TestWorkflows_CheckoutsDoNotPersistCredentials keeps the job token out
// of `.git/config`, where every later step could read it. No workflow
// here pushes with git.
func TestWorkflows_CheckoutsDoNotPersistCredentials(t *testing.T) {
	paths, err := filepath.Glob(filepath.FromSlash("../../.github/workflows/*.yml"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no workflows found: %v", err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		// Steps only: other job keys, such as an expression matrix, do not
		// fit workflowJob.
		var doc struct {
			Jobs map[string]struct {
				Steps []workflowStep `yaml:"steps"`
			} `yaml:"jobs"`
		}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for name, job := range doc.Jobs {
			for i, s := range job.Steps {
				if !strings.HasPrefix(s.Uses, "actions/checkout@") {
					continue
				}
				if persist, ok := s.With["persist-credentials"].(bool); !ok || persist {
					t.Errorf("%s job %q checkout (step %d) must set persist-credentials: false", filepath.Base(path), name, i+1)
				}
			}
		}
	}
}
