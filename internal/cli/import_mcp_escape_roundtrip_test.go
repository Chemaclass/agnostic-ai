package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// An escaped `$${NAME}` stays escaped and a `${NAME}` stays a reference
// when import reads back what sync wrote, even when the run cannot keep
// the spec as it was and rewrites it from the tool's file.
func TestImport_SyncedMCPEscapesRoundTripOnEveryTarget(t *testing.T) {
	var targets []string
	for target := range targetsSupportingKind[spec.KindMCP] {
		targets = append(targets, target)
	}
	slices.Sort(targets)
	const source = "name: app\ncommand: srv\nargs:\n  - --token=$${ARG_TOKEN}\nenv:\n  LIT: $${LIT}\n  REF: ${REF}\n"
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			captureLogOut(t)
			writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+target+"]\n")
			path := filepath.Join(dir, ".agnostic-ai", "mcps", "app.yaml")
			writeFile(t, path, source)
			mustSync(t)
			// Without the ledger, import cannot tell the tool's file is
			// the one sync wrote, so it rewrites the spec from it.
			if err := os.Remove(stateFilePath(dir)); err != nil {
				t.Fatal(err)
			}
			if out, err := runCLI(t, "import", target, "--overwrite"); err != nil {
				t.Fatalf("import: %v\n%s", err, out)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var want, got map[string]any
			if err := yaml.Unmarshal([]byte(source), &want); err != nil {
				t.Fatal(err)
			}
			if err := yaml.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			// A tool with no reference form in a field never receives
			// the reference, so only the values it shows are compared.
			if got["args"] != nil && !reflect.DeepEqual(got["args"], want["args"]) {
				t.Errorf("args after sync then import = %#v, want %#v\n%s", got["args"], want["args"], data)
			}
			gotEnv, _ := got["env"].(map[string]any)
			wantEnv, _ := want["env"].(map[string]any)
			for key, value := range gotEnv {
				if value != wantEnv[key] {
					t.Errorf("env.%s after sync then import = %#v, want %#v\n%s", key, value, wantEnv[key], data)
				}
			}
		})
	}
}

func TestKeepSyncedMCPEscapes(t *testing.T) {
	const before = "name: app\ncommand: srv\nargs:\n  - --token=$${ARG}\nenv:\n  LIT: $${LIT}\n"
	cases := []struct {
		name, now, want string
		sources         []string
	}{
		{
			name:    "a tool that expands the reference gets the escape back",
			now:     "args:\n    - --token=${ARG}\ncommand: srv\nenv:\n    LIT: ${LIT}\nname: app\n",
			want:    "args:\n  - --token=$${ARG}\ncommand: srv\nenv:\n  LIT: $${LIT}\nname: app\n",
			sources: []string{"claude"},
		},
		{
			name:    "an argument added before the escaped one",
			now:     "args:\n    - --verbose\n    - --token=${ARG}\ncommand: srv\nenv:\n    LIT: ${LIT}\nname: app\n",
			want:    "args:\n  - --verbose\n  - --token=$${ARG}\ncommand: srv\nenv:\n  LIT: $${LIT}\nname: app\n",
			sources: []string{"claude"},
		},
		{
			name:    "a tool whose own form tells a reference from text",
			now:     "args:\n    - --token=${ARG}\ncommand: srv\nenv:\n    LIT: ${LIT}\nname: app\n",
			sources: []string{"opencode"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, changed := keepSyncedMCPEscapes([]byte(before), []byte(c.now), c.sources)
			want := c.want
			if want == "" {
				want = c.now
			}
			if changed != (c.want != "") || string(got) != want {
				t.Errorf("keepSyncedMCPEscapes = %v\n%s\nwant\n%s", changed, got, want)
			}
		})
	}
}
