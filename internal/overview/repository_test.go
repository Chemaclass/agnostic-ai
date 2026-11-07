package overview

import "testing"

func TestGitHubRepository_NormalizesRemoteForms(t *testing.T) {
	for _, remote := range []string{
		"git@github.com:octo/hello.git",
		"git@github.com:octo/hello",
		"org-123@github.com:octo/hello.git",
		"ssh://git@github.com/octo/hello",
		"ssh://git@github.com:22/octo/hello.git",
		"git://github.com/octo/hello.git",
		"https://github.com/octo/hello",
		"https://github.com/octo/hello.git",
		"https://github.com/octo/hello/",
		"https://GitHub.com/octo/hello.git\n",
		"https://user:secret-token@github.com/octo/hello.git",
		"https://x-access-token@github.com/octo/hello",
	} {
		got := GitHubRepository(remote)
		if got == nil || got.URL != "https://github.com/octo/hello" || got.Name != "octo/hello" {
			t.Errorf("GitHubRepository(%q) = %+v, want https://github.com/octo/hello", remote, got)
		}
	}
}

func TestGitHubRepository_RejectsOtherHostsAndShapes(t *testing.T) {
	for _, remote := range []string{
		"",
		"origin",
		"/srv/git/hello.git",
		"file:///srv/git/hello.git",
		"git@gitlab.com:octo/hello.git",
		"https://gitlab.com/octo/hello",
		"https://github.com.evil.example/octo/hello",
		"https://evil.example/github.com/octo/hello",
		"https://github.com/octo",
		"https://github.com/octo/hello/tree/main",
		"https://github.com/../hello",
		"javascript://github.com/octo/hello",
	} {
		if got := GitHubRepository(remote); got != nil {
			t.Errorf("GitHubRepository(%q) = %+v, want nil", remote, got)
		}
	}
}

func TestBuild_LinksTheProjectRepositoryButNotUserConfiguration(t *testing.T) {
	remote := "git@github.com:octo/hello.git"
	if got := Build(Inputs{OriginURL: remote}).Repository; got == nil || got.URL != "https://github.com/octo/hello" {
		t.Errorf("project view repository = %+v, want https://github.com/octo/hello", got)
	}
	if got := Build(Inputs{OriginURL: remote, Global: true}).Repository; got != nil {
		t.Errorf("user configuration view repository = %+v, want none", got)
	}
}
