package codex

import "testing"

func TestRemoveWritableRoots(t *testing.T) {
	drop := func(root string) bool { return root == "/home/me/store" }
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
			want: "[sandbox_workspace_write]\nwritable_roots = [\n  \"/opt/a\",\n]\n",
			ok:   true,
		},
		{
			name: "a multi-line array keeps the lines of the roots that stay",
			in:   "[sandbox_workspace_write]\nwritable_roots = [\n  \"/opt/a\", # mine\n  # sync\n  \"/home/me/store\",\n  \"/opt/b\",\n]\n",
			want: "[sandbox_workspace_write]\nwritable_roots = [\n  \"/opt/a\", # mine\n  # sync\n  \"/opt/b\",\n]\n",
			ok:   true,
		},
		{
			name: "entries sharing a line are rewritten on one",
			in:   "[sandbox_workspace_write]\nwritable_roots = [\n  \"/opt/a\", \"/home/me/store\",\n]\n",
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
			name: "a header with a trailing comment",
			in:   "[sandbox_workspace_write] # sandbox\nwritable_roots = [\"/opt/a\", \"/home/me/store\"]\n",
			want: "[sandbox_workspace_write] # sandbox\nwritable_roots = [\"/opt/a\"]\n",
			ok:   true,
		},
		{
			name: "a quoted header",
			in:   "[\"sandbox_workspace_write\"]\nwritable_roots = [\"/opt/a\", \"/home/me/store\"]\n",
			want: "[\"sandbox_workspace_write\"]\nwritable_roots = [\"/opt/a\"]\n",
			ok:   true,
		},
		{
			name: "CRLF line breaks",
			in:   "model = \"x\"\r\n[sandbox_workspace_write]\r\nwritable_roots = [\"/opt/a\", \"/home/me/store\"]\r\nnetwork_access = true\r\n",
			want: "model = \"x\"\r\n[sandbox_workspace_write]\r\nwritable_roots = [\"/opt/a\"]\r\nnetwork_access = true\r\n",
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
			got, _, ok := RemoveWritableRoots(tc.in, drop)
			if ok != tc.ok || got != tc.want {
				t.Errorf("RemoveWritableRoots = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestRemoveWritableRoots_ReportsTheComments(t *testing.T) {
	drop := func(root string) bool { return root == "/home/me/store" }
	for _, tc := range []struct {
		name, in string
		want     bool
	}{
		{"one entry per line", "[sandbox_workspace_write]\nwritable_roots = [\n  \"/opt/a\", # mine\n  \"/home/me/store\",\n]\n", false},
		{"one line", "[sandbox_workspace_write]\nwritable_roots = [\"/opt/a\", \"/home/me/store\"]\n", false},
		{"entries sharing a line with a comment", "[sandbox_workspace_write]\nwritable_roots = [\n  \"/opt/a\", \"/home/me/store\", # both\n]\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, got, ok := RemoveWritableRoots(tc.in, drop); !ok || got != tc.want {
				t.Errorf("lostComments = %v (ok %v), want %v", got, ok, tc.want)
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
			name: "a multi-line array keeps its layout and comments",
			in:   "[sandbox_workspace_write]\nwritable_roots = [\n  \"/opt/a\", # shared\n]\n",
			want: "[sandbox_workspace_write]\nwritable_roots = [\n  \"/opt/a\", # shared\n  \"/home/me/store\",\n]\n",
			ok:   true,
		},
		{
			name: "a multi-line array without a final comma",
			in:   "[sandbox_workspace_write]\nwritable_roots = [\n  \"/opt/a\"\n]\n",
			want: "[sandbox_workspace_write]\nwritable_roots = [\"/opt/a\", \"/home/me/store\"]\n",
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
