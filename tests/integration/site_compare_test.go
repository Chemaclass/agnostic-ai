package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/cli"
)

const (
	compareData      = "../../docs/site/data/compare.toml"
	capabilitiesData = "../../docs/site/data/capabilities.toml"
	compareFixture   = "fixtures/compare"
)

// compareKindDirs maps each capability feature to the fixture folder that holds
// its one sample spec.
var compareKindDirs = map[string]string{
	"agent":       "agents",
	"skill":       "skills",
	"rule":        "rules",
	"hook":        "hooks",
	"mcp":         "mcps",
	"command":     "commands",
	"settings":    "settings",
	"review":      "reviews",
	"environment": "environments",
	"ignore":      "ignore",
}

type capabilityData struct {
	Features []struct {
		ID string `toml:"id"`
	} `toml:"features"`
	Targets []struct {
		ID       string   `toml:"id"`
		Statuses []string `toml:"statuses"`
	} `toml:"targets"`
}

type compareFile struct {
	Targets []compareTarget `toml:"targets"`
}

type compareTarget struct {
	ID    string        `toml:"id"`
	Kinds []compareKind `toml:"kinds"`
}

type compareKind struct {
	ID    string   `toml:"id"`
	Paths []string `toml:"paths"`
}

// TestSiteDocs_CompareMatchesSync keeps the compare page honest. For every
// target and every spec kind in capabilities.toml it syncs the instructions
// file alone, then the instructions file plus that kind's one sample spec from
// fixtures/compare, and records the outputs the spec added or changed. A cell
// the matrix calls Native or Mapped must write something; any other state must
// write nothing, so the matrix and the adapters cannot drift apart silently.
//
// Regenerate with: UPDATE_GOLDEN=1 go test ./tests/integration/ -run TestSiteDocs_Compare
func TestSiteDocs_CompareMatchesSync(t *testing.T) {
	var capabilities capabilityData
	if _, err := toml.DecodeFile(capabilitiesData, &capabilities); err != nil {
		t.Fatalf("decode capability data: %v", err)
	}
	if len(capabilities.Targets) == 0 || len(capabilities.Features) == 0 {
		t.Fatal("capability data lists no targets or no features")
	}
	for _, feature := range capabilities.Features {
		dir, ok := compareKindDirs[feature.ID]
		if !ok {
			t.Fatalf("capability feature %q has no sample folder in compareKindDirs", feature.ID)
		}
		if _, err := os.Stat(filepath.Join(compareFixture, ".agnostic-ai", dir)); err != nil {
			t.Fatalf("capability feature %q has no sample spec in %s/.agnostic-ai/%s", feature.ID, compareFixture, dir)
		}
	}

	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	fixture := filepath.Join(packageDir, compareFixture, ".agnostic-ai")
	t.Setenv("HOME", t.TempDir())

	var data compareFile
	for _, target := range capabilities.Targets {
		if len(target.Statuses) != len(capabilities.Features) {
			t.Fatalf("target %q has %d statuses for %d features", target.ID, len(target.Statuses), len(capabilities.Features))
		}
		baseline := syncCompareSample(t, fixture, target.ID, "")
		entry := compareTarget{ID: target.ID}
		for index, feature := range capabilities.Features {
			outputs := syncCompareSample(t, fixture, target.ID, compareKindDirs[feature.ID])
			paths := addedOutputs(baseline, outputs)
			status := target.Statuses[index]
			switch status {
			case "native", "mapped":
				if len(paths) == 0 {
					t.Errorf("capabilities.toml marks %s %s as %s, but a sync of the sample %s spec writes nothing new", target.ID, feature.ID, status, feature.ID)
				}
			default:
				if len(paths) > 0 {
					t.Errorf("capabilities.toml marks %s %s as %s, but a sync of the sample %s spec writes %s", target.ID, feature.ID, status, feature.ID, strings.Join(paths, ", "))
				}
			}
			entry.Kinds = append(entry.Kinds, compareKind{ID: feature.ID, Paths: paths})
		}
		data.Targets = append(data.Targets, entry)
	}
	if t.Failed() {
		return
	}

	var want bytes.Buffer
	want.WriteString("# Generated from tests/integration/fixtures/compare by a real sync. Do not edit.\n")
	want.WriteString("# Regenerate with: UPDATE_GOLDEN=1 go test ./tests/integration/ -run TestSiteDocs_Compare\n\n")
	if err := toml.NewEncoder(&want).Encode(data); err != nil {
		t.Fatalf("encode compare data: %v", err)
	}

	target := filepath.Join(packageDir, compareData)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(target, want.Bytes(), 0o644); err != nil {
			t.Fatalf("write compare data: %v", err)
		}
		t.Logf("compare data updated: %s (%d targets)", target, len(data.Targets))
		return
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read compare data: %v (run UPDATE_GOLDEN=1 to create it)", err)
	}
	if !bytes.Equal(got, want.Bytes()) {
		t.Errorf("%s no longer matches a sync of fixtures/compare; run UPDATE_GOLDEN=1 go test ./tests/integration/ -run TestSiteDocs_Compare and review the diff", compareData)
	}
}

// TestSiteDocs_CompareListsEveryTarget fails fast when a target is added to or
// removed from the capability matrix without regenerating the compare data.
func TestSiteDocs_CompareListsEveryTarget(t *testing.T) {
	var capabilities capabilityData
	if _, err := toml.DecodeFile(capabilitiesData, &capabilities); err != nil {
		t.Fatalf("decode capability data: %v", err)
	}
	var compare compareFile
	if _, err := toml.DecodeFile(compareData, &compare); err != nil {
		t.Fatalf("decode compare data: %v", err)
	}
	var wantTargets, gotTargets, wantKinds []string
	for _, target := range capabilities.Targets {
		wantTargets = append(wantTargets, target.ID)
	}
	for _, feature := range capabilities.Features {
		wantKinds = append(wantKinds, feature.ID)
	}
	for _, target := range compare.Targets {
		gotTargets = append(gotTargets, target.ID)
		var gotKinds []string
		for _, kind := range target.Kinds {
			gotKinds = append(gotKinds, kind.ID)
		}
		if !reflect.DeepEqual(gotKinds, wantKinds) {
			t.Errorf("compare.toml target %q lists kinds %v, capabilities.toml lists %v", target.ID, gotKinds, wantKinds)
		}
	}
	if !reflect.DeepEqual(gotTargets, wantTargets) {
		t.Errorf("compare.toml lists targets %v, capabilities.toml lists %v; run UPDATE_GOLDEN=1 go test ./tests/integration/ -run TestSiteDocs_Compare", gotTargets, wantTargets)
	}
}

// TestSiteDocs_ComparePageRendersDefaultPair checks the page a reader without
// JavaScript sees: Claude Code against Codex, one row per spec kind, with the
// synced paths, and a payload the script can parse after minification.
func TestSiteDocs_ComparePageRendersDefaultPair(t *testing.T) {
	zolaPath := lookupPinnedZola(t)
	outputDir := t.TempDir()
	command := exec.Command(zolaPath, "--root", "../../docs/site", "build", "--force", "--minify", "--output-dir", outputDir)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build documentation site: %v\n%s", err, output)
	}
	page := readBuiltFile(t, filepath.Join(outputDir, "docs", "compare", "index.html"))

	var capabilities capabilityData
	if _, err := toml.DecodeFile(capabilitiesData, &capabilities); err != nil {
		t.Fatalf("decode capability data: %v", err)
	}
	for _, feature := range capabilities.Features {
		if !strings.Contains(page, "data-compare-kind="+feature.ID) && !strings.Contains(page, `data-compare-kind="`+feature.ID+`"`) {
			t.Errorf("compare page has no row for %s", feature.ID)
		}
	}
	for _, required := range []string{".claude/agents/test-writer.md", ".codex/agents/test-writer.toml", "compare-targets.js", "Only Claude Code writes by default"} {
		if !strings.Contains(page, required) {
			t.Errorf("compare page is missing %q", required)
		}
	}

	match := regexp.MustCompile(`(?s)data-compare-data[^>]*>(.*?)</script>`).FindStringSubmatch(page)
	if match == nil {
		t.Fatal("compare page has no data payload")
	}
	var payload struct {
		Features []struct {
			ID string `json:"id"`
		} `json:"features"`
		Targets []struct {
			ID       string     `json:"id"`
			Statuses []string   `json:"statuses"`
			Paths    [][]string `json:"paths"`
		} `json:"targets"`
	}
	if err := json.Unmarshal([]byte(match[1]), &payload); err != nil {
		t.Fatalf("parse compare payload: %v", err)
	}
	if len(payload.Targets) != len(capabilities.Targets) || len(payload.Features) != len(capabilities.Features) {
		t.Fatalf("payload has %d targets and %d features, want %d and %d", len(payload.Targets), len(payload.Features), len(capabilities.Targets), len(capabilities.Features))
	}
	for _, target := range payload.Targets {
		if len(target.Paths) != len(payload.Features) || len(target.Statuses) != len(payload.Features) {
			t.Errorf("payload target %s has %d statuses and %d path lists for %d features", target.ID, len(target.Statuses), len(target.Paths), len(payload.Features))
		}
	}
}

// syncCompareSample syncs one target in a fresh directory holding the fixture's
// instructions file and, when kindDir is set, that kind's sample spec. It
// returns every file sync wrote outside the source tree, keyed by slash path.
func syncCompareSample(t *testing.T, fixture, target, kindDir string) map[string]string {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, ".agnostic-ai")
	copyCompareFile(t, filepath.Join(fixture, "AGNOSTIC_AI.md"), filepath.Join(source, "AGNOSTIC_AI.md"))
	if kindDir != "" {
		copyLandingFixture(t, filepath.Join(fixture, kindDir), filepath.Join(source, kindDir))
	}
	config := fmt.Sprintf("version: 1\ntargets:\n  - %s\n", target)
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	root := cli.NewRootCmd("test")
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"sync", "--gitignore=off"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync %s with sample %q: %v", target, kindDir, err)
	}

	files := map[string]string{}
	if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == ".agnostic-ai" {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == "agnostic-ai.yaml" {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		files[rel] = string(body)
		return nil
	}); err != nil {
		t.Fatalf("walk sync output: %v", err)
	}
	return files
}

func addedOutputs(baseline, outputs map[string]string) []string {
	paths := []string{}
	for path, body := range outputs {
		if before, ok := baseline[path]; !ok || before != body {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

func copyCompareFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(dst), err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}
