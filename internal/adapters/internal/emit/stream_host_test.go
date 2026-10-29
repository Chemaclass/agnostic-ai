package emit

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestClearStaleStreamHost_RemovesTheEmptyHostOfARecordedColonOutput(t *testing.T) {
	testutil.TempCwd(t)
	host := filepath.Join(".gemini", "commands", "vc")
	if err := os.MkdirAll(filepath.Dir(host), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(host, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	sess := NewSession()
	sess.BackUpEditsSince(map[string]string{".gemini/commands/vc:save.toml": "sum"})
	if err := sess.ClearStaleStreamHost(host, false); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, err := os.Lstat(host); !os.IsNotExist(err) {
		t.Errorf("host file still present: %v", err)
	}
}

func TestClearStaleStreamHost_NamesAFileItCannotProveIsAHost(t *testing.T) {
	cases := map[string]struct {
		body   string
		ledger map[string]string
	}{
		"no colon output recorded": {body: "", ledger: map[string]string{".gemini/commands/other.toml": "sum"}},
		"file has bytes":           {body: "mine\n", ledger: map[string]string{".gemini/commands/vc:save.toml": "sum"}},
		"no ledger":                {body: ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			testutil.TempCwd(t)
			host := filepath.Join(".gemini", "commands", "vc")
			if err := os.MkdirAll(filepath.Dir(host), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(host, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			sess := NewSession()
			if tc.ledger != nil {
				sess.BackUpEditsSince(tc.ledger)
			}
			err := sess.ClearStaleStreamHost(host, false)
			if err == nil || !strings.Contains(err.Error(), "delete it and sync again") || !strings.Contains(err.Error(), host) {
				t.Errorf("clear = %v, want an error naming %s", err, host)
			}
			if _, statErr := os.Lstat(host); statErr != nil {
				t.Errorf("file removed: %v", statErr)
			}
		})
	}
}

func TestClearStaleStreamHost_KeepsTheHostOfAnEditedColonOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a colon file name is a stream of the host on Windows")
	}
	testutil.TempCwd(t)
	host := filepath.Join(".gemini", "commands", "vc")
	colon := ".gemini/commands/vc:save.toml"
	if err := os.MkdirAll(filepath.Dir(host), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(host, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(colon, []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess := NewSession()
	sess.BackUpEditsSince(map[string]string{colon: ContentSum("written\n")})
	err := sess.ClearStaleStreamHost(host, false)
	if err == nil || !strings.Contains(err.Error(), colon) || !strings.Contains(err.Error(), "edited") {
		t.Errorf("clear = %v, want an error naming the edited %s", err, colon)
	}
	if _, statErr := os.Lstat(host); statErr != nil {
		t.Errorf("host removed: %v", statErr)
	}
}

func TestClearStaleStreamHost_FollowsALinkToAFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating a link needs extra rights on Windows")
	}
	testutil.TempCwd(t)
	if err := os.MkdirAll("shared", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(".gemini", "commands"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(".gemini", "commands", "vc")
	if err := os.Symlink(filepath.Join("..", "..", "shared"), link); err != nil {
		t.Fatal(err)
	}
	if err := NewSession().ClearStaleStreamHost(link, false); err != nil {
		t.Errorf("clear: %v", err)
	}
}

func TestClearStaleStreamHost_CaptureLeavesTheDiskAlone(t *testing.T) {
	testutil.TempCwd(t)
	host := filepath.Join(".gemini", "commands", "vc")
	if err := os.MkdirAll(filepath.Dir(host), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(host, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess := NewSession()
	sess.StartCapture()
	if err := sess.ClearStaleStreamHost(host, false); err != nil {
		t.Errorf("clear under capture: %v", err)
	}
	if _, err := os.Lstat(host); err != nil {
		t.Errorf("capture removed the file: %v", err)
	}
}

func TestClearStaleStreamHost_LeavesAFolderAlone(t *testing.T) {
	testutil.TempCwd(t)
	dir := filepath.Join(".gemini", "commands", "vc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := NewSession().ClearStaleStreamHost(dir, false); err != nil {
		t.Errorf("clear: %v", err)
	}
	if err := NewSession().ClearStaleStreamHost(filepath.Join(dir, "missing"), false); err != nil {
		t.Errorf("clear missing: %v", err)
	}
}
