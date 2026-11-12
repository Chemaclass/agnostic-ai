package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type isolation struct {
	Version    int    `json:"schema_version"`
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Evidence   string `json:"evidence_directory"`
}

func (a app) isolationPath() (string, error) {
	path, err := a.command("git", "rev-parse", "--git-path", "target-audit-isolation.json")
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(a.root, path)
	}
	return path, nil
}

func (a app) evidenceDirectory() (string, error) {
	path, err := a.isolationPath()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return filepath.Join(a.root, "local/target-audit"), nil
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	var receipt isolation
	if err = json.Unmarshal(b, &receipt); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	root, err := filepath.EvalSymlinks(a.root)
	if err != nil {
		return "", fmt.Errorf("%s: %w", a.root, err)
	}
	if receipt.Version != 1 || receipt.Repository != root || !filepath.IsAbs(receipt.Evidence) {
		return "", fmt.Errorf("%s: invalid isolation receipt", path)
	}
	return receipt.Evidence, nil
}

func (a app) isolate(args []string) (string, error) {
	f := flag.NewFlagSet("isolate", flag.ContinueOnError)
	out := f.String("out", "", "new dedicated worktree path")
	commit := f.String("commit", "", "explicit committed inputs only, excluding working changes")
	if err := f.Parse(args); err != nil {
		return "", err
	}
	if *out == "" || len(f.Args()) != 0 {
		return "", fmt.Errorf("isolate requires --out <new-worktree> and optional --commit <revision>")
	}
	status, err := a.command("git", "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return "", err
	}
	embedded, err := a.command("git", "ls-files", "--others", "--ignored", "--exclude-standard", "--", "internal/builtins/data")
	if err != nil {
		return "", err
	}
	if *commit == "" && (status != "" || embedded != "") {
		return "", fmt.Errorf("isolate: working changes exist; commit them first or use --commit <revision> to audit committed inputs only")
	}
	if *commit == "" {
		*commit = "HEAD"
	}
	revision, err := a.command("git", "rev-parse", "--verify", *commit+"^{commit}")
	if err != nil {
		return "", err
	}
	destination, err := filepath.Abs(*out)
	if err != nil {
		return "", fmt.Errorf("%s: %w", *out, err)
	}
	if _, err = os.Lstat(destination); !os.IsNotExist(err) {
		return "", fmt.Errorf("%s: a new worktree path is required", destination)
	}
	evidence, err := a.evidenceDirectory()
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(evidence, 0o755); err != nil {
		return "", fmt.Errorf("%s: %w", evidence, err)
	}
	evidence, err = filepath.EvalSymlinks(evidence)
	if err != nil {
		return "", fmt.Errorf("%s: %w", evidence, err)
	}
	if _, err = a.command("git", "worktree", "add", "--detach", destination, revision); err != nil {
		return "", err
	}
	destination, err = filepath.EvalSymlinks(destination)
	if err != nil {
		return "", fmt.Errorf("%s: %w", destination, err)
	}
	isolated := app{root: destination}
	path, err := isolated.isolationPath()
	if err != nil {
		return "", err
	}
	receipt := isolation{Version: 1, Repository: destination, Commit: revision, Evidence: evidence}
	b, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode isolation receipt: %w", err)
	}
	if err = atomicWrite(path, append(b, '\n')); err != nil {
		return "", fmt.Errorf("retain worktree %s for recovery: %w", destination, err)
	}
	return strings.TrimSpace(destination) + "\n", nil
}
