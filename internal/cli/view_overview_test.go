package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
	webui "github.com/chemaclass/agnostic-ai/ui"
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
					result <- nil
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

func TestView_ReusesOnlyMatchingServer(t *testing.T) {
	globalOverviewHome(t)
	globalRoot, err := globalSourceRoot()
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	mustWrite(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	other := t.TempDir()
	mustWrite(t, filepath.Join(other, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	for _, tc := range []struct {
		name      string
		server    *viewServer
		args      []string
		wantError string
		wantOpen  bool
		wantProbe bool
	}{
		{name: "same project", server: &viewServer{OK: true, Root: project, UI: true}, args: []string{"--dir", project}, wantOpen: true, wantProbe: true},
		{name: "other project", server: &viewServer{OK: true, Root: other, UI: true}, args: []string{"--dir", project, "--addr", defaultServeAddr}, wantError: other, wantProbe: true},
		{name: "other scope", server: &viewServer{OK: true, Root: project, Global: true, UI: true}, args: []string{"--dir", project, "--addr", defaultServeAddr}, wantError: project, wantProbe: true},
		{name: "same global", server: &viewServer{OK: true, Root: globalRoot, Global: true, UI: true}, args: []string{"--global"}, wantOpen: true, wantProbe: true},
		{name: "project on global port", server: &viewServer{OK: true, Root: project, UI: true}, args: []string{"--global", "--addr", defaultServeAddr}, wantError: project, wantProbe: true},
		{name: "api-only server", server: &viewServer{OK: true, Root: project}, args: []string{"--dir", project, "--addr", defaultServeAddr}, wantError: "without the web interface", wantProbe: true},
		{name: "no server", args: []string{"--dir", project, "--addr", "127.0.0.1:0"}, wantOpen: true, wantProbe: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opened, probed := false, false
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cmd := newViewCmdWithBrowserAndProbe(func(string) error { opened = true; cancel(); return nil }, func(string, time.Duration) *viewServer { probed = true; return tc.server })
			cmd.SetContext(ctx)
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if tc.wantError == "" && err != nil {
				t.Fatal(err)
			}
			if tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError) || !strings.Contains(err.Error(), "--addr")) {
				t.Errorf("error = %v", err)
			}
			if opened != tc.wantOpen || probed != tc.wantProbe {
				t.Errorf("opened=%t probed=%t", opened, probed)
			}
			if tc.server != nil && tc.wantOpen && !strings.Contains(out.String(), "Using agnostic-ai at") {
				t.Errorf("output = %q", out.String())
			}
		})
	}
}

func TestView_ReusesSameProjectSpelledInOtherCase(t *testing.T) {
	project := filepath.Join(t.TempDir(), "MyProj")
	mustWrite(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	lower := filepath.Join(filepath.Dir(project), "myproj")
	if _, err := os.Stat(lower); err != nil {
		t.Skip("the file system is case-sensitive")
	}
	opened := false
	cmd := newViewCmdWithBrowserAndProbe(func(string) error { opened = true; return nil }, func(string, time.Duration) *viewServer {
		return &viewServer{OK: true, Root: lower, UI: true}
	})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--dir", project})
	if err := cmd.Execute(); err != nil || !opened {
		t.Fatalf("opened=%t err=%v", opened, err)
	}
}

func TestView_ReusesGlobalServerForRelativeHome(t *testing.T) {
	source := filepath.Join(globalOverviewHome(t), "source")
	t.Chdir(filepath.Dir(source))
	t.Setenv("AGNOSTIC_AI_HOME", filepath.Base(source))
	opened := false
	cmd := newViewCmdWithBrowserAndProbe(func(string) error { opened = true; return nil }, func(string, time.Duration) *viewServer {
		return &viewServer{OK: true, Root: source, Global: true, UI: true}
	})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--global"})
	if err := cmd.Execute(); err != nil || !opened {
		t.Fatalf("opened=%t err=%v", opened, err)
	}
}

func TestView_ProbesAgainWhenTheAddressIsTaken(t *testing.T) {
	project := t.TempDir()
	mustWrite(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	for _, tc := range []struct {
		name      string
		late      *viewServer
		wantError string
	}{
		{name: "slow matching server", late: &viewServer{OK: true, Root: project, UI: true}},
		{name: "unrelated service", wantError: "listen on"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var timeouts []time.Duration
			opened := false
			cmd := newViewCmdWithBrowserAndProbe(func(string) error { opened = true; return nil }, func(_ string, timeout time.Duration) *viewServer {
				timeouts = append(timeouts, timeout)
				if len(timeouts) == 1 {
					return nil
				}
				return tc.late
			})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs([]string{"--dir", project, "--addr", ln.Addr().String()})
			err := cmd.Execute()
			if len(timeouts) != 2 || timeouts[1] <= timeouts[0] {
				t.Fatalf("probe timeouts = %v", timeouts)
			}
			if tc.wantError == "" && (err != nil || !opened) {
				t.Errorf("opened=%t err=%v", opened, err)
			}
			if tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError) || opened) {
				t.Errorf("opened=%t err=%v", opened, err)
			}
		})
	}
}

func TestProbeViewServer_ReadsTheRealHealthEndpoint(t *testing.T) {
	project := t.TempDir()
	mustWrite(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	src, err := openAuthoritySource(project, false)
	if err != nil {
		t.Fatal(err)
	}
	defer src.close()
	srv := httptest.NewServer(newServeMux(src.load, authority.HandlerOptions{Root: src.root, ConfigFiles: src.configFiles, UIFiles: webui.Files()}))
	defer srv.Close()
	got := probeViewServer(strings.TrimPrefix(srv.URL, "http://"), time.Second)
	if got == nil || got.Global || !got.UI || !sameProjectRoot(got.Root, project) {
		t.Fatalf("probe = %+v", got)
	}
}

func TestServeMux_HealthSkipsTheAPIQueue(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(serializeAPI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/slow" {
			close(started)
			<-release
		}
	})))
	defer srv.Close()
	defer close(release)
	go func() {
		if resp, err := http.Get(srv.URL + "/api/slow"); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-started
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get(srv.URL + authority.HealthPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
}

func TestUIAlias_InHelpAndCompletion(t *testing.T) {
	root := NewRootCmd("test")
	cmd, _, err := root.Find([]string{"ui"})
	if err != nil || cmd.Name() != "view" {
		t.Fatalf("ui resolves to %v, %v", cmd, err)
	}
	if !slices.Contains(cmd.Aliases, "ui") {
		t.Errorf("aliases = %v", cmd.Aliases)
	}
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "alias: ui") {
		t.Error("ui missing from root help")
	}
	out.Reset()
	root = NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{cobra.ShellCompRequestCmd, "u"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(strings.Split(out.String(), "\n"), "ui\t"+cmd.Short) {
		t.Errorf("completion for u = %q", out.String())
	}
}

func TestProbeViewServer_IgnoresUnrelatedService(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()
	if got := probeViewServer(strings.TrimPrefix(srv.URL, "http://"), time.Second); got != nil {
		t.Errorf("probe = %+v", got)
	}
}

func freeLoopbackAddrs(t *testing.T, n int) []string {
	t.Helper()
	var addrs []string
	for range n {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addrs = append(addrs, ln.Addr().String())
		_ = ln.Close()
	}
	return addrs
}

func runViewOnPorts(t *testing.T, ports []string, servers map[string]*viewServer, args ...string) (string, string, error) {
	t.Helper()
	return runViewOnPortsWithProbe(t, ports, func(addr string, _ time.Duration) *viewServer { return servers[addr] }, args...)
}

func runViewOnPortsWithProbe(t *testing.T, ports []string, probe func(string, time.Duration) *viewServer, args ...string) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opened := ""
	cmd := newViewCmdWithPorts(func(url string) error { opened = url; cancel(); return nil }, probe, ports)
	cmd.SetContext(ctx)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return opened, out.String(), err
}

func TestView_OpensAnotherConfigurationOnTheNextFreePort(t *testing.T) {
	globalOverviewHome(t)
	globalRoot, err := globalSourceRoot()
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	mustWrite(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	other := t.TempDir()
	mustWrite(t, filepath.Join(other, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	for _, tc := range []struct {
		name    string
		running *viewServer
		args    []string
	}{
		{name: "global after a project", running: &viewServer{OK: true, Root: project, UI: true}, args: []string{"--global"}},
		{name: "project after global", running: &viewServer{OK: true, Root: globalRoot, Global: true, UI: true}, args: []string{"--dir", project}},
		{name: "second project", running: &viewServer{OK: true, Root: other, UI: true}, args: []string{"--dir", project}},
		{name: "api-only server of the same project", running: &viewServer{OK: true, Root: project}, args: []string{"--dir", project}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ports := freeLoopbackAddrs(t, 2)
			opened, out, err := runViewOnPorts(t, ports, map[string]*viewServer{ports[0]: tc.running}, tc.args...)
			if err != nil {
				t.Fatal(err)
			}
			want := "http://" + ports[1] + "/"
			if opened != want || !strings.Contains(out, "Serving agnostic-ai at "+want) {
				t.Errorf("opened %q, output %q, want %s", opened, out, want)
			}
		})
	}
}

func TestView_ReusesMatchingServerOnALaterPort(t *testing.T) {
	globalOverviewHome(t)
	globalRoot, err := globalSourceRoot()
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	mustWrite(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	ports := freeLoopbackAddrs(t, 3)
	servers := map[string]*viewServer{
		ports[1]: {OK: true, Root: project, UI: true},
		ports[2]: {OK: true, Root: globalRoot, Global: true, UI: true},
	}
	opened, out, err := runViewOnPorts(t, ports, servers, "--global")
	if err != nil {
		t.Fatal(err)
	}
	want := "http://" + ports[2] + "/"
	if opened != want || !strings.Contains(out, "Using agnostic-ai at "+want) || strings.Contains(out, "Serving") {
		t.Errorf("opened %q, output %q, want reuse of %s", opened, out, want)
	}
}

func TestView_SkipsAPortHeldByAnotherProcess(t *testing.T) {
	project := t.TempDir()
	mustWrite(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()
	ports := append([]string{held.Addr().String()}, freeLoopbackAddrs(t, 1)...)
	opened, _, err := runViewOnPorts(t, ports, nil, "--dir", project)
	if err != nil {
		t.Fatal(err)
	}
	if want := "http://" + ports[1] + "/"; opened != want {
		t.Errorf("opened %q, want %s", opened, want)
	}
}

func TestView_FailsWhenEveryPortServesAnotherConfiguration(t *testing.T) {
	project := t.TempDir()
	mustWrite(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	other := t.TempDir()
	ports := freeLoopbackAddrs(t, 2)
	servers := map[string]*viewServer{
		ports[0]: {OK: true, Root: other, UI: true},
		ports[1]: {OK: true, Root: other, UI: true},
	}
	opened, _, err := runViewOnPorts(t, ports, servers, "--dir", project)
	if err == nil || !strings.Contains(err.Error(), ports[0]) || !strings.Contains(err.Error(), ports[1]) || !strings.Contains(err.Error(), "--addr") {
		t.Errorf("error = %v", err)
	}
	if opened != "" {
		t.Errorf("opened %q", opened)
	}
}

func TestView_ExplicitAddrDoesNotMoveToAnotherPort(t *testing.T) {
	project := t.TempDir()
	mustWrite(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	other := t.TempDir()
	ports := freeLoopbackAddrs(t, 2)
	opened, _, err := runViewOnPorts(t, ports, map[string]*viewServer{ports[0]: {OK: true, Root: other, UI: true}}, "--dir", project, "--addr", ports[0])
	if err == nil || !strings.Contains(err.Error(), other) {
		t.Errorf("error = %v", err)
	}
	if opened != "" {
		t.Errorf("opened %q", opened)
	}
}

func TestViewPortCandidates_ScanAFixedLoopbackRange(t *testing.T) {
	got := viewPortCandidates()
	if len(got) != viewPortScan || got[0] != defaultServeAddr || got[len(got)-1] != fmt.Sprintf("127.0.0.1:%d", 4310+viewPortScan-1) {
		t.Errorf("candidates = %v", got)
	}
}

func TestView_ScanRechecksATakenPortAfterTheBindFails(t *testing.T) {
	project := t.TempDir()
	mustWrite(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	other := t.TempDir()
	for _, tc := range []struct {
		name     string
		late     *viewServer
		wantPort int
		wantText string
	}{
		{name: "another configuration started", late: &viewServer{OK: true, Root: other, UI: true}, wantPort: 1, wantText: "Serving agnostic-ai at "},
		{name: "the same configuration started", late: &viewServer{OK: true, Root: project, UI: true}, wantPort: 0, wantText: "Using agnostic-ai at "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			held, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = held.Close() }()
			ports := append([]string{held.Addr().String()}, freeLoopbackAddrs(t, 1)...)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opened := ""
			probes := map[string]int{}
			cmd := newViewCmdWithPorts(func(url string) error { opened = url; cancel(); return nil }, func(addr string, _ time.Duration) *viewServer {
				probes[addr]++
				if addr == ports[0] && probes[addr] > 1 {
					return tc.late
				}
				return nil
			}, ports)
			cmd.SetContext(ctx)
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(io.Discard)
			cmd.SetArgs([]string{"--dir", project})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			want := "http://" + ports[tc.wantPort] + "/"
			if opened != want || !strings.Contains(out.String(), tc.wantText+want) {
				t.Errorf("opened %q, output %q, want %s", opened, out.String(), want)
			}
			if probes[ports[0]] != 2 {
				t.Errorf("probes = %v, want a recheck of %s", probes, ports[0])
			}
		})
	}
}

func TestView_ScanKeepsTheBindErrorWhenNoPortOpens(t *testing.T) {
	project := t.TempDir()
	mustWrite(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	var ports []string
	for range 2 {
		held, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = held.Close() }()
		ports = append(ports, held.Addr().String())
	}
	_, _, err := runViewOnPorts(t, ports, nil, "--dir", project)
	var listenErr *net.OpError
	if !errors.As(err, &listenErr) || listenErr.Op != "listen" {
		t.Fatalf("error = %v, want the wrapped listen error", err)
	}
	if !strings.Contains(err.Error(), "--addr") || strings.Contains(err.Error(), "another configuration") {
		t.Errorf("error = %v", err)
	}
}

func heldLoopbackAddrs(t *testing.T, n int) []string {
	t.Helper()
	var addrs []string
	for range n {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ln.Close() })
		addrs = append(addrs, ln.Addr().String())
	}
	return addrs
}

func TestView_ScanWaitsForASlowServerOfTheSameConfiguration(t *testing.T) {
	project := t.TempDir()
	mustWrite(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	ports := append(heldLoopbackAddrs(t, 1), freeLoopbackAddrs(t, 1)...)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opened := ""
	cmd := newViewCmdWithPorts(func(url string) error { opened = url; cancel(); return nil }, func(addr string, timeout time.Duration) *viewServer {
		if addr == ports[0] && timeout >= 500*time.Millisecond {
			return &viewServer{OK: true, Root: project, UI: true}
		}
		return nil
	}, ports)
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--dir", project})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if want := "http://" + ports[0] + "/"; opened != want {
		t.Errorf("opened %q, want reuse of the slow server at %s", opened, want)
	}
}

func TestView_ScanSharesOneRecheckBudgetAcrossSilentPorts(t *testing.T) {
	project := t.TempDir()
	mustWrite(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	held := heldLoopbackAddrs(t, 4)
	ports := append(slices.Clone(held), freeLoopbackAddrs(t, 1)...)
	probes := map[string]int{}
	var rechecks time.Duration
	opened, _, err := runViewOnPortsWithProbe(t, ports, func(addr string, timeout time.Duration) *viewServer {
		probes[addr]++
		if probes[addr] > 1 {
			rechecks += timeout
		}
		return nil
	}, "--dir", project)
	if err != nil {
		t.Fatal(err)
	}
	if want := "http://" + ports[4] + "/"; opened != want {
		t.Errorf("opened %q, want %s", opened, want)
	}
	if limit := viewBusyProbeTimeout + time.Duration(len(held)-1)*viewProbeTimeout; rechecks > limit {
		t.Errorf("rechecks waited %v in total, want at most %v", rechecks, limit)
	}
}
