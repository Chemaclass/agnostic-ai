package cli

import (
	"encoding/binary"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
	"golang.org/x/sys/windows"
)

func TestNewImportSourceAlias_CreatesDirectoryJunction(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	target := t.TempDir()
	alias, err := newImportSourceAlias(t.TempDir(), target)
	if err != nil {
		t.Fatal(err)
	}
	if got := importSourceAliasReparseTag(t, alias); got != windows.IO_REPARSE_TAG_MOUNT_POINT {
		t.Errorf("alias reparse tag = %#x, want directory junction", got)
	}
}

func TestImport_AbsoluteSourceJunctionsSharePreviewWrites(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	external := t.TempDir()
	shared := filepath.Join(external, "shared")
	alias := filepath.Join(external, "alias")
	const original = "---\nname: foo\ndescription: Mine.\n---\nMine.\n"
	mustWriteFile(t, filepath.Join(shared, "foo.md"), original)
	if err := createImportSourceAlias(shared, alias); err != nil {
		t.Fatalf("create input junction: %v", err)
	}
	if got := importSourceAliasReparseTag(t, alias); got != windows.IO_REPARSE_TAG_MOUNT_POINT {
		t.Fatalf("input reparse tag = %#x, want directory junction", got)
	}
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nsources:\n  rules: "+filepath.ToSlash(shared)+"\n  agents: "+filepath.ToSlash(alias)+"\n")
	mustWriteFile(t, ".claude/rules/foo.md", "Rule replacement.\n")
	mustWriteFile(t, ".claude/agents/foo.md", "---\nname: foo\ndescription: Agent.\n---\nAgent replacement.\n")
	for _, flags := range [][]string{{"--dry-run"}, {"--dry-run", "--diff"}} {
		out, err := runAbsoluteImportCLI(t, append([]string{"import", "claude", "--overwrite"}, flags...)...)
		if err != nil {
			t.Fatalf("flags %v: preview: %v\n%s", flags, err, out)
		}
		if got := readFile(t, filepath.Join(shared, "foo.md")); got != original {
			t.Errorf("flags %v: preview changed original = %q", flags, got)
		}
	}
	preview, err := planImportPreview([]string{"claude"})
	if err != nil {
		t.Fatal(err)
	}
	if out, err := runAbsoluteImportCLI(t, "import", "claude", "--overwrite"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	paths := map[string]bool{
		filepath.ToSlash(filepath.Join(shared, "foo.md")): false,
		filepath.ToSlash(filepath.Join(alias, "foo.md")):  false,
	}
	for _, entry := range preview.entries {
		if _, ok := paths[entry.path]; !ok {
			continue
		}
		paths[entry.path] = true
		if got := readFile(t, filepath.FromSlash(entry.path)); got != string(entry.after) {
			t.Errorf("preview differs from real destination %s: preview=%q real=%q", entry.path, entry.after, got)
		}
	}
	for path, found := range paths {
		if !found {
			t.Errorf("preview lost lexical destination %s: %#v", path, preview.entries)
		}
	}
}

func importSourceAliasReparseTag(t *testing.T, alias string) uint32 {
	t.Helper()
	path, err := windows.UTF16PtrFromString(alias)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(path, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	data := make([]byte, windows.MAXIMUM_REPARSE_DATA_BUFFER_SIZE)
	var returned uint32
	if err := windows.DeviceIoControl(handle, windows.FSCTL_GET_REPARSE_POINT, nil, 0, &data[0], uint32(len(data)), &returned, nil); err != nil {
		t.Fatal(err)
	}
	if returned < 8 {
		t.Fatalf("short reparse data: %d bytes", returned)
	}
	return binary.LittleEndian.Uint32(data)
}
