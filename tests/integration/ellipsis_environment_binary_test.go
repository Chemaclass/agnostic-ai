package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildEllipsisBinary builds the CLI the way a release does, so the test
// runs the program a user runs and not the package under test.
func buildEllipsisBinary(t *testing.T) string {
	t.Helper()
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "agnostic-ai")
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = filepath.Join(packageDir, "..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return binary
}

// ellipsisCommand runs the built binary in dir with no inherited target
// selection and an isolated home.
func ellipsisCommand(t *testing.T, binary, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	env := []string{}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "AGNOSTIC_AI_TARGET=") {
			env = append(env, kv)
		}
	}
	cmd.Env = append(env, "HOME="+t.TempDir(), "AGNOSTIC_AI_HOME="+t.TempDir())
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func writeProjectFile(t *testing.T, dir, rel, text string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readProjectFile(t *testing.T, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestEllipsisEnvironment_BuiltBinaryWritesSetupMCPAndTheReviewLink(t *testing.T) {
	const secret = "ELLIPSIS_BINARY_SECRET_2188"
	t.Setenv("TOOLS_TOKEN", secret)
	binary := buildEllipsisBinary(t)
	dir := t.TempDir()
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [ellipsis]\ngitignore:\n  enabled: false\n")
	writeProjectFile(t, dir, ".agnostic-ai/AGNOSTIC_AI.md", "Project guidance.\n")
	writeProjectFile(t, dir, ".agnostic-ai/reviews/general.md", "---\nname: general\n---\n\nFlag handlers that query the database directly.\n")
	writeProjectFile(t, dir, ".agnostic-ai/settings/ellipsis.yaml", `x-ellipsis:
  reviewer:
    name: project-review
    claude_code:
      model: claude-opus-5-5
  environment:
    name: api-environment
    repository:
      name: api-repo
`)
	writeProjectFile(t, dir, ".agnostic-ai/environments/dev.yaml", "name: dev\ninstall: npm ci\nsetup: npm run build --if-present\n")
	writeProjectFile(t, dir, ".agnostic-ai/mcps/internal-tools.yaml", "name: internal-tools\ntype: http\nurl: https://tools.example.com/mcp\nheaders:\n  Authorization: Bearer ${TOOLS_TOKEN}\n")
	writeProjectFile(t, dir, ".agnostic-ai/mcps/repo-tools.yaml", "name: repo-tools\ncommand: node\nargs: [/sandbox/api-repo/scripts/mcp-server.js]\nenv:\n  API_TOKEN: ${REPO_TOOLS_TOKEN}\n")

	preview, err := ellipsisCommand(t, binary, dir, "sync", "--dry-run")
	if err != nil {
		t.Fatalf("sync --dry-run: %v\n%s", err, preview)
	}
	for _, want := range []string{".ellipsis/environments/api-environment.yaml", "cd /sandbox/api-repo", "Bearer ${TOOLS_TOKEN}"} {
		if !strings.Contains(preview, want) {
			t.Errorf("preview lacks %q:\n%s", want, preview)
		}
	}
	if strings.Contains(preview, secret) {
		t.Error("the preview printed a local secret")
	}
	if _, err := os.Stat(filepath.Join(dir, ".ellipsis")); !os.IsNotExist(err) {
		t.Errorf("dry run wrote files (%v)", err)
	}

	if out, err := ellipsisCommand(t, binary, dir, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	environment := readProjectFile(t, dir, ".ellipsis/environments/api-environment.yaml")
	for _, want := range []string{
		"build_base:\n    run: |\n      cd /sandbox/api-repo\n      npm ci\n",
		"after_checkout: |\n    cd /sandbox/api-repo\n    npm run build --if-present\n",
		"Authorization: Bearer ${TOOLS_TOKEN}",
		"API_TOKEN: ${REPO_TOOLS_TOKEN}",
		"- /sandbox/api-repo/scripts/mcp-server.js",
	} {
		if !strings.Contains(environment, want) {
			t.Errorf("environment file lacks %q:\n%s", want, environment)
		}
	}
	if strings.Contains(environment, secret) {
		t.Error("the environment file holds a local secret")
	}
	if review := readProjectFile(t, dir, ".ellipsis/code-review.yaml"); !strings.Contains(review, "environment: api-environment") {
		t.Errorf("the review does not reference the environment:\n%s", review)
	}

	if out, err := ellipsisCommand(t, binary, dir, "sync", "--check"); err != nil {
		t.Errorf("sync --check after sync: %v\n%s", err, out)
	}
	writeProjectFile(t, dir, ".agnostic-ai/environments/dev.yaml", "name: dev\ninstall: npm install\nsetup: npm run build --if-present\n")
	if out, err := ellipsisCommand(t, binary, dir, "sync", "--check"); err == nil {
		t.Errorf("sync --check passed after the install changed:\n%s", out)
	}
	if out, err := ellipsisCommand(t, binary, dir, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if out, err := ellipsisCommand(t, binary, dir, "sync", "--check"); err != nil {
		t.Errorf("sync --check after re-sync: %v\n%s", err, out)
	}
	if !strings.Contains(readProjectFile(t, dir, ".ellipsis/environments/api-environment.yaml"), "npm install") {
		t.Error("the edit did not reach the environment file")
	}
}

func TestEllipsisEnvironment_BuiltBinaryImportsANativeEnvironmentFile(t *testing.T) {
	binary := buildEllipsisBinary(t)
	dir := t.TempDir()
	native := `ellipsis:
  kind: environment
  name: api-environment
repositories:
  - name: api-repo
variables:
  - name: NPM_TOKEN
hooks:
  build_base:
    inputs: [api-repo/package.json]
    run: |
      cd /sandbox/api-repo
      npm ci
  before_start: |
    cd /sandbox/api-repo
    test -d node_modules
compute:
  memory: 8GB
mcp_servers:
  - name: internal-tools
    url: https://tools.example.com/mcp
    headers:
      Authorization: Bearer ${TOOLS_TOKEN}
      X-Team: platform
`
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [ellipsis]\ngitignore:\n  enabled: false\n")
	writeProjectFile(t, dir, ".ellipsis/environments/api-environment.yaml", native)
	if out, err := ellipsisCommand(t, binary, dir, "import", "ellipsis"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if out, err := ellipsisCommand(t, binary, dir, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	got := readProjectFile(t, dir, ".ellipsis/environments/api-environment.yaml")
	for _, want := range []string{
		"inputs:\n      - api-repo/package.json\n",
		"cd /sandbox/api-repo\n      npm ci\n",
		"before_start: |\n    cd /sandbox/api-repo\n    test -d node_modules\n",
		"- name: NPM_TOKEN\n",
		"memory: 8GB",
		"Authorization: Bearer ${TOOLS_TOKEN}",
		"X-Team: platform",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the file lost %q:\n%s", want, got)
		}
	}
	if out, err := ellipsisCommand(t, binary, dir, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
	if out, err := ellipsisCommand(t, binary, dir, "lint"); err != nil {
		t.Errorf("lint: %v\n%s", err, out)
	}
	before := readProjectFile(t, dir, ".ellipsis/environments/api-environment.yaml")
	if out, err := ellipsisCommand(t, binary, dir, "import", "ellipsis"); err != nil {
		t.Fatalf("second import: %v\n%s", err, out)
	}
	if out, err := ellipsisCommand(t, binary, dir, "sync"); err != nil {
		t.Fatalf("second sync: %v\n%s", err, out)
	}
	if after := readProjectFile(t, dir, ".ellipsis/environments/api-environment.yaml"); after != before {
		t.Errorf("repeated import and sync changed the file:\n%s", after)
	}
}
