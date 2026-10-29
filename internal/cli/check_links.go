package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// plainFileLinkFailures names each link Git tracks that the check found
// as a plain file holding the link text, the way Git checks a link out
// under core.symlinks=false. The check then reads that text instead of
// the target, so regenerating does not settle the drift. dir is the
// directory the links are relative to, and skip lists the links another
// diagnosis already names.
func plainFileLinkFailures(dir string, links []string, skip []againstSkillLinkFailure, ref string) ([]againstSkillLinkFailure, error) {
	named := map[string]bool{}
	for _, f := range skip {
		named[f.path] = true
	}
	sort.Strings(links)
	var out []againstSkillLinkFailure
	for _, link := range links {
		if named[link] {
			continue
		}
		info, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(link)))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("%s: %w", link, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		message := link + ": Git checked out this link as a plain file, as it does with core.symlinks=false, so the check reads the link text instead of its target; where the system can create links, run git config --local core.symlinks true"
		if ref == "" {
			message += ", then git checkout -- " + link
		} else {
			message += " and rerun the check"
		}
		out = append(out, againstSkillLinkFailure{path: link, message: message})
	}
	return out, nil
}

// trackedLinks lists the links Git tracks in the index under the working
// directory, relative to it. Outside a Git work tree there are none.
func trackedLinks() []string {
	out, err := gitOutput(".", nil, "ls-files", "--stage", "-z")
	if err != nil {
		return nil
	}
	var links []string
	for _, rec := range strings.Split(out, "\x00") {
		meta, p, ok := strings.Cut(rec, "\t")
		if ok && strings.HasPrefix(meta, "120000 ") {
			links = append(links, p)
		}
	}
	return links
}

// exportedLinks lists the links the export checked out, relative to the
// project directory.
func (t *againstTree) exportedLinks() []string {
	var links []string
	for p := range t.exported {
		if t.modes[p] != "120000" {
			continue
		}
		if rel, ok := strings.CutPrefix(p, t.prefix); ok {
			links = append(links, rel)
		}
	}
	return links
}
