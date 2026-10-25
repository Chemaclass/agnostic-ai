package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func runViewCmd(t *testing.T, args ...string) string {
	t.Helper()
	cmd := newViewCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("view %v: %v", args, err)
	}
	return out.String()
}

func TestNewViewCmd_PrintsJSONThatParsesAsAnOverviewView(t *testing.T) {
	dir := t.TempDir()
	writeBareReadProject(t, dir)
	before, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	out := runViewCmd(t, "--dir", dir, "--json")

	var view overview.View
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("output is not an overview view: %v\n%s", err, out)
	}
	if view.SchemaVersion != overview.SchemaVersion || len(view.Sections) != 5 || view.Snapshot.Project.Name != filepath.Base(dir) {
		t.Errorf("view = %+v", view)
	}
	if got := itemWithFinding(view, "LINT038"); got != "permissions" {
		t.Errorf("LINT038 sits on %q, want the permissions settings item", got)
	}
	if after, _ := os.Getwd(); after != before {
		t.Errorf("working directory moved from %s to %s", before, after)
	}
}

func TestView_HasNoSubcommands(t *testing.T) {
	for _, sub := range []string{"authority", "overview"} {
		_, err := runCLI(t, "view", sub)
		if err == nil || !strings.Contains(err.Error(), `unknown command "`+sub+`"`) {
			t.Errorf("view %s error = %v, want an unknown command", sub, err)
		}
	}
}

func TestView_DefaultSourceDoesNotMixProjectAndGlobalTargets(t *testing.T) {
	home := globalOverviewHome(t)
	project := t.TempDir()
	mustWrite(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\ntargets: [cursor, opencode]\n")
	nested := filepath.Join(project, "src", "nested")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	source := filepath.Join(home, "source")
	local := filepath.Join(source, "local")
	mustWrite(t, filepath.Join(local, "agnostic-ai.yaml"), "targets: [codex]\n")
	mustWrite(t, filepath.Join(local, "rules", "local-rule.md"), "---\nname: local-rule\n---\nLocal instructions.\n")
	localProject := filepath.Join(local, "projects", "service")
	mustWrite(t, filepath.Join(localProject, "agnostic-ai.yaml"), "version: 1\ntargets: [cursor, opencode]\n")
	localAlias := filepath.Join(t.TempDir(), "local-alias")
	localAliasErr := os.Symlink(local, localAlias)
	globalNested := filepath.Join(source, "rules")
	if err := os.MkdirAll(globalNested, 0755); err != nil {
		t.Fatal(err)
	}
	nestedProject := filepath.Join(source, "projects", "service")
	mustWrite(t, filepath.Join(nestedProject, "agnostic-ai.yaml"), "version: 1\ntargets: [cursor, opencode]\n")
	alias := filepath.Join(t.TempDir(), "alias")
	aliasErr := os.Symlink(nested, alias)

	for _, tc := range []struct {
		name, cwd     string
		args, targets []string
		global        bool
	}{

		{name: "global local root", cwd: local, targets: []string{"codex"}, global: true},
		{name: "global local child", cwd: filepath.Join(local, "rules"), targets: []string{"codex"}, global: true},
		{name: "global local physical alias", cwd: localAlias, targets: []string{"codex"}, global: true},
		{name: "project inside global local layer", cwd: localProject, targets: []string{"cursor", "opencode"}},
		{name: "explicit local directory as project", cwd: outside, args: []string{"--dir", local}, targets: []string{"codex"}},
		{name: "global source root", cwd: source, targets: []string{"codex"}, global: true},
		{name: "global source child", cwd: globalNested, targets: []string{"codex"}, global: true},
		{name: "project inside global source", cwd: nestedProject, targets: []string{"cursor", "opencode"}},
		{name: "explicit global directory as project", cwd: outside, args: []string{"--dir", source}, targets: []string{"claude", "codex"}},
		{name: "PWD symlink to nested directory", cwd: alias, targets: []string{"cursor", "opencode"}},
		{name: "outside", cwd: outside, targets: []string{"codex"}, global: true},
		{name: "project", cwd: project, targets: []string{"cursor", "opencode"}},
		{name: "nested", cwd: nested, targets: []string{"cursor", "opencode"}},
		{name: "explicit global", cwd: project, args: []string{"--global"}, targets: []string{"codex"}, global: true},
		{name: "explicit directory", cwd: outside, args: []string{"--dir", project}, targets: []string{"cursor", "opencode"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.cwd == localAlias && localAliasErr != nil {
				t.Skipf("directory symlink unavailable: %v", localAliasErr)
			}
			if tc.cwd == alias && aliasErr != nil {
				t.Skipf("directory symlink unavailable: %v", aliasErr)
			}
			testutil.Chdir(t, tc.cwd)
			t.Setenv("PWD", tc.cwd)
			var view overview.View
			if err := json.Unmarshal([]byte(runViewCmd(t, append(tc.args, "--json")...)), &view); err != nil {
				t.Fatal(err)
			}
			if err := checkViewDiscoveryScope(view, tc.targets, tc.global); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestView_RejectsInvalidSourcesBeforeOpeningBrowser(t *testing.T) {
	globalOverviewHome(t)
	outside := t.TempDir()
	invalid := t.TempDir()
	mustWrite(t, filepath.Join(invalid, "agnostic-ai.yaml"), "targets: [\n")
	nested := filepath.Join(invalid, "nested")
	if err := os.Mkdir(nested, 0755); err != nil {
		t.Fatal(err)
	}
	sourceOnly := t.TempDir()
	mustWrite(t, filepath.Join(sourceOnly, ".agnostic-ai", "rules", "example.md"), "---\nname: example\n---\nExample.\n")
	file := filepath.Join(outside, "file")
	mustWrite(t, file, "not a directory")
	for _, tc := range []struct {
		name, cwd, want string
		args            []string
	}{
		{name: "invalid project", cwd: invalid, want: "parse agnostic-ai.yaml"},
		{name: "nested invalid project", cwd: nested, want: "parse agnostic-ai.yaml"},
		{name: "missing directory", cwd: outside, args: []string{"--dir", filepath.Join(outside, "missing")}, want: "project directory"},
		{name: "file directory", cwd: outside, args: []string{"--dir", file}, want: "not a directory"},
		{name: "explicit unconfigured directory", cwd: outside, args: []string{"--dir", outside}, want: "no agnostic-ai.yaml"},
		{name: "source-only directory needs config", cwd: outside, args: []string{"--dir", sourceOnly}, want: "no agnostic-ai.yaml"},
		{name: "conflicting flags", cwd: outside, args: []string{"--global", "--dir", outside}, want: "cannot be combined"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.Chdir(t, tc.cwd)
			for _, asJSON := range []bool{false, true} {
				opened := false
				cmd := newViewCmdWithBrowser(func(string) error { opened = true; return nil })
				cmd.SetOut(io.Discard)
				cmd.SetErr(io.Discard)
				args := slices.Clone(tc.args)
				if asJSON {
					args = append(args, "--json")
				}
				cmd.SetArgs(args)
				if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Errorf("%v error = %v, want %q", args, err, tc.want)
				}
				if opened {
					t.Error("opened browser for invalid source")
				}
			}
		})
	}
}

func TestView_OpensBrowserWithBundledUIAndSelectedOverview(t *testing.T) {
	home := globalOverviewHome(t)
	project := t.TempDir()
	mustWrite(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\ntargets: [cursor, opencode]\n")
	nested := filepath.Join(project, "src")
	if err := os.Mkdir(nested, 0755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	source := filepath.Join(home, "source")
	local := filepath.Join(source, "local")
	mustWrite(t, filepath.Join(local, "agnostic-ai.yaml"), "targets: [codex]\n")
	mustWrite(t, filepath.Join(local, "rules", "local-rule.md"), "---\nname: local-rule\n---\nLocal instructions.\n")
	localProject := filepath.Join(local, "projects", "service")
	mustWrite(t, filepath.Join(localProject, "agnostic-ai.yaml"), "version: 1\ntargets: [cursor, opencode]\n")
	localAlias := filepath.Join(t.TempDir(), "local-alias")
	localAliasErr := os.Symlink(local, localAlias)
	globalNested := filepath.Join(source, "rules")
	if err := os.MkdirAll(globalNested, 0755); err != nil {
		t.Fatal(err)
	}
	nestedProject := filepath.Join(source, "projects", "service")
	mustWrite(t, filepath.Join(nestedProject, "agnostic-ai.yaml"), "version: 1\ntargets: [cursor, opencode]\n")
	alias := filepath.Join(t.TempDir(), "alias")
	aliasErr := os.Symlink(nested, alias)

	for _, tc := range []struct {
		name, cwd     string
		args, targets []string
		global        bool
	}{

		{name: "global local root", cwd: local, targets: []string{"codex"}, global: true},
		{name: "global local child", cwd: filepath.Join(local, "rules"), targets: []string{"codex"}, global: true},
		{name: "global local physical alias", cwd: localAlias, targets: []string{"codex"}, global: true},
		{name: "project inside global local layer", cwd: localProject, targets: []string{"cursor", "opencode"}},
		{name: "explicit local directory as project", cwd: outside, args: []string{"--dir", local}, targets: []string{"codex"}},
		{name: "global source root", cwd: source, targets: []string{"codex"}, global: true},
		{name: "global source child", cwd: globalNested, targets: []string{"codex"}, global: true},
		{name: "project inside global source", cwd: nestedProject, targets: []string{"cursor", "opencode"}},
		{name: "explicit global directory as project", cwd: outside, args: []string{"--dir", source}, targets: []string{"claude", "codex"}},
		{name: "PWD symlink to nested directory", cwd: alias, targets: []string{"cursor", "opencode"}},
		{name: "outside", cwd: outside, targets: []string{"codex"}, global: true},
		{name: "project", cwd: project, targets: []string{"cursor", "opencode"}},
		{name: "nested", cwd: nested, targets: []string{"cursor", "opencode"}},
		{name: "explicit global", cwd: project, args: []string{"--global"}, targets: []string{"codex"}, global: true},
		{name: "explicit directory", cwd: outside, args: []string{"--dir", project}, targets: []string{"cursor", "opencode"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.cwd == localAlias && localAliasErr != nil {
				t.Skipf("directory symlink unavailable: %v", localAliasErr)
			}
			if tc.cwd == alias && aliasErr != nil {
				t.Skipf("directory symlink unavailable: %v", aliasErr)
			}
			testutil.Chdir(t, tc.cwd)
			t.Setenv("PWD", tc.cwd)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			result := make(chan error, 1)
			opened := 0
			cmd := newViewCmdWithBrowser(func(url string) error {
				opened++
				go func() {
					defer cancel()
					client := &http.Client{Timeout: 5 * time.Second}
					resp, err := client.Get(url)
					if err != nil {
						result <- err
						return
					}
					data, err := io.ReadAll(resp.Body)
					_ = resp.Body.Close()
					if err != nil || resp.StatusCode != http.StatusOK || !strings.Contains(string(data), "<html") {
						result <- fmt.Errorf("UI status %d, read error %v", resp.StatusCode, err)
						return
					}
					resp, err = client.Get(url + "api/views/overview")
					if err != nil {
						result <- err
						return
					}
					var view overview.View
					err = json.NewDecoder(resp.Body).Decode(&view)
					_ = resp.Body.Close()
					if err != nil || resp.StatusCode != http.StatusOK {
						result <- fmt.Errorf("overview status %d, error %v", resp.StatusCode, err)
						return
					}
					if err := checkViewDiscoveryScope(view, tc.targets, tc.global); err != nil {
						result <- err
						return
					}
					result <- checkViewScopes(client, url, tc.global)
				}()
				return nil
			})
			cmd.SetContext(ctx)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(append(slices.Clone(tc.args), "--addr", "127.0.0.1:0"))
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if opened != 1 {
				t.Fatalf("opened browser %d times, want 1", opened)
			}
			select {
			case err := <-result:
				if err != nil {
					t.Fatal(err)
				}
			default:
				t.Fatal("browser did not load the UI and overview")
			}
		})
	}
}

func TestView_BrowserFailureKeepsURLAvailable(t *testing.T) {
	globalOverviewHome(t)
	testutil.Chdir(t, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out, warnings bytes.Buffer
	cmd := newViewCmdWithBrowser(func(string) error { cancel(); return fmt.Errorf("browser unavailable") })
	cmd.SetContext(ctx)
	cmd.SetOut(&out)
	cmd.SetErr(&warnings)
	cmd.SetArgs([]string{"--addr", "127.0.0.1:0"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Serving agnostic-ai at http://127.0.0.1:") || !strings.Contains(warnings.String(), "could not open a browser: browser unavailable") {
		t.Errorf("output = %q, warning = %q", out.String(), warnings.String())
	}
}

func checkViewDiscoveryScope(view overview.View, targets []string, global bool) error {
	if !slices.Equal(view.Snapshot.Targets, targets) || strings.HasPrefix(view.Snapshot.Project.Name, "global (") != global {
		return fmt.Errorf("snapshot = %+v, want targets %v, global %v", view.Snapshot, targets, global)
	}
	shared, local := false, false
	for _, section := range view.Sections {
		for _, item := range section.Items {
			if item.Kind == "skill" && item.Name == "review" {
				shared = true
			}
			if item.Kind == "rule" && item.Name == "local-rule" {
				local = true
			}
		}
	}
	if shared != global || local != global {
		return fmt.Errorf("shared/local specs = %v/%v, want both %v", shared, local, global)
	}
	return nil
}

// checkViewScopes asks the server view started which configurations it
// reads: a project with the global configuration to switch to, or the
// global configuration alone.
func checkViewScopes(client *http.Client, url string, global bool) error {
	resp, err := client.Get(url + "api/scopes")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	var listing struct {
		Default string `json:"default"`
		Scopes  []struct {
			ID string `json:"id"`
		} `json:"scopes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
		return fmt.Errorf("scopes: %w", err)
	}
	want := []string{scopeProject, scopeGlobal}
	if global {
		want = []string{scopeGlobal}
	}
	var got []string
	for _, s := range listing.Scopes {
		got = append(got, s.ID)
	}
	if listing.Default != want[0] || !slices.Equal(got, want) {
		return fmt.Errorf("scopes = %s %v, want %s %v", listing.Default, got, want[0], want)
	}
	return nil
}
