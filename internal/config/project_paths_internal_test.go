package config

import "testing"

func TestGitDirName_MatchesWindowsAliasesOnWindowsOnly(t *testing.T) {
	for _, seg := range []string{"GIT~1", "git~1 ", ".git::$INDEX_ALLOCATION", "x:y"} {
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
	for _, seg := range []string{".github", "git", ".gitignore", "git~2"} {
		if gitDirName(seg, "windows") {
			t.Errorf("%q: want a plain name", seg)
		}
	}
}
