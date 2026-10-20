package spec

import (
	"path/filepath"
	"reflect"
	"testing"
)

func historyLayer(t *testing.T, name, body string, extends bool) Layer {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "rules", "shared.md"), "---\nname: shared\n---\n"+body)
	return Layer{Name: name, Root: root, Sources: defaultsForTest().Sources, Extends: extends}
}

func TestLoadLayeredWithHistory_TracksReplacementAndExtension(t *testing.T) {
	t.Parallel()
	layers := []Layer{
		historyLayer(t, "pack:team", "pack\n", false),
		historyLayer(t, "project", "project\n", false),
		historyLayer(t, "project-user", "::parent\npersonal\n", true),
	}
	bundle, history, err := LoadLayeredWithHistory(layers)
	if err != nil {
		t.Fatal(err)
	}
	wantOps := []string{"base", "replace", "extend"}
	wantRoles := []string{"replaced", "contributor", "contributor"}
	if len(history) != len(layers) {
		t.Fatalf("history has %d candidates, want %d", len(history), len(layers))
	}
	for i, candidate := range history {
		if candidate.Operation != wantOps[i] || candidate.Role != wantRoles[i] {
			t.Errorf("candidate %d = %s/%s, want %s/%s", i, candidate.Operation, candidate.Role, wantOps[i], wantRoles[i])
		}
		if candidate.Entry.Path != filepath.Join(layers[i].Root, "rules", "shared.md") || candidate.Entry.Layer != layers[i].Name {
			t.Errorf("candidate %d lost source identity: %+v", i, candidate.Entry)
		}
	}
	if history[2].Entry.Body != "::parent\npersonal\n" {
		t.Errorf("extension candidate body = %q, want original body", history[2].Entry.Body)
	}
	if bundle.Rules[0].Body != "project\npersonal\n" {
		t.Errorf("effective body = %q", bundle.Rules[0].Body)
	}
	plain, err := LoadLayered(layers)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(bundle, plain) {
		t.Error("recording history changed the effective bundle")
	}
}

func TestLoadLayeredWithHistory_ReplacementDiscardsTheWholeContributorChain(t *testing.T) {
	t.Parallel()
	layers := []Layer{
		historyLayer(t, "builtin", "builtin\n", false),
		historyLayer(t, "personal", "::parent\nextra\n", true),
		historyLayer(t, "project", "replacement\n", false),
	}
	_, history, err := LoadLayeredWithHistory(layers)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"replaced", "replaced", "contributor"} {
		if history[i].Role != want {
			t.Errorf("candidate %d role = %q, want %q", i, history[i].Role, want)
		}
	}
}

func TestLoadLayeredWithHistory_RecordsOnlyTheSameLayerWinner(t *testing.T) {
	t.Parallel()
	layer := historyLayer(t, "project", "loser\n", false)
	winner := filepath.Join(layer.Root, "rules", "z.md")
	mustWrite(t, winner, "---\nname: shared\n---\nwinner\n")
	bundle, history, err := LoadLayeredWithHistory([]Layer{layer})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Entry.Path != winner || history[0].Operation != "base" || history[0].Role != "contributor" {
		t.Errorf("history = %+v, want only the winner as a base contributor", history)
	}
	if len(bundle.Shadowed) != 1 || bundle.Shadowed[0].Body != "loser\n" {
		t.Errorf("shadowed = %+v, want the same-layer loser", bundle.Shadowed)
	}
}

func TestLoadLayeredWithHistory_ScopesNamesByKind(t *testing.T) {
	t.Parallel()
	base := historyLayer(t, "pack:team", "rule\n", false)
	mustWrite(t, filepath.Join(base.Root, "agents", "shared.md"), "---\nname: shared\n---\nagent\n")
	over := historyLayer(t, "project", "replacement\n", false)
	_, history, err := LoadLayeredWithHistory([]Layer{base, over})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 3 {
		t.Fatalf("history has %d candidates, want 3", len(history))
	}
	if history[0].Entry.Kind != KindAgent || history[0].Role != "contributor" || history[1].Entry.Kind != KindRule || history[1].Role != "replaced" {
		t.Errorf("history = %+v, want unchanged agent and replaced rule", history)
	}
}

func TestLoadLayeredWithHistory_NewNameInExtendingLayerIsABase(t *testing.T) {
	t.Parallel()
	layer := historyLayer(t, "project-user", "::parent\nnew\n", true)
	bundle, history, err := LoadLayeredWithHistory([]Layer{layer})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Operation != "base" || history[0].Entry.Body != "::parent\nnew\n" {
		t.Errorf("history = %+v, want the original entry as a base", history)
	}
	if bundle.Rules[0].Body != "new\n" {
		t.Errorf("effective body = %q, want stray parent marker removed", bundle.Rules[0].Body)
	}
}
