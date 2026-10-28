package config

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

// LinkRefusal is the error for a path whose links lead outside the
// project or into a Git folder.
type LinkRefusal struct{ msg string }

func (e *LinkRefusal) Error() string { return e.msg }

func refuse(format string, args ...any) error {
	return &LinkRefusal{msg: fmt.Sprintf(format, args...)}
}

// CheckProjectLinks refuses to verb path, inside the project at root,
// when a link along it leads outside root or into a Git folder: a cloned
// repository can commit such links in its output folders to plant Git
// hooks or edit the user's global tool settings. parentsOnly checks only
// the folders above path, for a removal or an exclusive create, which
// never follow a link at path itself. A path that lies outside root
// before any link is followed is left to UnsafeProjectPathIn.
func CheckProjectLinks(root, path, verb string, parentsOnly bool) error {
	return checkProjectLinks(root, path, verb, parentsOnly, false)
}

// CheckBookkeepingPath is CheckProjectLinks for a file sync keeps for
// itself in the spec folder, such as its lock or ledger, which follows a
// linked spec folder so a project sharing its specs still syncs. It
// follows the link only when sync reads specs from that folder (see
// holdsSpecs). The file itself must not be a link, and it never lands in
// a Git folder.
func CheckBookkeepingPath(root, path string) error {
	return checkProjectLinks(root, path, "write", false, true)
}

func checkProjectLinks(root, path, verb string, parentsOnly, bookkeeping bool) error {
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
		if splitPath(rel)[0] != SourceBaseDir || len(splitPath(rel)) != 2 {
			return refuse("refusing to %s %s: not a file directly in %s", verb, path, SourceBaseDir)
		}
		if info, err := os.Lstat(clean); err == nil && info.Mode()&fs.ModeSymlink != 0 {
			target, _ := os.Readlink(clean)
			return refuse("refusing to %s %s: it is a link to %s", verb, path, target)
		}
		source, _, err := resolvePath(filepath.Join(absRoot, SourceBaseDir), "")
		if err != nil {
			return fmt.Errorf("%s %s: %w", verb, path, err)
		}
		if !isWithin(realRoot, source) {
			if !holdsSpecs(source) {
				return refuse("refusing to %s %s: %s is a link to %s, which holds no %s, so it is not a spec folder sync can keep its files in", verb, path, SourceBaseDir, source, specMarker)
			}
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
		raw = strings.TrimRightFunc(raw, func(r rune) bool { return r == '/' || r == filepath.Separator })
		if i := strings.LastIndexAny(raw, `/`+string(filepath.Separator)); i >= 0 {
			raw = raw[:i+1]
		}
	}
	real, link, err := resolvePath(raw, realRoot)
	if err != nil {
		return refuse("refusing to %s %s: %v", verb, path, err)
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
		return refuse("refusing to %s %s: it is %s", verb, path, where)
	}
	name, _ := filepath.Rel(realRoot, link)
	return refuse("refusing to %s %s: %s is a link, and the path leads to %s, %s", verb, path, name, real, where)
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

// specMarker is the shared instructions file every synced spec folder
// holds. Tool folders such as ~/.claude also hold agents/ or skills/, so
// only this file marks a folder as one sync reads specs from.
const specMarker = "AGNOSTIC_AI.md"

// holdsSpecs reports whether dir, a resolved spec folder, holds the spec
// marker. A link to a folder of other files, such as a tool folder or a
// Git folder, does not make sync keep its files there.
func holdsSpecs(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, specMarker))
	return err == nil && info.Mode().IsRegular()
}

func splitPath(path string) []string {
	return strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == filepath.Separator })
}

// pathWithin reports whether path is root or lies under it.
func pathWithin(root, path string) bool {
	return path == root || strings.HasPrefix(path, strings.TrimSuffix(root, string(filepath.Separator))+string(filepath.Separator))
}

// isWithin is pathWithin that also matches when the spelling differs but
// the folders are the same, as letter case can on macOS and Windows. It
// compares the nearest existing folder above path, and each one above
// that, with root.
func isWithin(root, path string) bool {
	if pathWithin(root, path) {
		return true
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		return false
	}
	for dir := path; ; dir = filepath.Dir(dir) {
		if info, err := os.Stat(dir); err == nil && os.SameFile(info, rootInfo) {
			return true
		}
		if filepath.Dir(dir) == dir {
			return false
		}
	}
}

func withinAny(roots []string, path string) bool {
	for _, root := range roots {
		if isWithin(root, path) {
			return true
		}
	}
	return false
}

// inGitFolder reports whether path, already resolved, lies in a folder
// Git treats as .git (see GitDirName), such as a vendored or sibling clone's,
// or in the Git directory of the checkout at root, which a linked
// worktree keeps elsewhere. The folders holding root itself do not
// count.
func inGitFolder(root, path string) bool {
	var git []fs.FileInfo
	if gitDir, common, ok := GitDirs(root); ok {
		for _, dir := range []string{gitDir, common} {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				git = append(git, info)
			}
		}
	}
	rootInfo, _ := os.Stat(root)
	for dir := path; !pathWithin(dir, root); dir = filepath.Dir(dir) {
		info, statErr := os.Stat(dir)
		if statErr == nil && rootInfo != nil && os.SameFile(info, rootInfo) {
			break
		}
		if GitDirName(filepath.Base(dir)) {
			return true
		}
		if statErr == nil {
			for _, folder := range git {
				if os.SameFile(info, folder) {
					return true
				}
			}
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return false
}

// GitDirs returns the Git directory of a checkout whose top folder is
// dir, and the common directory a linked worktree shares with its main
// checkout. Both are the same folder in a main checkout.
func GitDirs(dir string) (gitDir, common string, ok bool) {
	dotGit := filepath.Join(dir, ".git")
	info, err := os.Stat(dotGit)
	if err != nil {
		return "", "", false
	}
	gitDir = dotGit
	if !info.IsDir() {
		data, err := os.ReadFile(dotGit)
		if err != nil {
			return "", "", false
		}
		ref, found := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir: ")
		if !found {
			return "", "", false
		}
		if !filepath.IsAbs(ref) {
			ref = filepath.Join(dir, ref)
		}
		gitDir = ref
	}
	common = gitDir
	if data, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		ref := strings.TrimSpace(string(data))
		if !filepath.IsAbs(ref) {
			ref = filepath.Join(gitDir, ref)
		}
		common = ref
	}
	return gitDir, common, true
}
