package cli

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// plainFileLinkFailures names each link Git tracks that the check found
// as a plain file holding the link text, the way Git checks a link out
// under core.symlinks=false. The check then reads that text instead of
// the target, so regenerating does not settle the drift. Only links the
// check reads count: at, under, or above a path in read. dir is the
// directory the links are relative to, and skip lists the links another
// diagnosis already names.
func plainFileLinkFailures(dir string, links, read []string, skip []againstSkillLinkFailure, ref string) ([]againstSkillLinkFailure, error) {
	named := map[string]bool{}
	for _, f := range skip {
		named[f.path] = true
	}
	sort.Strings(links)
	var out []againstSkillLinkFailure
	for _, link := range links {
		if named[link] || !readsLink(link, read) {
			continue
		}
		info, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(link)))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
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

// readsLink reports whether link sits at, under, or above one of the
// paths in read.
func readsLink(link string, read []string) bool {
	for _, p := range read {
		if p == "." || link == p || strings.HasPrefix(link, p+"/") || strings.HasPrefix(p, link+"/") {
			return true
		}
	}
	return false
}

// checkedPaths lists what the check reads, relative to the project: the
// spec inputs `explain --inputs` prints, and the outputs in reports.
func checkedPaths(inputs []string, reports []driftReport) []string {
	var out []string
	for _, in := range inputs {
		if !strings.HasPrefix(in, "builtin:") {
			out = append(out, path.Clean(strings.TrimSuffix(in, "/**")))
		}
	}
	for _, p := range driftGeneratedPaths(reports) {
		out = append(out, filepath.ToSlash(p))
	}
	for _, r := range reports {
		for _, p := range append(append([]string(nil), r.Leftover...), r.Orphaned...) {
			out = append(out, filepath.ToSlash(p))
		}
	}
	return out
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
