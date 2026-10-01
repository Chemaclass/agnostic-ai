package emit

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestWriteMergedYAML_RecordsValueSums(t *testing.T) {
	testutil.TempCwd(t)
	const path = "conf.yml"
	sess := NewSession()
	sess.StartDetailedRecording()
	err := sess.WriteMergedYAML(path, HeaderBlock(FormatYAML)+"model: x\nread:\n    - a\n", []MergedKey{{Path: []string{"model"}}, {Path: []string{"read"}, Items: []string{"a"}}}, nil, false)
	writes := sess.StopDetailedRecording()
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) != 1 || !writes[0].Merged || len(writes[0].Keys) != 2 {
		t.Fatalf("want one merged write with two keys, got %#v", writes)
	}
	var model yaml.Node
	if err := yaml.Unmarshal([]byte("x"), &model); err != nil {
		t.Fatal(err)
	}
	if got := writes[0].Keys[0].Sum; got != yamlValueSum(model.Content[0]) {
		t.Errorf("model sum = %q", got)
	}
	if got := writes[0].Keys[1].Items; len(got) != 1 || got[0] != ContentSum("a") {
		t.Errorf("read items = %v", got)
	}
}

func TestReleaseMergedYAML(t *testing.T) {
	sumOf := func(t *testing.T, v string) string {
		t.Helper()
		var doc yaml.Node
		if err := yaml.Unmarshal([]byte(v), &doc); err != nil {
			t.Fatal(err)
		}
		return yamlValueSum(doc.Content[0])
	}
	for _, tc := range []struct {
		name, seed, want string
		keys             func(t *testing.T) []MergedKey
		created          bool
		result           MergedRelease
	}{
		{
			name: "keeps the user's keys, comments, and flow list",
			seed: HeaderBlock(FormatYAML) + "# mine\nzebra: 1 # z\nread: [NOTES.md, CONVENTIONS.md]\nmodel: x\nalpha: 2\n",
			keys: func(t *testing.T) []MergedKey {
				return []MergedKey{{Path: []string{"model"}, Sum: sumOf(t, "x")}, {Path: []string{"read"}, Items: []string{ContentSum("CONVENTIONS.md")}}}
			},
			want:   "# mine\nzebra: 1 # z\nread: [NOTES.md]\nalpha: 2\n",
			result: MergedStripped,
		},
		{
			name: "drops a scalar entry and an emptied mapping",
			seed: "read: CONVENTIONS.md\nnested:\n  model: x\nkeep: true\n",
			keys: func(t *testing.T) []MergedKey {
				return []MergedKey{{Path: []string{"nested", "model"}, Sum: sumOf(t, "x")}, {Path: []string{"read"}, Items: []string{ContentSum("CONVENTIONS.md")}}}
			},
			want:   "keep: true\n",
			result: MergedStripped,
		},
		{
			name: "keeps an edited value",
			seed: "model: mine\n",
			keys: func(t *testing.T) []MergedKey {
				return []MergedKey{{Path: []string{"model"}, Sum: sumOf(t, "x")}}
			},
			want:   "model: mine\n",
			result: MergedEdited,
		},
		{
			name: "removes a file sync created",
			seed: HeaderBlock(FormatYAML) + "model: x\n",
			keys: func(t *testing.T) []MergedKey {
				return []MergedKey{{Path: []string{"model"}, Sum: sumOf(t, "x")}}
			},
			created: true,
			result:  MergedRemoved,
		},
		{
			name: "keeps a value the user anchored",
			seed: "model: &m x\nweak-model: *m\nread: [&r CONVENTIONS.md, *r]\n",
			keys: func(t *testing.T) []MergedKey {
				return []MergedKey{{Path: []string{"model"}, Sum: sumOf(t, "x")}, {Path: []string{"read"}, Items: []string{ContentSum("CONVENTIONS.md")}}}
			},
			want:   "model: &m x\nweak-model: *m\nread: [&r CONVENTIONS.md, *r]\n",
			result: MergedEdited,
		},
		{
			name: "keeps a list the user anchored",
			seed: "read: &docs [CONVENTIONS.md]\nfile: *docs\n",
			keys: func(*testing.T) []MergedKey {
				return []MergedKey{{Path: []string{"read"}, Items: []string{ContentSum("CONVENTIONS.md")}}}
			},
			want:   "read: &docs [CONVENTIONS.md]\nfile: *docs\n",
			result: MergedEdited,
		},
		{
			name:   "keeps a file that does not parse",
			seed:   "model: [x\n",
			keys:   func(*testing.T) []MergedKey { return []MergedKey{{Path: []string{"model"}}} },
			want:   "model: [x\n",
			result: MergedKept,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.TempCwd(t)
			const path = ".aider.conf.yml"
			if err := os.WriteFile(path, []byte(tc.seed), 0o644); err != nil {
				t.Fatal(err)
			}
			result, _, err := NewSession().ReleaseMergedYAML(path, tc.keys(t), tc.created, false, false)
			if err != nil {
				t.Fatal(err)
			}
			if result != tc.result {
				t.Errorf("result = %v, want %v", result, tc.result)
			}
			data, err := os.ReadFile(path)
			if tc.result == MergedRemoved {
				if err == nil {
					t.Errorf("%s stayed:\n%s", path, data)
				}
				return
			}
			if string(data) != tc.want {
				t.Errorf("%s =\n%s\nwant\n%s", path, data, tc.want)
			}
		})
	}
}
