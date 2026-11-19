package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApprovalsPolicy_ResolveDefaultsToAsk(t *testing.T) {
	got := ApprovalsPolicy{}.Resolve("claude", "/work/app")

	if got.Mode != ApprovalsAsk || got.From != ApprovalsFromDefault {
		t.Fatalf("got %+v, want ask from the default", got)
	}
}

func TestApprovalsPolicy_ResolveOrder(t *testing.T) {
	home := ApprovalsConfig{
		Mode:     ApprovalsReview,
		Tools:    map[string]string{"codex": ApprovalsYolo, "claude": ApprovalsAsk},
		Projects: map[string]string{"~/Code/sandbox": ApprovalsYolo, "/work/locked": ApprovalsAsk},
	}
	cases := []struct {
		name   string
		policy ApprovalsPolicy
		tool   string
		dir    string
		want   ApprovalDecision
	}{
		{"mode", ApprovalsPolicy{Home: home}, "cursor", "/work/app", ApprovalDecision{ApprovalsReview, "approvals.mode"}},
		{"tools replace mode", ApprovalsPolicy{Home: home}, "codex", "/work/app", ApprovalDecision{ApprovalsYolo, "approvals.tools.codex"}},
		{"projects replace tools", ApprovalsPolicy{Home: home}, "codex", "/work/locked/sub", ApprovalDecision{ApprovalsAsk, "approvals.projects[/work/locked]"}},
		{"projects expand the home folder", ApprovalsPolicy{Home: home, UserHome: "/u"}, "claude", "/u/Code/sandbox", ApprovalDecision{ApprovalsYolo, "approvals.projects[~/Code/sandbox]"}},
		{"a sibling folder is not inside", ApprovalsPolicy{Home: home, UserHome: "/u"}, "claude", "/u/Code/sandbox2", ApprovalDecision{ApprovalsAsk, "approvals.tools.claude"}},
		{"project file says ask", ApprovalsPolicy{Home: home, UserHome: "/u", ProjectAsk: "/u/Code/sandbox/agnostic-ai.yaml"}, "codex", "/u/Code/sandbox", ApprovalDecision{ApprovalsAsk, "/u/Code/sandbox/agnostic-ai.yaml"}},
		{"environment says ask", ApprovalsPolicy{Home: home, UserHome: "/u", Env: "ask"}, "codex", "/u/Code/sandbox", ApprovalDecision{ApprovalsAsk, EnvApprovals}},
		{"environment cannot loosen", ApprovalsPolicy{Home: ApprovalsConfig{}, Env: "yolo"}, "codex", "/work/app", ApprovalDecision{ApprovalsAsk, ApprovalsFromDefault}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.policy.Resolve(c.tool, filepath.FromSlash(c.dir)); got != c.want {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestApprovalsPolicy_LongestProjectWins(t *testing.T) {
	policy := ApprovalsPolicy{Home: ApprovalsConfig{Projects: map[string]string{
		"/work":         ApprovalsYolo,
		"/work/release": ApprovalsAsk,
	}}}

	got := policy.Resolve("claude", filepath.FromSlash("/work/release/app"))

	if got.Mode != ApprovalsAsk {
		t.Fatalf("got %+v, want the deeper project's ask", got)
	}
}

func TestApprovalsPolicy_InvalidHomeValueAsks(t *testing.T) {
	policy := ApprovalsPolicy{Home: ApprovalsConfig{Mode: "always"}}

	if got := policy.Resolve("claude", "/work"); got.Mode != ApprovalsAsk {
		t.Fatalf("got %+v, want ask", got)
	}
}

func writeApprovalsFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadHomeApprovals_LocalReplacesShared(t *testing.T) {
	source := t.TempDir()
	writeApprovalsFile(t, filepath.Join(source, ConfigFileName), "version: 1\napprovals:\n  mode: yolo\n  tools:\n    codex: review\n")
	writeApprovalsFile(t, filepath.Join(source, "local", ConfigFileName), "approvals:\n  mode: ask\n")

	got, err := LoadHomeApprovals(source)
	if err != nil {
		t.Fatal(err)
	}

	if got.Mode != ApprovalsAsk || len(got.Tools) != 0 {
		t.Fatalf("got %+v, want the local block alone", got)
	}
}

func TestLoadHomeApprovals_MissingFilesAreEmpty(t *testing.T) {
	got, err := LoadHomeApprovals(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if got.Mode != "" || got.Tools != nil || got.Projects != nil || got.AlwaysAsk != nil {
		t.Fatalf("got %+v, want an empty block", got)
	}
}

func TestLoadHomeApprovals_RejectsUnknownKeyAndValue(t *testing.T) {
	for body, want := range map[string]string{
		"approvals:\n  mod: yolo\n":             "mod",
		"approvals:\n  mode: always\n":          `"always" is not one of ask, review, yolo`,
		"approvals:\n  tools:\n    codex: on\n": "approvals.tools.codex",
	} {
		source := t.TempDir()
		writeApprovalsFile(t, filepath.Join(source, ConfigFileName), body)

		_, err := LoadHomeApprovals(source)

		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %v, want an error naming %q", body, err, want)
		}
	}
}

func TestProjectApprovalsAsk_FindsAskInAnAncestor(t *testing.T) {
	root := t.TempDir()
	writeApprovalsFile(t, filepath.Join(root, LocalOverrideFileName), "approvals:\n  mode: ask\n")
	dir := filepath.Join(root, "pkg", "sub")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := ProjectApprovalsAsk(dir)
	if err != nil {
		t.Fatal(err)
	}

	if got != filepath.Join(root, LocalOverrideFileName) {
		t.Fatalf("got %q, want the local file", got)
	}
}

func TestProjectApprovalsAsk_IgnoresLooseningValues(t *testing.T) {
	root := t.TempDir()
	writeApprovalsFile(t, filepath.Join(root, ConfigFileName), "version: 1\napprovals:\n  mode: yolo\n")

	got, err := ProjectApprovalsAsk(root)
	if err != nil {
		t.Fatal(err)
	}

	if got != "" {
		t.Fatalf("got %q, want no project ask", got)
	}
}

func TestProjectApprovalsIssues(t *testing.T) {
	cases := map[string]struct {
		shared, local string
		want          []string
	}{
		"no approvals":       {"version: 1\n", "", nil},
		"ask is allowed":     {"version: 1\napprovals:\n  mode: ask\n", "approvals:\n  mode: ask\n", nil},
		"yolo in shared":     {"version: 1\napprovals:\n  mode: yolo\n", "", []string{ConfigFileName + ": approvals.mode: yolo"}},
		"review in local":    {"version: 1\n", "approvals:\n  mode: review\n", []string{LocalOverrideFileName + ": approvals.mode: review"}},
		"tools in a project": {"version: 1\napprovals:\n  tools:\n    codex: ask\n", "", []string{ConfigFileName + ": approvals.tools"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeApprovalsFile(t, filepath.Join(root, ConfigFileName), c.shared)
			if c.local != "" {
				writeApprovalsFile(t, filepath.Join(root, LocalOverrideFileName), c.local)
			}

			issues, err := ProjectApprovalsIssues(root)
			if err != nil {
				t.Fatal(err)
			}

			if len(issues) != len(c.want) {
				t.Fatalf("got %+v, want %d issue(s)", issues, len(c.want))
			}
			for i, want := range c.want {
				got := filepath.Base(issues[i].Path) + ": " + issues[i].Message
				if !strings.HasPrefix(got, want) {
					t.Errorf("issue %d: got %q, want prefix %q", i, got, want)
				}
			}
		})
	}
}

func TestLoad_ApprovalsWithoutTheKeyResolvesToAsk(t *testing.T) {
	root := t.TempDir()
	writeApprovalsFile(t, filepath.Join(root, ConfigFileName), "version: 1\n")

	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if got := (ApprovalsPolicy{Home: cfg.Approvals}).Resolve("claude", root); got.Mode != ApprovalsAsk {
		t.Fatalf("got %+v, want ask", got)
	}
}

func TestLoad_RejectsUnknownApprovalsMode(t *testing.T) {
	root := t.TempDir()
	writeApprovalsFile(t, filepath.Join(root, ConfigFileName), "version: 1\napprovals:\n  mode: always\n")

	_, err := Load(root)

	if err == nil || !strings.Contains(err.Error(), "approvals.mode") {
		t.Fatalf("got %v, want an approvals.mode error", err)
	}
}
