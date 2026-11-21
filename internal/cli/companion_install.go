package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// companionBuild is one build of the companion for this machine.
type companionBuild struct {
	goos string
	// version is this agnostic-ai's release, which the companion is built for.
	version string
	// source is a checkout's companion folder to build from; empty builds
	// the commit this agnostic-ai came from, fetched by Go.
	source string
	// revision is that commit.
	revision string
	// module is the version Go stamped on this agnostic-ai.
	module string
	// into is the folder the built program is written to.
	into string
}

// goArgs are the arguments of the Go command that builds b, and the
// folder to run it in.
func (b companionBuild) goArgs() (args []string, dir string) {
	linker := "-s -w -X main.version=" + b.version
	if b.goos == "windows" {
		// A windowed program: Windows opens no console for it.
		linker += " -H windowsgui"
	}
	flags := []string{"-v", "-tags", companionTags, "-trimpath", "-ldflags", linker}
	if b.source != "" {
		return append(append([]string{"build"}, flags...), "-o", filepath.Join(b.into, companionProgramName(b.goos)), "."), b.source
	}
	return append(append([]string{"install"}, flags...), companionModule+"@"+b.revision), ""
}

// goEnv is the environment of that command: this one's, with C turned
// on where the companion needs it and the result sent to b.into.
func (b companionBuild) goEnv(base []string) []string {
	env := append([]string(nil), base...)
	if b.goos != "windows" {
		env = append(env, "CGO_ENABLED=1")
	}
	if b.source == "" {
		env = append(env, "GOBIN="+b.into)
	}
	return env
}

// buildProgress turns the Go command's output into one line that says
// how far the build is, and keeps the last lines for a failure.
type buildProgress struct {
	out io.Writer
	// live rewrites one line in place; otherwise only steps are printed.
	live bool

	fetched, compiled int
	last              []string
}

// maxBuildTail is how many of the Go command's last lines a failed build shows.
const maxBuildTail = 30

func (p *buildProgress) line(text string) {
	p.last = append(p.last, text)
	if len(p.last) > maxBuildTail {
		p.last = p.last[1:]
	}
	switch {
	case strings.HasPrefix(text, "go: downloading "):
		p.fetched++
	case text != "" && !strings.ContainsAny(text, " \t:"):
		// With -v, Go prints the path of each package it compiles.
		p.compiled++
	default:
		return
	}
	if p.live {
		_, _ = fmt.Fprintf(p.out, "\r  %d modules fetched, %d packages compiled", p.fetched, p.compiled)
	}
}

func (p *buildProgress) done() {
	if p.live {
		_, _ = fmt.Fprintln(p.out)
		return
	}
	_, _ = fmt.Fprintf(p.out, "  %d modules fetched, %d packages compiled\n", p.fetched, p.compiled)
}

// downloads reports whether b can be the released companion rather than
// a build: on macOS, for an agnostic-ai built from that release's tag.
// A build of any other commit still reports the last release's version,
// so only the version Go stamped from the tag tells them apart.
func (b companionBuild) downloads() bool {
	return b.goos == "darwin" && b.source == "" && releaseVersionRE.MatchString(b.version) && b.module == "v"+b.version
}

// installCompanion puts the companion in place: on macOS the one the
// release ships, otherwise one built here. On macOS, loginEntries are
// where a bare program an earlier install left kept its start at login.
func installCompanion(ctx context.Context, out io.Writer, live bool, tools companionTools, dir string, build companionBuild, loginEntries []string) error {
	if _, known := companionNeeds[build.goos]; !known {
		return fmt.Errorf("the companion does not build on %s", build.goos)
	}
	if build.goos == "darwin" && tools.macOS != nil {
		if version := tools.macOS(); macOSTooOld(version) {
			return fmt.Errorf("the companion needs macOS %d or later, and this Mac runs macOS %s", companionMinMacOS, version)
		}
	}
	staging, err := stageCompanion(dir)
	if err != nil {
		return err
	}
	keepStaging := false
	defer func() {
		if !keepStaging {
			_ = os.RemoveAll(staging)
		}
	}()
	build.into = filepath.Join(staging, "companion")
	if err := os.MkdirAll(build.into, 0o700); err != nil {
		return fmt.Errorf("install the companion: %w", err)
	}

	built, err := obtainCompanion(ctx, out, live, tools, build)
	if err != nil {
		return err
	}
	target := companionFile(dir, build.goos)
	place := func() error { return tools.rename(built, target) }
	if build.goos == "darwin" {
		app := filepath.Join(staging, companionAppName+".app")
		if err := assembleCompanionApp(ctx, tools, dir, built, app, build.version); err != nil {
			return err
		}
		target = companionBundle(dir)
		place = func() error { return replaceCompanionApp(tools.rename, app, target, staging) }
	}
	// An interrupt stops here, before anything installed changes. Past
	// this point the install finishes, so a stopped companion starts again.
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("the install was interrupted and changed nothing: %w", err)
	}
	ctx = context.WithoutCancel(ctx)
	installed, found, err := companionInstalled(dir, build.goos)
	if err != nil {
		return err
	}
	if !found {
		if err := place(); err != nil {
			return fmt.Errorf("install the companion: %w", err)
		}
		if err := recordCompanionInstall(dir, build.revision); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "✓ Installed %s\n", target)
		_, _ = fmt.Fprintln(out, "Start it with: agnostic-ai companion")
		return nil
	}
	keepStaging, err = replaceInstalledCompanion(ctx, out, tools, dir, build, installed, target, place, loginEntries)
	return err
}

// stageCompanion clears old build folders and makes the staging folder
// a new program is written to. The program is moved into place at the
// end, so a failure leaves the installed one alone.
func stageCompanion(dir string) (staging string, err error) {
	if err := removeBuildFolders(dir, time.Now().Add(-companionBuildAge)); err != nil {
		return "", fmt.Errorf("install the companion: %w", err)
	}
	if err := os.MkdirAll(companionFolder(dir), 0o700); err != nil {
		return "", fmt.Errorf("install the companion: %w", err)
	}
	staging, err = os.MkdirTemp(dir, companionBuildPrefix+"*")
	if err != nil {
		return "", fmt.Errorf("install the companion: %w", err)
	}
	return staging, nil
}

// obtainCompanion downloads the released companion when build can be
// one, and builds it otherwise.
func obtainCompanion(ctx context.Context, out io.Writer, live bool, tools companionTools, build companionBuild) (built string, err error) {
	if tools.download != nil && build.downloads() {
		_, _ = fmt.Fprintf(out, "Downloading the companion for agnostic-ai %s.\n", build.version)
		built, err = tools.download(ctx, out, build.version, build.into)
		switch {
		case errors.Is(err, errNoPrebuiltCompanion):
			_, _ = fmt.Fprintln(out, "! This release ships no built companion for this Mac, so it is built here.")
		case errors.Is(err, errProvenanceFailed) && os.Getenv(verifyAttestationEnv) != "1":
			return "", fmt.Errorf("download the companion: %w. If gh failed rather than the archive, %s=0 installs it on the checksum alone", err, verifyAttestationEnv)
		case err != nil:
			// A download that does not verify is never replaced by a build.
			return "", fmt.Errorf("download the companion: %w", err)
		default:
			_, _ = fmt.Fprintln(out, "✓ Downloaded and verified the companion")
		}
	}
	if built == "" {
		return buildCompanion(ctx, out, live, tools, build)
	}
	return built, nil
}

// replaceInstalledCompanion puts the new companion over the installed
// one: it stops a running one, places the new one, and starts it again.
// keepStaging reports that the staging folder holds the only copy of the
// app place could not put back.
func replaceInstalledCompanion(ctx context.Context, out io.Writer, tools companionTools, dir string, build companionBuild, installed, target string, place func() error, loginEntries []string) (keepStaging bool, err error) {
	// Windows does not replace the file of a program that runs.
	ran, err := endCompanion(ctx, tools, installed)
	alone := err == nil
	switch {
	case err != nil && ran:
		// It still runs. A start now would only open its list.
		return false, err
	case err != nil:
		_, _ = fmt.Fprintf(out, "! Could not tell whether a companion runs (%v). If one does, end it and start it again.\n", err)
	case ran:
		_, _ = fmt.Fprintln(out, "✓ Stopped the running companion")
	}
	err = whileHeld(tools, place)
	if _, ok := errors.AsType[companionKeptAside](err); ok {
		keepStaging = true
	}
	if err != nil {
		err = fmt.Errorf("install the companion: %w", err)
	} else {
		_, _ = fmt.Fprintf(out, "✓ Installed %s\n", target)
		installed = companionFile(dir, build.goos)
		if build.goos == "darwin" {
			err = leaveBareCompanion(ctx, out, tools, bareCompanionFile(dir), installed, loginEntries, alone)
		}
		if err == nil {
			err = recordCompanionInstall(dir, build.revision)
		}
	}
	if !ran {
		if err == nil {
			_, _ = fmt.Fprintln(out, "Start it with: agnostic-ai companion")
		}
		return keepStaging, err
	}
	// A companion that ran is started again whatever happened since: the
	// one that is installed now, old or new.
	if startErr := startInstalledCompanion(tools, installed); startErr != nil {
		return keepStaging, errors.Join(err, startErr)
	}
	if err == nil {
		_, _ = fmt.Fprintln(out, "✓ Started it again")
	}
	return keepStaging, err
}

// buildCompanion builds the companion into build.into and returns the program.
func buildCompanion(ctx context.Context, out io.Writer, live bool, tools companionTools, build companionBuild) (string, error) {
	_, _ = fmt.Fprintf(out, "Building the companion for agnostic-ai %s on this machine.\n", build.version)
	var missing []companionNeed
	for _, need := range companionNeeds[build.goos] {
		// Without its check program a need cannot be told apart from met.
		if checkerMissing(missing, need.check[0]) {
			continue
		}
		if !tools.succeeds(ctx, need.check[0], need.check[1:]...) {
			missing = append(missing, need)
			continue
		}
		_, _ = fmt.Fprintf(out, "✓ %s found\n", need.what)
	}
	if len(missing) > 0 {
		return "", missingNeedsError(ctx, tools, missing)
	}
	if build.source == "" && build.revision == "" {
		return "", errors.New("this agnostic-ai does not record the commit it was built from, so it cannot fetch the companion's source; pass --source with the companion folder of an agnostic-ai checkout")
	}
	if build.source != "" {
		if _, err := os.Stat(filepath.Join(build.source, "go.mod")); err != nil {
			return "", fmt.Errorf("--source %s is not the companion folder of an agnostic-ai checkout: %w", build.source, err)
		}
	}
	_, _ = fmt.Fprintln(out, "→ Fetching and compiling. The first build takes a few minutes.")
	progress := &buildProgress{out: out, live: live}
	args, runIn := build.goArgs()
	buildErr := tools.build(ctx, runIn, build.goEnv(os.Environ()), progress.line, args...)
	progress.done()
	if buildErr != nil {
		return "", fmt.Errorf("the build failed (%w):\n%s", buildErr, strings.Join(progress.last, "\n"))
	}
	return builtCompanion(build)
}

// checkerMissing reports whether one of missing is program itself.
func checkerMissing(missing []companionNeed, program string) bool {
	for _, need := range missing {
		if need.what == program {
			return true
		}
	}
	return false
}

// missingNeedsError names every need the machine lacks and how to meet
// them, with one install command for this machine's package manager.
func missingNeedsError(ctx context.Context, tools companionTools, missing []companionNeed) error {
	var whats, fixes []string
	var packaged []companionNeed
	for _, need := range missing {
		whats = append(whats, need.what)
		if need.packages == nil {
			fixes = append(fixes, need.fix)
		} else {
			packaged = append(packaged, need)
		}
	}
	if len(packaged) > 0 {
		fixes = append(fixes, packagesFix(ctx, tools, packaged))
	}
	return fmt.Errorf("the companion needs %s to build. %s, then run this again", andList(whats), strings.Join(fixes, ". "))
}

func packagesFix(ctx context.Context, tools companionTools, needs []companionNeed) string {
	for _, manager := range linuxPackageManagers {
		if tools.succeeds(ctx, manager.name, "--version") {
			return "Run: " + manager.installCommand(needs)
		}
	}
	if len(needs) == 1 {
		return needs[0].fix
	}
	return "Install them (" + installLines(needs) + ")"
}

// whileHeld runs change again for a while when it fails. A program that
// just ended can keep its file a moment longer on Windows.
func whileHeld(tools companionTools, change func() error) error {
	for try := 0; ; try++ {
		err := change()
		if err == nil || try == companionStopTries {
			return err
		}
		tools.pause(companionStopPause)
	}
}

func startInstalledCompanion(tools companionTools, installed string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find the agnostic-ai binary: %w", err)
	}
	if err := tools.start(installed, append(os.Environ(), "AGNOSTIC_AI_BIN="+self)); err != nil {
		return fmt.Errorf("start the companion: %w", err)
	}
	return nil
}

// builtCompanion finds the program the build wrote. `go install` names
// it after the module's folder; `go build -o` uses the name it was given.
func builtCompanion(build companionBuild) (string, error) {
	candidates := []string{filepath.Join(build.into, companionProgramName(build.goos)), filepath.Join(build.into, "companion"), filepath.Join(build.into, "companion.exe")}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
	}
	return "", errors.New("the build finished without writing the companion")
}

// recordCompanionInstall records where this agnostic-ai is and the commit
// it was built from, empty when the build does not record one.
func recordCompanionInstall(dir, revision string) error {
	if err := recordCompanionCommand(dir); err != nil {
		return err
	}
	path := filepath.Join(dir, companionRevisionFile)
	if revision == "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("record the agnostic-ai commit: %w", err)
		}
		return nil
	}
	if err := os.WriteFile(path, []byte(revision+"\n"), 0o600); err != nil {
		return fmt.Errorf("record the agnostic-ai commit: %w", err)
	}
	return nil
}

// recordCompanionCommand writes down where this agnostic-ai is, for the companion to find it.
func recordCompanionCommand(dir string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find the agnostic-ai binary: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("record the agnostic-ai binary: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, companionCommandFile), []byte(self+"\n"), 0o600); err != nil {
		return fmt.Errorf("record the agnostic-ai binary: %w", err)
	}
	return nil
}
