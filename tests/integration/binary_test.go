package integration

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

type builtCLI struct {
	once   sync.Once
	dir    string
	binary string
	err    error
}

var (
	builtCLIMu sync.Mutex
	builtCLIs  = map[string]*builtCLI{}
	// Set by TestMain before any test runs.
	builtCLIRoot, moduleRoot string
)

// builtBinary returns the CLI built the way a release builds it, so a test
// runs the program a user runs. The build happens once per test process;
// callers must not modify the file or write next to it.
func builtBinary(t *testing.T) string {
	t.Helper()
	return builtBinaryWithLDFlags(t, "")
}

// builtCandidateBinary returns a CLI that reports version as both its own
// version and the candidate version it accepts in a project pin.
func builtCandidateBinary(t *testing.T, version string) string {
	t.Helper()
	return builtBinaryWithLDFlags(t, "-X main.version="+version+" -X github.com/chemaclass/agnostic-ai/internal/cli.candidateVersion="+version)
}

func builtBinaryWithLDFlags(t *testing.T, ldflags string) string {
	t.Helper()
	builtCLIMu.Lock()
	cli, ok := builtCLIs[ldflags]
	if !ok {
		cli = &builtCLI{dir: filepath.Join(builtCLIRoot, fmt.Sprint(len(builtCLIs)))}
		builtCLIs[ldflags] = cli
	}
	builtCLIMu.Unlock()
	cli.once.Do(func() { cli.binary, cli.err = buildCLI(cli.dir, ldflags) })
	if cli.err != nil {
		t.Fatal(cli.err)
	}
	return cli.binary
}

func buildCLI(dir, ldflags string) (string, error) {
	if err := os.Mkdir(dir, 0o755); err != nil {
		return "", err
	}
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(dir, name)
	args := []string{"build"}
	if ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	build := exec.Command("go", append(args, "-o", binary, "./cmd/agnostic-ai")...)
	build.Dir = moduleRoot
	if out, err := build.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build CLI (ldflags %q): %v\n%s", ldflags, err, out)
	}
	return binary, nil
}

// copyBuiltBinary places a copy of the built CLI at path, for tests whose
// subject is where the binary lives.
func copyBuiltBinary(t *testing.T, binary, path string) {
	t.Helper()
	source, err := os.Open(binary)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	target, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(target, source); err != nil {
		target.Close()
		t.Fatal(err)
	}
	if err := target.Close(); err != nil {
		t.Fatal(err)
	}
}
