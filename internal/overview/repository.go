package overview

import (
	"net/url"
	"regexp"
	"strings"
)

// Repository is the GitHub repository a project's origin remote points at.
type Repository struct {
	// URL is the repository's https page, with no credentials.
	URL string `json:"url"`
	// Name is owner/repo.
	Name string `json:"name"`
}

var githubPathPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// GitHubRepository reads a Git remote URL in its SSH, scp-like, git or
// HTTPS form and returns the GitHub repository it names, or nil for any
// other host or shape. The result never carries the remote's userinfo.
func GitHubRepository(remote string) *Repository {
	remote = strings.TrimSpace(remote)
	var host, path string
	if u, err := url.Parse(remote); err == nil && u.Scheme != "" && u.Host != "" {
		switch strings.ToLower(u.Scheme) {
		case "https", "http", "ssh", "git", "git+ssh", "ssh+git":
		default:
			return nil
		}
		host, path = u.Hostname(), u.Path
	} else if at := strings.Index(remote, "@"); at >= 0 && !strings.Contains(remote, "://") {
		hostPath := remote[at+1:]
		h, p, ok := strings.Cut(hostPath, ":")
		if !ok {
			return nil
		}
		host, path = h, p
	} else {
		return nil
	}
	if !strings.EqualFold(host, "github.com") {
		return nil
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	owner, repo, ok := strings.Cut(path, "/")
	if !ok || !githubPathPart.MatchString(owner) || !githubPathPart.MatchString(repo) || repo == "." || repo == ".." || owner == "." || owner == ".." {
		return nil
	}
	return &Repository{URL: "https://github.com/" + owner + "/" + repo, Name: owner + "/" + repo}
}
