package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type specMoveTestReply struct {
	OK           bool     `json:"ok"`
	Source       string   `json:"source"`
	Destination  string   `json:"destination"`
	Exists       bool     `json:"exists"`
	Files        []string `json:"files"`
	SyncCommands []string `json:"syncCommands"`
	Expected     string   `json:"expected"`
	Error        string   `json:"error"`
	Missing      bool     `json:"missing"`
	Root         string   `json:"root"`
}

// moveProject is a project server's working folder with a global setup
// beside it, both under a fake home.
func moveProject(t *testing.T) (project, global string) {
	t.Helper()
	global = editHome(t)
	project = scopeProject(t)
	mustWrite(t, ".agnostic-ai/skills/review-pr/SKILL.md", "---\nname: review-pr\ndescription: Review a pull request.\n---\n\nReview it.\n")
	mustWrite(t, ".agnostic-ai/skills/review-pr/checklist.md", "- tests\n")
	mustWrite(t, ".agnostic-ai/agents/guard.md", "---\nname: guard\ndescription: Guards the shell.\n---\n\nGuard.\n")
	mustWrite(t, ".agnostic-ai/rules/style.md", "---\nname: style\ndescription: Style.\n---\n\nUse tabs.\n")
	mustWrite(t, ".agnostic-ai/rules/api.md", "---\nname: api\ndescription: API.\npaths: [\"api/**\"]\n---\n\nVersion the API.\n")
	mustWrite(t, ".agnostic-ai/commands/ship.md", "---\nname: ship\ndescription: Ship.\n---\n\nShip it.\n")
	mustWrite(t, ".agnostic-ai/local/agents/mine.md", "---\nname: mine\ndescription: Mine.\n---\n\nMine.\n")
	return project, global
}

func projectMoveHandler(project, global string) http.Handler {
	return newSpecMoveHandler(project, false, &switchPeer{root: global, global: true})
}

func postSpecMove(t *testing.T, h http.Handler, request map[string]any) (int, specMoveTestReply) {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	rec := postEdit(t, h, editSpecMovePath, string(body), nil)
	var reply specMoveTestReply
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
		t.Fatalf("reply %d is not JSON: %s", rec.Code, rec.Body)
	}
	return rec.Code, reply
}

func moveBody(kind, name, from, to, mode string) map[string]any {
	return map[string]any{"action": "preview", "kind": kind, "name": name, "from": from, "to": to, "mode": mode}
}

func previewThenApply(t *testing.T, h http.Handler, request map[string]any) (specMoveTestReply, int, specMoveTestReply) {
	t.Helper()
	code, preview := postSpecMove(t, h, request)
	if code != http.StatusOK {
		t.Fatalf("preview = %d %+v", code, preview)
	}
	request["action"], request["expected"] = "apply", preview.Expected
	code, applied := postSpecMove(t, h, request)
	return preview, code, applied
}

func TestSpecMove_MovesASkillFolderToGlobal(t *testing.T) {
	project, global := moveProject(t)
	h := projectMoveHandler(project, global)
	preview, code, applied := previewThenApply(t, h, moveBody("skill", "review-pr", "project", "global", "move"))
	if preview.Source != ".agnostic-ai/skills/review-pr/SKILL.md" || preview.Destination != "~/source/skills/review-pr/SKILL.md" || preview.Exists {
		t.Errorf("preview = %+v", preview)
	}
	if !slices.Equal(preview.Files, []string{"SKILL.md", "checklist.md"}) {
		t.Errorf("files = %v", preview.Files)
	}
	if code != http.StatusOK || !applied.OK {
		t.Fatalf("apply = %d %+v", code, applied)
	}
	if !slices.Equal(applied.SyncCommands, []string{"agnostic-ai sync", "agnostic-ai sync --global"}) {
		t.Errorf("sync commands = %v", applied.SyncCommands)
	}
	if got := readOptional(t, filepath.Join(global, "skills", "review-pr", "checklist.md")); got != "- tests\n" {
		t.Errorf("resource = %q", got)
	}
	if _, err := os.Stat(filepath.Join(project, ".agnostic-ai", "skills", "review-pr")); !os.IsNotExist(err) {
		t.Errorf("move left the source folder: %v", err)
	}
	b, err := loadGlobalBundle(global, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(b.Skills, func(e spec.Entry) bool { return e.Name == "review-pr" && e.Layer == "global" }) {
		t.Errorf("global setup does not load the skill: %+v", b.Skills)
	}
}

func TestSpecMove_CopyKeepsTheSourceAndNamesOnlyTheDestinationSync(t *testing.T) {
	project, global := moveProject(t)
	_, code, applied := previewThenApply(t, projectMoveHandler(project, global), moveBody("agent", "guard", "project", "global", "copy"))
	if code != http.StatusOK || !slices.Equal(applied.SyncCommands, []string{"agnostic-ai sync --global"}) {
		t.Fatalf("apply = %d %+v", code, applied)
	}
	want := readOptional(t, filepath.Join(project, ".agnostic-ai", "agents", "guard.md"))
	if want == "" || readOptional(t, filepath.Join(global, "agents", "guard.md")) != want {
		t.Errorf("copy did not keep both files equal")
	}
}

func TestSpecMove_OverwriteNeedsTheFlagAndReplacesTheWholeFolder(t *testing.T) {
	project, global := moveProject(t)
	mustWriteGlobalTest(t, filepath.Join(global, "skills", "review-pr", "SKILL.md"), "---\nname: review-pr\ndescription: Old.\n---\n\nOld.\n")
	mustWriteGlobalTest(t, filepath.Join(global, "skills", "review-pr", "stale.md"), "old\n")
	h := projectMoveHandler(project, global)

	preview, code, reply := previewThenApply(t, h, moveBody("skill", "review-pr", "project", "global", "copy"))
	if !preview.Exists || code != http.StatusConflict || !strings.Contains(reply.Error, "overwrite") {
		t.Fatalf("preview exists=%v, apply = %d %+v", preview.Exists, code, reply)
	}
	if got := readOptional(t, filepath.Join(global, "skills", "review-pr", "stale.md")); got != "old\n" {
		t.Fatalf("a refused copy changed the destination")
	}

	request := moveBody("skill", "review-pr", "project", "global", "copy")
	request["overwrite"] = true
	_, code, reply = previewThenApply(t, h, request)
	if code != http.StatusOK {
		t.Fatalf("overwrite = %d %+v", code, reply)
	}
	if _, err := os.Stat(filepath.Join(global, "skills", "review-pr", "stale.md")); !os.IsNotExist(err) {
		t.Errorf("overwrite kept a file the source does not have: %v", err)
	}
	if !strings.Contains(readOptional(t, filepath.Join(global, "skills", "review-pr", "SKILL.md")), "Review a pull request.") {
		t.Errorf("overwrite did not replace SKILL.md")
	}
}

func TestSpecMove_Refusals(t *testing.T) {
	project, global := moveProject(t)
	h := projectMoveHandler(project, global)
	scope, err := loadCheckScope(false)
	if err != nil {
		t.Fatal(err)
	}
	var builtin spec.Entry
	for _, e := range scope.bundle.All() {
		if e.Layer == layerNameBuiltin && (e.Kind == spec.KindRule || e.Kind == spec.KindSkill || e.Kind == spec.KindHook) {
			builtin = e
			break
		}
	}
	if builtin.Name == "" {
		t.Fatal("the test project loads no built-in spec")
	}
	cases := []struct {
		name    string
		request map[string]any
		status  int
		says    string
	}{
		{"built-in", moveBody(string(builtin.Kind), builtin.Name, "project", "global", "copy"), http.StatusUnprocessableEntity, "built-in"},
		{"local layer", moveBody("agent", "mine", "project", "global", "copy"), http.StatusUnprocessableEntity, "local"},
		{"command", moveBody("command", "ship", "project", "global", "copy"), http.StatusBadRequest, "kind"},
		{"settings", moveBody("settings", "defaults", "project", "global", "copy"), http.StatusBadRequest, "kind"},
		{"scoped rule to global", moveBody("rule", "api", "project", "global", "copy"), http.StatusUnprocessableEntity, "global rules"},
		{"unknown spec", moveBody("agent", "nobody", "project", "global", "copy"), http.StatusNotFound, "nobody"},
		{"from another level", moveBody("agent", "guard", "global", "project", "copy"), http.StatusBadRequest, "from"},
		{"same level", moveBody("agent", "guard", "project", "project", "copy"), http.StatusBadRequest, "to"},
		{"bad mode", moveBody("agent", "guard", "project", "global", "link"), http.StatusBadRequest, "mode"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, reply := postSpecMove(t, h, c.request)
			if code != c.status || !strings.Contains(reply.Error, c.says) {
				t.Errorf("= %d %+v, want %d naming %q", code, reply, c.status, c.says)
			}
		})
	}
	if readOptional(t, filepath.Join(project, ".agnostic-ai", "agents", "guard.md")) == "" {
		t.Error("a refusal removed the source")
	}
}

func TestSpecMove_StalePreviewWritesNothing(t *testing.T) {
	project, global := moveProject(t)
	h := projectMoveHandler(project, global)
	request := moveBody("agent", "guard", "project", "global", "move")
	_, preview := postSpecMove(t, h, request)
	mustWrite(t, ".agnostic-ai/agents/guard.md", "---\nname: guard\ndescription: Changed.\n---\n\nGuard.\n")
	request["action"], request["expected"] = "apply", preview.Expected
	code, reply := postSpecMove(t, h, request)
	if code != http.StatusConflict {
		t.Fatalf("stale apply = %d %+v", code, reply)
	}
	if _, err := os.Stat(filepath.Join(global, "agents", "guard.md")); !os.IsNotExist(err) {
		t.Errorf("stale apply wrote the destination: %v", err)
	}
}

func TestSpecMove_MissingGlobalSetupOffersCreate(t *testing.T) {
	project, _ := moveProject(t)
	missing := filepath.Join(t.TempDir(), "nowhere")
	code, reply := postSpecMove(t, projectMoveHandler(project, missing), moveBody("agent", "guard", "project", "global", "copy"))
	if code != http.StatusConflict || !reply.Missing || reply.Root != missing {
		t.Errorf("= %d %+v", code, reply)
	}
}

func TestSpecMove_GlobalServerCopiesToItsProject(t *testing.T) {
	project, global := moveProject(t)
	mustWriteGlobalTest(t, filepath.Join(global, "hooks", "lint.yaml"), "name: lint\non: after-edit\ncommand: npm run lint\n")
	h := newSpecMoveHandler(global, true, &switchPeer{root: project})
	_, code, reply := previewThenApply(t, h, moveBody("hook", "lint", "global", "project", "copy"))
	if code != http.StatusOK {
		t.Fatalf("apply = %d %+v", code, reply)
	}
	if reply.Destination != ".agnostic-ai/hooks/lint.yaml" && !strings.HasSuffix(reply.Destination, "/.agnostic-ai/hooks/lint.yaml") {
		t.Errorf("destination = %q", reply.Destination)
	}
	if !slices.Equal(reply.SyncCommands, []string{"cd " + shellQuote(project) + " && agnostic-ai sync"}) {
		t.Errorf("sync commands = %v", reply.SyncCommands)
	}
	if readOptional(t, filepath.Join(project, ".agnostic-ai", "hooks", "lint.yaml")) == "" {
		t.Error("the hook did not reach the project")
	}

	code, reply = postSpecMove(t, newSpecMoveHandler(global, true, nil), moveBody("hook", "lint", "global", "project", "copy"))
	if code != http.StatusConflict || !strings.Contains(reply.Error, "project") {
		t.Errorf("no project = %d %+v", code, reply)
	}
}

func TestSpecMove_UsesTheEditGuards(t *testing.T) {
	project, global := moveProject(t)
	h := projectMoveHandler(project, global)
	rec := postEdit(t, h, editSpecMovePath, `{}`, func(r *http.Request) { r.Method = http.MethodGet })
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d", rec.Code)
	}
	rec = postEdit(t, h, editSpecMovePath, `{}`, func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") })
	if rec.Code != http.StatusForbidden {
		t.Errorf("cross origin = %d", rec.Code)
	}
}

func TestServeMux_RoutesSpecMoveToThePeer(t *testing.T) {
	project, global := moveProject(t)
	src, err := openAuthoritySource(".", false)
	if err != nil {
		t.Fatal(err)
	}
	switches := newSwitchHandler(&switchPeer{root: global, global: true}, nil, probeViewServer, nil)
	defer switches.close()
	mux := newServeMuxWithSwitch(src.load, authority.HandlerOptions{Root: src.root, ConfigFiles: src.configFiles}, true, switches)
	_, code, reply := previewThenApply(t, mux, moveBody("agent", "guard", "project", "global", "move"))
	if code != http.StatusOK {
		t.Fatalf("apply = %d %+v", code, reply)
	}
	if readOptional(t, filepath.Join(global, "agents", "guard.md")) == "" || readOptional(t, filepath.Join(project, ".agnostic-ai", "agents", "guard.md")) != "" {
		t.Error("the served move did not reach the global setup")
	}
}
