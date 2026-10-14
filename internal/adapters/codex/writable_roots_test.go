package codex

import "testing"

func TestRemoveWritableRoots(t *testing.T) {
	drop := []string{"/home/me/store"}
	for _, tc := range []struct {
		name, in, want string
		ok             bool
	}{
		{
			name: "only the dropped root removes the table",
			in:   "model = \"x\"\n\n[sandbox_workspace_write]\nwritable_roots = [\"/home/me/store\"]\n\n[history]\npersistence = \"none\"\n",
			want: "model = \"x\"\n\n[history]\npersistence = \"none\"\n",
			ok:   true,
		},
		{
			name: "a user root stays",
			in:   "[sandbox_workspace_write]\n# scratch\nwritable_roots = [\"/opt/a\", \"/home/me/store\"]\nnetwork_access = true\n",
			want: "[sandbox_workspace_write]\n# scratch\nwritable_roots = [\"/opt/a\"]\nnetwork_access = true\n",
			ok:   true,
		},
		{
			name: "a multi-line array",
			in:   "[sandbox_workspace_write]\nwritable_roots = [\n  \"/home/me/store\", # sync\n  \"/opt/a\",\n]\n",
			want: "[sandbox_workspace_write]\nwritable_roots = [\"/opt/a\"]\n",
			ok:   true,
		},
		{
			name: "the table keeps its other keys",
			in:   "[sandbox_workspace_write]\nwritable_roots = [\"/home/me/store\"]\nnetwork_access = true\n",
			want: "[sandbox_workspace_write]\nnetwork_access = true\n",
			ok:   true,
		},
		{
			name: "no roots to drop",
			in:   "model = \"x\"\n",
			want: "model = \"x\"\n",
			ok:   true,
		},
		{
			name: "a root the edit cannot read",
			in:   "sandbox_workspace_write.writable_roots = [\"/home/me/store\"]\n",
			want: "sandbox_workspace_write.writable_roots = [\"/home/me/store\"]\n",
			ok:   false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := RemoveWritableRoots(tc.in, drop)
			if ok != tc.ok || got != tc.want {
				t.Errorf("RemoveWritableRoots = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestAddWritableRoots(t *testing.T) {
	add := []string{"/home/me/store", "/opt/a"}
	for _, tc := range []struct {
		name, in, want string
		ok             bool
	}{
		{
			name: "appends to the roots",
			in:   "[sandbox_workspace_write]\nwritable_roots = [\"/opt/a\"]\n",
			want: "[sandbox_workspace_write]\nwritable_roots = [\"/opt/a\", \"/home/me/store\"]\n",
			ok:   true,
		},
		{
			name: "adds the key to a table without it",
			in:   "[sandbox_workspace_write]\nnetwork_access = true\n",
			want: "[sandbox_workspace_write]\nwritable_roots = [\"/home/me/store\", \"/opt/a\"]\nnetwork_access = true\n",
			ok:   true,
		},
		{
			name: "no table",
			in:   "model = \"x\"\n",
			want: "model = \"x\"\n",
			ok:   false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := addWritableRoots(tc.in, add)
			if ok != tc.ok || got != tc.want {
				t.Errorf("addWritableRoots = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}
