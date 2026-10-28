package emit

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// maxLinkHops matches the limit most systems put on links followed while
// opening one path.
const maxLinkHops = 40

// sourceDir is the project's spec folder, which users may link to share
// specs between projects.
const sourceDir = ".agnostic-ai"

// CheckProjectPath refuses a write to path, inside the project at root,
// when a link along it leads outside root or into a Git folder: a cloned
// repository can commit such links in its output folders to plant Git
// hooks or edit the user's global tool settings. A path that lies
// outside root before any link is followed is not checked here.
func CheckProjectPath(root, path string) error {
	return checkProjectPath(root, path, "write", false, false)
}

// CheckBookkeepingPath is CheckProjectPath for a file sync keeps for
// itself in the spec folder, such as its lock or ledger, which follows a
// linked spec folder so a project sharing its specs still syncs. It
// follows the link only when sync reads specs from that folder (see
// holdsSpecs). The file itself must not be a link, and it never lands in
// a Git folder.
func CheckBookkeepingPath(root, path string) error {
	return checkProjectPath(root, path, "write", false, true)
}

// confine is checkProjectPath for a session write or removal, whose
// relative paths are relative to the project root, the working
// directory. parentsOnly checks only the folders above path, for a
// removal: removing a link deletes the link itself, never what it
// points to.
func confine(path, verb string, parentsOnly bool) error {
	return checkProjectPath(".", path, verb, parentsOnly, false)
}

func checkProjectPath(root, path, verb string, parentsOnly, bookkeeping bool) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("%s %s: %w", verb, path, err)
	}
	clean, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("%s %s: %w", verb, path, err)
	}
	rel, err := filepath.Rel(absRoot, clean)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	realRoot, _, err := resolvePath(absRoot, "")
	if err != nil {
		return fmt.Errorf("%s %s: %w", verb, path, err)
	}
	allowed := []string{realRoot}
	if bookkeeping {
		if splitPath(rel)[0] != sourceDir || len(splitPath(rel)) != 2 {
			return fmt.Errorf("refusing to %s %s: not a file directly in %s", verb, path, sourceDir)
		}
		if info, err := os.Lstat(clean); err == nil && info.Mode()&fs.ModeSymlink != 0 {
			target, _ := os.Readlink(clean)
			return fmt.Errorf("refusing to %s %s: it is a link to %s", verb, path, target)
		}
		source, _, err := resolvePath(filepath.Join(absRoot, sourceDir), "")
		if err != nil {
			return fmt.Errorf("%s %s: %w", verb, path, err)
		}
		if holdsSpecs(source) {
			allowed = append(allowed, source)
		}
	}
	// Resolve the path as the system will open it: an unclean path such
	// as `link/../x` goes up from where link points, not from link.
	raw := path
	if !filepath.IsAbs(raw) {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("%s %s: %w", verb, path, err)
		}
		raw = cwd + string(filepath.Separator) + raw
	}
	if parentsOnly {
		raw = strings.TrimRight(raw, `/`+string(filepath.Separator))
		if i := strings.LastIndexAny(raw, `/`+string(filepath.Separator)); i >= 0 {
			raw = raw[:i+1]
		}
	}
	real, link, err := resolvePath(raw, realRoot)
	if err != nil {
		return fmt.Errorf("refusing to %s %s: %w", verb, path, err)
	}
	where := ""
	if !withinAny(allowed, real) {
		where = "outside the project"
	} else if inGitFolder(realRoot, real) {
		where = "inside a Git folder"
	}
	if where == "" {
		return nil
	}
	if link == "" {
		return fmt.Errorf("refusing to %s %s: it is %s", verb, path, where)
	}
	name, _ := filepath.Rel(realRoot, link)
	return fmt.Errorf("refusing to %s %s: %s is a link, and the path leads to %s, %s", verb, path, name, real, where)
}

// resolvePath returns where path lands once every link along it is
// followed, one part at a time as the system does, so `..` after a link
// climbs from the link's target. A link to a target that does not exist
// yet is followed too, since a write would create it. firstLink is the
// first link met under within, spelled from its real parent folder.
func resolvePath(path, within string) (real, firstLink string, err error) {
	vol := filepath.VolumeName(path)
	cur := vol + string(filepath.Separator)
	pending := splitPath(path[len(vol):])
	for hops := 0; len(pending) > 0; {
		part := pending[0]
		pending = pending[1:]
		switch part {
		case ".":
			continue
		case "..":
			cur = filepath.Dir(cur)
			continue
		}
		next := filepath.Join(cur, part)
		info, err := os.Lstat(next)
		if err != nil || info.Mode()&fs.ModeSymlink == 0 {
			cur = next
			continue
		}
		if hops++; hops > maxLinkHops {
			return "", "", errors.New("too many links")
		}
		target, err := os.Readlink(next)
		if err != nil {
			return "", "", err
		}
		if firstLink == "" && within != "" && pathWithin(within, next) {
			firstLink = next
		}
		var rest string
		cur, rest = linkTargetStart(cur, target, runtime.GOOS == "windows")
		pending = append(splitPath(rest), pending...)
	}
	return cur, firstLink, nil
}

// linkTargetStart returns the folder a link's target is read from, and
// the rest of the target to follow from there. dir is the link's folder.
// On Windows a target with a drive, such as `C:\x` or the drive-relative
// `C:x`, starts at that drive's root, and one starting with a slash but
// no drive, which Git writes for a committed `/x`, starts at the root of
// dir's drive: filepath.IsAbs calls neither absolute.
func linkTargetStart(dir, target string, windows bool) (start, rest string) {
	if !windows {
		if strings.HasPrefix(target, "/") {
			return "/", target
		}
		return dir, target
	}
	if vol := windowsVolume(target); vol != "" {
		return vol + `\`, target[len(vol):]
	}
	if strings.HasPrefix(target, `\`) || strings.HasPrefix(target, "/") {
		return windowsVolume(dir) + `\`, target
	}
	return dir, target
}

// windowsVolume returns the drive (`C:`) or share (`\\host\share`) that
// starts a Windows path, or "" when it has none. It parses the string
// itself so the rule holds, and is tested, on every system.
func windowsVolume(p string) string {
	if len(p) >= 2 && p[1] == ':' && (p[0] >= 'a' && p[0] <= 'z' || p[0] >= 'A' && p[0] <= 'Z') {
		return p[:2]
	}
	isSep := func(b byte) bool { return b == '\\' || b == '/' }
	if len(p) < 3 || !isSep(p[0]) || !isSep(p[1]) || isSep(p[2]) {
		return ""
	}
	end, seps := 2, 0
	for end < len(p) {
		if isSep(p[end]) {
			if seps++; seps == 2 {
				break
			}
		}
		end++
	}
	return p[:end]
}

// specEntries are the spec folder entries sync reads by default: the
// shared instructions and one folder per spec kind.
var specEntries = []string{
	"AGNOSTIC_AI.md", "agents", "skills", "rules", "hooks", "mcps",
	"commands", "settings", "reviews", "environments", "ignore",
}

// holdsSpecs reports whether dir, a resolved spec folder, holds an entry
// sync reads as specs. A link to a folder of other files, such as a tool
// folder or a Git folder, does not make sync keep its files there.
func holdsSpecs(dir string) bool {
	for _, name := range specEntries {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

func splitPath(path string) []string {
	return strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == filepath.Separator })
}

// pathWithin reports whether path is root or lies under it.
func pathWithin(root, path string) bool {
	return path == root || strings.HasPrefix(path, strings.TrimSuffix(root, string(filepath.Separator))+string(filepath.Separator))
}

func withinAny(roots []string, path string) bool {
	for _, root := range roots {
		if pathWithin(root, path) {
			return true
		}
	}
	return false
}

// inGitFolder reports whether path, already resolved, lies in a folder
// Git treats as .git (see gitFolderName), such as a vendored or sibling clone's,
// or in the Git directory of the checkout at root, which a linked
// worktree keeps elsewhere. The folders holding root itself do not
// count.
func inGitFolder(root, path string) bool {
	var git []fs.FileInfo
	if gitDir, common, ok := gitDirsAt(root); ok {
		for _, dir := range []string{gitDir, common} {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				git = append(git, info)
			}
		}
	}
	for dir := path; !pathWithin(dir, root); dir = filepath.Dir(dir) {
		if gitFolderName(filepath.Base(dir), runtime.GOOS) {
			return true
		}
		if len(git) > 0 {
			if info, err := os.Stat(dir); err == nil {
				for _, folder := range git {
					if os.SameFile(info, folder) {
						return true
					}
				}
			}
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return false
}

// gitFolderName reports whether one path segment names a .git folder:
// in any letter case and with trailing dots or spaces, which
// case-insensitive and Windows file systems drop, and on Windows also as
// the short name GIT~1 or with a colon, which names an NTFS stream such
// as `.git::$INDEX_ALLOCATION`.
func gitFolderName(seg, goos string) bool {
	trimmed := strings.TrimRight(seg, ". ")
	if strings.EqualFold(trimmed, ".git") {
		return true
	}
	return goos == "windows" && (strings.EqualFold(trimmed, "git~1") || strings.HasPrefix(strings.ToLower(seg), ".git:"))
}
