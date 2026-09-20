package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// goreleaserConfigPath is the release config these tests guard.
var goreleaserConfigPath = filepath.Join("..", "..", ".goreleaser.yml")

// caskConfig returns the first homebrew_casks entry from .goreleaser.yml.
func caskConfig(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(goreleaserConfigPath)
	if err != nil {
		t.Fatalf("read %s: %v", goreleaserConfigPath, err)
	}
	var doc struct {
		HomebrewCasks []map[string]any `yaml:"homebrew_casks"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", goreleaserConfigPath, err)
	}
	if len(doc.HomebrewCasks) == 0 {
		t.Fatal("no homebrew_casks entry in .goreleaser.yml")
	}
	return doc.HomebrewCasks[0]
}

// TestGoreleaserCask_AvoidsTheDeprecatedPostflightStanza keeps the cask
// off Homebrew's deprecated `postflight` block.
//
// goreleaser's `hooks` key renders raw Ruby as `postflight do`, which
// Homebrew deprecated in favour of the declarative `postflight_steps`
// DSL. Every `brew` command touching the tap then prints a warning
// naming our tap and asking the user to report it to us (#933).
//
// Nothing else catches a regression here. The cask is only generated at
// tag time, so switching back to `hooks` would look fine in CI and
// surface as a warning on every user's machine after the release.
func TestGoreleaserCask_AvoidsTheDeprecatedPostflightStanza(t *testing.T) {
	cask := caskConfig(t)
	if _, ok := cask["hooks"]; ok {
		t.Error("homebrew_casks has a `hooks` key; goreleaser renders it as Homebrew's deprecated `postflight do`, so use `custom_block` with `postflight_steps` instead")
	}
	block, _ := cask["custom_block"].(string)
	if !strings.Contains(block, "postflight_steps do") {
		t.Errorf("custom_block does not declare postflight_steps:\n%s", block)
	}
}

// TestGoreleaserCask_StripsQuarantine pins the reason the block exists
// at all.
//
// Homebrew propagates com.apple.quarantine from the downloaded archive
// to the extracted files, and the `binary` artifact only chmods +x, so
// it never strips it. Without this call macOS Gatekeeper blocks the
// unsigned binary on first run. Deleting the block is the obvious way
// to silence the deprecation warning and it breaks every macOS install.
func TestGoreleaserCask_StripsQuarantine(t *testing.T) {
	block, _ := caskConfig(t)["custom_block"].(string)
	for _, want := range []string{"xattr", "com.apple.quarantine"} {
		if !strings.Contains(block, want) {
			t.Errorf("custom_block no longer strips quarantine, missing %q:\n%s", want, block)
		}
	}
}

// TestGoreleaserCask_EscapesHomebrewsStagedPathToken guards a failure
// that can only happen at tag time.
//
// goreleaser renders the cask and then runs the whole file through its
// own Go template engine a second time. Homebrew's `{{staged_path}}` is
// not a goreleaser variable, so a bare token aborts the release with
// `function "staged_path" not defined`. Wrapping it in a goreleaser
// template that yields the literal string survives that pass.
//
// Verified by reverting the wrapper locally: `goreleaser release
// --snapshot` fails with exactly that error, and no CI job would have
// caught it, because CI never renders the cask.
func TestGoreleaserCask_EscapesHomebrewsStagedPathToken(t *testing.T) {
	block, _ := caskConfig(t)["custom_block"].(string)
	const wrapped = `{{ "{{staged_path}}" }}`
	if !strings.Contains(block, wrapped) {
		t.Fatalf("custom_block must reference Homebrew's staged path as %s:\n%s", wrapped, block)
	}
	// A bare token anywhere outside the wrapper breaks the release the
	// same way, so check none survives once the wrapped form is removed.
	if rest := strings.ReplaceAll(block, wrapped, ""); strings.Contains(rest, "{{staged_path}}") {
		t.Errorf("custom_block has a bare {{staged_path}}; goreleaser templates the rendered cask, so it fails the release at tag time:\n%s", block)
	}
}
