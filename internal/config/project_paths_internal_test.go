package config

import (
	"runtime"
	"testing"
)

func TestGitDirName_MatchesWindowsAliasesOnWindowsOnly(t *testing.T) {
	for _, seg := range []string{"GIT~1", "git~1 ", ".git::$INDEX_ALLOCATION", ".GIT.:x", "git~1:$DATA"} {
		if !gitDirName(seg, "windows") {
			t.Errorf("%q: want a Git folder name on Windows", seg)
		}
		if gitDirName(seg, "linux") {
			t.Errorf("%q: want a plain name on Linux", seg)
		}
	}
	for _, seg := range []string{".git", ".GIT", ".git.", ".git "} {
		if !gitDirName(seg, "linux") || !gitDirName(seg, "windows") {
			t.Errorf("%q: want a Git folder name everywhere", seg)
		}
	}
	for _, seg := range []string{".github", "git", ".gitignore", "git~2", "x:y", "git:commit", ":.git"} {
		if gitDirName(seg, "windows") {
			t.Errorf("%q: want a plain name", seg)
		}
	}
}

func TestNamesDataStream_MatchesAColonOnWindowsOnly(t *testing.T) {
	stream := []string{".claude/commands/git:commit.md", "dir/a:b/c", "dir/x:"}
	if runtime.GOOS == "windows" {
		stream = append(stream, `C:\p\git:commit.md`)
		if namesDataStream(`C:\p\git\commit.md`, "windows") {
			t.Errorf("a drive letter alone reads as a data stream")
		}
	}
	for _, p := range stream {
		if !namesDataStream(p, "windows") {
			t.Errorf("%q: want a data stream name on Windows", p)
		}
		if namesDataStream(p, "linux") {
			t.Errorf("%q: want a plain path on Linux", p)
		}
	}
	for _, p := range []string{".qwen/commands/git/commit.md", "plain.md"} {
		if namesDataStream(p, "windows") {
			t.Errorf("%q: want a plain path on Windows", p)
		}
	}
}
