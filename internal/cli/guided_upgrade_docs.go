package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
)

type releaseDoc struct{ Version, Path, Link string }

var releaseDocPath = regexp.MustCompile(`^docs/site/content/updates/([0-9]{4}-[0-9]{2}-[0-9]{2})-v([0-9]+\.[0-9]+\.[0-9]+)\.md$`)

func selectedReleaseDocs(paths []string, current, latest string) []releaseDoc {
	var docs []releaseDoc
	for _, path := range paths {
		m := releaseDocPath.FindStringSubmatch(path)
		if len(m) != 3 || !newerStableRelease(m[2], current) || newerStableRelease(m[2], latest) {
			continue
		}
		docs = append(docs, releaseDoc{Version: m[2], Path: path, Link: "https://agnostic-ai.org/updates/" + m[1] + "-v" + m[2] + "/"})
	}
	slices.SortFunc(docs, func(a, b releaseDoc) int {
		if newerStableRelease(a.Version, b.Version) {
			return 1
		}
		if newerStableRelease(b.Version, a.Version) {
			return -1
		}
		return 0
	})
	return docs
}

func fetchReleaseDocs(ctx context.Context, client *http.Client, current, latest string) (string, string) {
	treeURL := "https://api.github.com/repos/" + repoOwner + "/" + repoName + "/git/trees/v" + latest + "?recursive=1"
	return fetchReleaseDocsFrom(ctx, client, treeURL, "https://api.github.com/repos/"+repoOwner+"/"+repoName+"/releases", upgradeChangelogBase+"v"+latest+"/", current, latest)
}

func fetchReleaseDocsFrom(ctx context.Context, client *http.Client, treeURL, releaseURL, rawBase, current, latest string) (string, string) {
	data, err := readUpgradeDoc(ctx, client, treeURL, 4<<20)
	if err != nil {
		return "", err.Error()
	}
	var tree struct {
		Tree      []struct{ Path string }
		Truncated bool
	}
	if err := json.Unmarshal(data, &tree); err != nil {
		return "", fmt.Sprintf("parse release docs index: %v", err)
	}
	if tree.Truncated {
		return "", "release docs index is incomplete"
	}
	paths := make([]string, 0, len(tree.Tree))
	for _, entry := range tree.Tree {
		paths = append(paths, entry.Path)
	}
	docs := selectedReleaseDocs(paths, current, latest)
	versions, indexErr := publishedUpgradeVersions(ctx, client, releaseURL, current, latest)
	if indexErr == nil {
		byVersion := map[string]releaseDoc{}
		for _, doc := range docs {
			byVersion[doc.Version] = doc
		}
		docs = nil
		for _, version := range versions {
			doc, ok := byVersion[version]
			if !ok {
				doc = releaseDoc{Version: version, Link: releasesHTMLURL + "/tag/v" + version}
			}
			docs = append(docs, doc)
		}
	}
	if len(docs) == 0 || !versionsEqual(docs[len(docs)-1].Version, latest) {
		return "", "the docs index has no page for the target release"
	}
	guidance := make([]string, len(docs))
	missing := make([]string, len(docs))
	var workers sync.WaitGroup
	jobs := make(chan int)
	for range 3 {
		workers.Go(func() {
			for i := range jobs {
				doc := docs[i]
				title := fmt.Sprintf("Release %s upgrade guidance: %s\n", doc.Version, doc.Link)
				if doc.Path == "" {
					missing[i] = doc.Version + ": no upgrade document published"
					guidance[i] = title + "Docs unavailable; review this release page for manual steps.\n"
					continue
				}
				data, err := readUpgradeDoc(ctx, client, rawBase+doc.Path, 128<<10)
				if err != nil {
					missing[i] = doc.Version + ": " + err.Error()
					guidance[i] = title + "Docs unavailable; review this page for manual steps.\n"
					continue
				}
				guidance[i] = title + upgradeDocSection(string(data))
			}
		})
	}
	for i := range docs {
		jobs <- i
	}
	close(jobs)
	workers.Wait()
	missing = slices.DeleteFunc(missing, func(s string) bool { return s == "" })
	if indexErr != nil {
		missing = append(missing, "could not verify the full skipped-release list: "+indexErr.Error())
	}
	return cleanUpgradeGuidance(strings.Join(guidance, "\n")), strings.Join(missing, "; ")
}

func readUpgradeDoc(ctx context.Context, client *http.Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", url, err)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", url, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s: document exceeds %d bytes", url, limit)
	}
	return data, nil
}

var upgradeDocLink = regexp.MustCompile(`\]\(@/([^)]*)\.md([^)]*)\)`)

func upgradeDocSection(doc string) string {
	if strings.HasPrefix(doc, "+++\n") {
		if end := strings.Index(doc[4:], "\n+++"); end >= 0 {
			doc = doc[4+end+4:]
		}
	}
	lines := strings.Split(doc, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "## Upgrad") {
			start = i
			break
		}
	}
	if start < 0 {
		return strings.TrimSpace(doc) + "\n"
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	section := strings.Join(lines[start:end], "\n")
	section = upgradeDocLink.ReplaceAllString(section, "](https://agnostic-ai.org/$1/$2)")
	return strings.TrimSpace(section) + "\n"
}

func publishedUpgradeVersions(ctx context.Context, client *http.Client, url, current, latest string) ([]string, error) {
	var versions []string
	for page := 1; page <= 10; page++ {
		data, err := readUpgradeDoc(ctx, client, fmt.Sprintf("%s?per_page=100&page=%d", url, page), 2<<20)
		if err != nil {
			return nil, err
		}
		var releases []struct {
			TagName           string `json:"tag_name"`
			Draft, Prerelease bool
		}
		if err := json.Unmarshal(data, &releases); err != nil {
			return nil, fmt.Errorf("parse published releases: %w", err)
		}
		for _, release := range releases {
			version := strings.TrimPrefix(release.TagName, "v")
			if release.Draft || release.Prerelease || !stableRelease(version) {
				continue
			}
			if newerStableRelease(version, current) && !newerStableRelease(version, latest) {
				versions = append(versions, version)
			}
		}
		if len(releases) < 100 {
			slices.SortFunc(versions, func(a, b string) int {
				if newerStableRelease(a, b) {
					return 1
				}
				if newerStableRelease(b, a) {
					return -1
				}
				return 0
			})
			return slices.Compact(versions), nil
		}
	}
	return nil, fmt.Errorf("published release list exceeds its page limit")
}
