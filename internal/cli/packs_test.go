package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSplitSourceRef(t *testing.T) {
	cases := []struct {
		in, src, ref string
	}{
		{"github.com/foo/bar", "github.com/foo/bar", ""},
		{"github.com/foo/bar@v1.2.0", "github.com/foo/bar", "v1.2.0"},
		{"git@host:foo/bar@v1", "git@host:foo/bar", "v1"},
		{"./local/pack", "./local/pack", ""},
	}
	for _, c := range cases {
		gotSrc, gotRef := splitSourceRef(c.in)
		if gotSrc != c.src || gotRef != c.ref {
			t.Errorf("splitSourceRef(%q)=(%q,%q), want (%q,%q)", c.in, gotSrc, gotRef, c.src, c.ref)
		}
	}
}

func TestDerivePackName(t *testing.T) {
	cases := map[string]string{
		"github.com/chemaclass/go-rules":     "go-rules",
		"https://github.com/foo/bar.git":     "bar",
		"git@github.com:foo/baz":             "baz",
		"./local/pack":                       "pack",
		"file:///tmp/sample-pack/":           "sample-pack",
		"https://example.com/team/security/": "security",
	}
	for in, want := range cases {
		if got := derivePackName(in); got != want {
			t.Errorf("derivePackName(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestValidatePackName(t *testing.T) {
	good := []string{"foo", "go-rules", "team_security", "v1"}
	for _, n := range good {
		if err := validatePackName(n); err != nil {
			t.Errorf("validatePackName(%q) unexpected err: %v", n, err)
		}
	}
	bad := []string{"", ".", "..", "foo/bar", `foo\bar`}
	for _, n := range bad {
		if err := validatePackName(n); err == nil {
			t.Errorf("validatePackName(%q) accepted, expected error", n)
		}
	}
}

func TestPacksLifecycle_LocalSource(t *testing.T) {
	root := testutil.TempCwd(t)

	src := filepath.Join(t.TempDir(), "go-rules")
	mustWrite(t, filepath.Join(src, "rules", "x.md"), "---\nname: x\n---\nbody")
	mustWrite(t, filepath.Join(src, "agents", "y.md"), "---\nname: y\n---\nbody")

	var out bytes.Buffer
	if err := runPacksAdd(root, src, "", &out); err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.Contains(out.String(), "added go-rules") {
		t.Errorf("add output: %q", out.String())
	}

	dest := filepath.Join(root, packsDir, "go-rules", "rules", "x.md")
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("expected copied rule at %s: %v", dest, err)
	}

	lock, err := readPacksLock(root)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if len(lock.Packs) != 1 || lock.Packs[0].Name != "go-rules" {
		t.Fatalf("lock state: %+v", lock.Packs)
	}

	out.Reset()
	if err := runPacksList(root, &out); err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out.String(), "go-rules") {
		t.Errorf("list output: %q", out.String())
	}

	out.Reset()
	if err := runPacksRemove(root, "go-rules", &out); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, packsDir, "go-rules")); !os.IsNotExist(err) {
		t.Errorf("expected pack dir removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, packsLockfile)); !os.IsNotExist(err) {
		t.Errorf("expected empty lock removed, stat err=%v", err)
	}
}

func TestPacksRemove_NotInstalled(t *testing.T) {
	root := testutil.TempCwd(t)
	var out bytes.Buffer
	if err := runPacksRemove(root, "missing", &out); err == nil {
		t.Fatal("expected error removing missing pack")
	}
}

func TestPacksUpdate_LocalSource(t *testing.T) {
	root := testutil.TempCwd(t)

	src := filepath.Join(t.TempDir(), "go-rules")
	mustWrite(t, filepath.Join(src, "rules", "x.md"), "---\nname: x\n---\nv1")

	var out bytes.Buffer
	if err := runPacksAdd(root, src, "", &out); err != nil {
		t.Fatalf("add: %v", err)
	}

	mustWrite(t, filepath.Join(src, "rules", "x.md"), "---\nname: x\n---\nv2")

	out.Reset()
	if err := runPacksUpdate(root, "", &out); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(root, packsDir, "go-rules", "rules", "x.md"))
	if !strings.Contains(string(got), "v2") {
		t.Errorf("expected v2 after update, got %q", got)
	}
}

func TestResolveLayers_PacksLayerInsertedBetweenUserGlobalAndProject(t *testing.T) {
	t.Setenv(envUserGlobalRoot, "/nonexistent-agnostic-ai-test-path")
	t.Setenv("HOME", t.TempDir())

	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "p1")
	mustWrite(t, filepath.Join(src, "rules", "r.md"), "---\nname: r\n---\nb")

	var out bytes.Buffer
	if err := runPacksAdd(root, src, "", &out); err != nil {
		t.Fatalf("add: %v", err)
	}

	cfg := &config.Config{Sources: defaultLayerSources()}
	layers := resolveLayers(root, cfg)
	if len(layers) != 2 {
		t.Fatalf("expected 2 layers (pack + project), got %d (%+v)", len(layers), layers)
	}
	if !strings.HasPrefix(layers[0].Name, layerNamePackPrefix) {
		t.Errorf("layer[0]=%q, want pack:* (lower precedence than project)", layers[0].Name)
	}
	if layers[1].Name != layerNameProject {
		t.Errorf("layer[1]=%q, want %q", layers[1].Name, layerNameProject)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
