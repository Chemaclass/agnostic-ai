package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

const (
	// companionModule is the companion's Go module, in this repository's companion folder.
	companionModule = "github.com/chemaclass/agnostic-ai/companion"
	companionName   = "agnostic-ai-companion"
	// companionTags turn on the companion's release build; private_mac_apis
	// lets its windows be see-through on macOS and changes nothing elsewhere.
	companionTags = "production private_mac_apis"
	// companionCommandFile holds the path of the agnostic-ai command that
	// installed or last started the companion. The companion reads it to
	// find this command when it starts at login, with no terminal's PATH.
	companionCommandFile = "companion-command"
	// companionRevisionFile holds the commit of the agnostic-ai that
	// installed the companion. Builds between two releases share a version.
	companionRevisionFile = "companion-revision"
	// companionBuildPrefix names the folders an install builds in.
	companionBuildPrefix = ".companion-build-"
	// companionSettingsFile is where the companion keeps its place and shortcut.
	companionSettingsFile = "companion.json"

	// The companion takes these as its only argument. A companion that
	// already runs receives them from the one started with them.
	companionQuitFlag     = "--quit"
	companionLoginOffFlag = "--disable-start-at-login"
	companionLoginOnFlag  = "--enable-start-at-login"
	// companionNotRunningExit is how a companion started with
	// companionQuitFlag ends when no other companion runs.
	companionNotRunningExit = 3

	// companionAnswerWait is how long one of those calls may take.
	companionAnswerWait = 5 * time.Second
	// companionDownloadWait bounds the whole download, so a slow link still
	// fetches the archive.
	companionDownloadWait = 10 * time.Minute
	// companionMinMacOS is the oldest macOS the companion runs on: the
	// oldest the Go toolchain in companion/go.mod supports.
	companionMinMacOS = 13
	// A companion asked to quit is given companionStopTries pauses of
	// companionStopPause to end.
	companionStopTries = 50
	companionStopPause = 100 * time.Millisecond
)

// companionNeed is one thing the build needs on this machine.
type companionNeed struct {
	what string
	// check succeeds when the need is met.
	check []string
	// fix says how to meet it.
	fix string
}

var needsGo = companionNeed{what: "Go", check: []string{"go", "version"}, fix: "Install Go from https://go.dev/dl/"}

// companionNeeds lists what the build needs on each system. The
// companion draws its windows with the system's own web view, which on
// macOS and Linux is reached through C.
var companionNeeds = map[string][]companionNeed{
	"darwin": {
		needsGo,
		{what: "the Xcode Command Line Tools", check: []string{"xcode-select", "-p"}, fix: "Run: xcode-select --install"},
	},
	"linux": {
		needsGo,
		{what: "a C compiler", check: []string{"cc", "--version"}, fix: "Install gcc (Debian and Ubuntu: sudo apt install build-essential)"},
		{what: "GTK 4, WebKitGTK 6.0 and X11 for developers", check: []string{"pkg-config", "--exists", "gtk4", "webkitgtk-6.0", "x11"}, fix: "Install them (Debian and Ubuntu: sudo apt install pkg-config libgtk-4-dev libwebkitgtk-6.0-dev libx11-dev)"},
	},
	"windows": {needsGo},
}

// companionTools are the programs the command runs, swapped in tests.
type companionTools struct {
	// succeeds reports whether a short check command exits with 0.
	succeeds func(ctx context.Context, name string, args ...string) bool
	// build runs the Go command, sending every line it prints to lines.
	build func(ctx context.Context, dir string, env []string, lines func(string), args ...string) error
	// download writes the released macOS companion for version into a
	// folder and returns its path; nil where nothing is downloaded.
	download func(out io.Writer, version, into string) (string, error)
	// start runs the companion apart from this terminal.
	start func(path string, env []string) error
	// builtFor asks a companion which agnostic-ai release it was built for.
	builtFor func(ctx context.Context, path string) string
	// quit asks the running companion to end and reports whether one ran.
	quit func(ctx context.Context, path string) (running bool, err error)
	// loginOff removes the companion from what starts at login.
	loginOff func(ctx context.Context, path string) error
	// loginOn adds the companion to what starts at login.
	loginOn func(ctx context.Context, path string) error
	sign    func(ctx context.Context, app string) error
	// rename moves a file or folder, as os.Rename does.
	rename func(from, to string) error
	// macOS is this Mac's system version, such as "14.5"; empty elsewhere
	// or when unknown.
	macOS func() string
	// pause waits between two looks at a companion that is ending.
	pause func(time.Duration)
}

func systemCompanionTools() companionTools {
	return companionTools{
		succeeds: func(ctx context.Context, name string, args ...string) bool {
			return exec.CommandContext(ctx, name, args...).Run() == nil
		},
		build: runGoBuild,
		download: func(out io.Writer, version, into string) (string, error) {
			return downloadCompanion(out, &http.Client{Timeout: companionDownloadWait}, releasesBaseURL, version, machineArch(), into, ghProvenance)
		},
		start:    startCompanion,
		builtFor: companionBuiltFor,
		quit:     quitCompanion,
		loginOff: func(ctx context.Context, path string) error { return askCompanion(ctx, path, companionLoginOffFlag) },
		loginOn:  func(ctx context.Context, path string) error { return askCompanion(ctx, path, companionLoginOnFlag) },
		sign:     signCompanion,
		rename:   os.Rename,
		macOS:    macOSVersion,
		pause:    time.Sleep,
	}
}

// runGoBuild runs `go args...` in dir and hands each line of its output to lines.
func runGoBuild(ctx context.Context, dir string, env []string, lines func(string), args ...string) error {
	command := exec.CommandContext(ctx, "go", args...)
	command.Dir = dir
	command.Env = env
	reader, writer := io.Pipe()
	command.Stdout, command.Stderr = writer, writer
	var read sync.WaitGroup
	read.Go(func() {
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
		for scanner.Scan() {
			lines(scanner.Text())
		}
		// Whatever is left is thrown away so the command never blocks on a full pipe.
		_, _ = io.Copy(io.Discard, reader)
	})
	err := command.Run()
	_ = writer.Close()
	read.Wait()
	return err
}

func startCompanion(path string, env []string) error {
	command := exec.Command(path)
	command.Env = env
	command.SysProcAttr = companionSysProcAttr()
	if err := command.Start(); err != nil {
		return err
	}
	// The companion outlives this command; nothing waits for it.
	return command.Process.Release()
}

func companionBuiltFor(ctx context.Context, path string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func quitCompanion(ctx context.Context, path string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, companionAnswerWait)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, companionQuitFlag).CombinedOutput()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &exit) && exit.ExitCode() == companionNotRunningExit:
		return false, nil
	default:
		return false, withWhatItSaid(err, out)
	}
}

// withWhatItSaid adds what a failed program printed to its error.
func withWhatItSaid(err error, out []byte) error {
	if said := strings.TrimSpace(string(out)); said != "" {
		return fmt.Errorf("%w: %s", err, said)
	}
	return err
}

// askCompanion starts the companion with one of the flags it takes as
// its only argument and waits for it to end.
func askCompanion(ctx context.Context, path, flag string) error {
	ctx, cancel := context.WithTimeout(ctx, companionAnswerWait)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, flag).CombinedOutput()
	if err != nil {
		return withWhatItSaid(err, out)
	}
	return nil
}

// Where the window library writes the companion's start at login, named
// after the companion. On macOS an app registers with the system
// instead, and the launch agent is what a bare program left; inside the
// app, before macOS 13, it is named after the app.
const (
	companionLaunchAgent  = "wails.autostart.agnostic-ai-companion.plist"
	companionDesktopEntry = "agnostic-ai-companion.desktop"
	companionRunValue     = `agnostic-ai-companion value under HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
)

// companionLoginEntries are the files that start the companion at
// login, given the home and configuration folders; none on a system
// that keeps it elsewhere than in a file. The companion removes the
// entry that names its own path. This reaches one left by a program
// that is gone, moved into the app, or was built somewhere else.
func companionLoginEntries(goos, home, config string) []string {
	switch goos {
	case "darwin":
		agents := filepath.Join(home, "Library", "LaunchAgents")
		return []string{filepath.Join(agents, companionLaunchAgent), filepath.Join(agents, companionBundleID+".plist")}
	case "linux":
		return []string{filepath.Join(config, "autostart", companionDesktopEntry)}
	default:
		return nil
	}
}

func systemCompanionLoginEntries(goos string) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("locate the companion's start at login: %w", err)
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("locate the companion's start at login: %w", err)
	}
	return companionLoginEntries(goos, home, config), nil
}

// endCompanion asks the running companion to quit and waits until it is
// gone. It reports whether one ran.
func endCompanion(ctx context.Context, tools companionTools, installed string) (bool, error) {
	ran := false
	for try := 0; ; try++ {
		running, err := tools.quit(ctx, installed)
		if err != nil {
			return ran, fmt.Errorf("ask the companion to stop: %w", err)
		}
		if !running {
			return ran, nil
		}
		ran = true
		if try == companionStopTries {
			return true, fmt.Errorf("the companion was asked to stop and still runs. End the %s process yourself", companionName)
		}
		tools.pause(companionStopPause)
	}
}

// companionInstalled reports whether dir holds a built companion, and
// where. On macOS that is the app, or the bare program an install
// before the app left.
func companionInstalled(dir, goos string) (string, bool, error) {
	installed := companionFile(dir, goos)
	found, err := companionExists(installed)
	if err != nil || found || goos != "darwin" {
		return installed, found, err
	}
	bare := bareCompanionFile(dir)
	if found, err := companionExists(bare); err != nil || found {
		return bare, found, err
	}
	return installed, false, nil
}

func companionExists(path string) (bool, error) {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("the companion: %w", err)
	}
	return true, nil
}

func stopCompanionCommand(ctx context.Context, out io.Writer, tools companionTools, dir, goos string) error {
	installed, found, err := companionInstalled(dir, goos)
	if err != nil {
		return err
	}
	if !found {
		_, _ = fmt.Fprintln(out, "The companion is not installed.")
		return nil
	}
	ran, err := endCompanion(ctx, tools, installed)
	if err != nil {
		return err
	}
	if !ran {
		_, _ = fmt.Fprintln(out, "No companion is running.")
		return nil
	}
	_, _ = fmt.Fprintln(out, "The companion stopped. Start it again with: agnostic-ai companion")
	return nil
}

// uninstallCompanion ends the companion, takes it out of what starts at
// login, and deletes the program and its settings. The login entry goes
// first, while the program that knows how to remove it is still there;
// loginEntries are the files that hold it, removed too when they are left.
func uninstallCompanion(ctx context.Context, out io.Writer, tools companionTools, dir, goos string, loginEntries []string) error {
	installed, found, err := companionInstalled(dir, goos)
	if err != nil {
		return err
	}
	// A companion that cannot run, such as one built for another
	// processor, must not keep its own files: each failure here is only
	// a warning.
	loginOff := false
	if found {
		ran, err := endCompanion(ctx, tools, installed)
		switch {
		case err != nil:
			_, _ = fmt.Fprintf(out, "! Could not stop the companion (%v). If it still runs, end the %s process.\n", err, companionName)
		case ran:
			_, _ = fmt.Fprintln(out, "✓ Stopped the companion")
		}
		if err := tools.loginOff(ctx, installed); err != nil {
			_, _ = fmt.Fprintf(out, "! Could not turn off start at login (%v). Remove %s from the items that open at login.\n", err, companionAppName)
		} else {
			loginOff = true
		}
	}
	left, err := removeLoginEntries(loginEntries)
	if err != nil {
		return fmt.Errorf("turn off start at login: %w", err)
	}
	switch {
	case loginOff || left:
		_, _ = fmt.Fprintln(out, "✓ Start at login is off")
	case goos == "windows":
		_, _ = fmt.Fprintf(out, "! If the companion started at login, delete the %s.\n", companionRunValue)
	}
	folder := companionFolder(dir)
	remove := func() error { return os.RemoveAll(folder) }
	if found {
		err = whileHeld(tools, remove)
	} else {
		err = remove()
	}
	if err != nil {
		return fmt.Errorf("remove the companion: %w", err)
	}
	for _, name := range []string{companionCommandFile, companionRevisionFile, companionSettingsFile} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove the companion: %w", err)
		}
	}
	if err := removeBuildFolders(dir); err != nil {
		return fmt.Errorf("remove the companion: %w", err)
	}
	if !found {
		_, _ = fmt.Fprintln(out, "The companion is not installed.")
		return nil
	}
	_, _ = fmt.Fprintf(out, "✓ Removed %s\n", folder)
	_, _ = fmt.Fprintln(out, "Your read and archive marks stay: agnostic-ai serve uses them too.")
	return nil
}

// removeBuildFolders deletes the build folders an interrupted install left.
func removeBuildFolders(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), companionBuildPrefix) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// macOSTooOld reports whether a macOS version, such as "12.7.4", is older
// than the companion supports. A version it cannot read is not.
func macOSTooOld(version string) bool {
	major, _, _ := strings.Cut(version, ".")
	n, err := strconv.Atoi(major)
	return err == nil && n < companionMinMacOS
}

// companionDir is where the built companion and its notes live: the
// user's configuration folder, beside the conversation marks.
func companionDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate the companion: %w", err)
	}
	return filepath.Join(dir, "agnostic-ai"), nil
}

// companionFolder holds everything an install writes besides its notes.
func companionFolder(dir string) string {
	return filepath.Join(dir, "companion")
}

func companionProgramName(goos string) string {
	if goos == "windows" {
		return companionName + ".exe"
	}
	return companionName
}

func companionFile(dir, goos string) string {
	if goos == "darwin" {
		return filepath.Join(companionBundle(dir), "Contents", "MacOS", companionName)
	}
	return filepath.Join(companionFolder(dir), companionProgramName(goos))
}

// buildModuleVersion is the version Go stamped on this agnostic-ai: the
// tag for a release build, a pseudo-version for any other commit.
func buildModuleVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return info.Main.Version
}

// buildRevision is the commit this agnostic-ai was built from; empty
// when the build does not record one.
func buildRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return setting.Value
		}
	}
	return ""
}

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
	if err := removeBuildFolders(dir); err != nil {
		return fmt.Errorf("install the companion: %w", err)
	}
	// The program is written in a folder of its own and moved into place
	// at the end, so a failure leaves the installed one alone.
	if err := os.MkdirAll(companionFolder(dir), 0o700); err != nil {
		return fmt.Errorf("install the companion: %w", err)
	}
	staging, err := os.MkdirTemp(dir, companionBuildPrefix+"*")
	if err != nil {
		return fmt.Errorf("install the companion: %w", err)
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

	built := ""
	if tools.download != nil && build.downloads() {
		_, _ = fmt.Fprintf(out, "Downloading the companion for agnostic-ai %s.\n", build.version)
		built, err = tools.download(out, build.version, build.into)
		switch {
		case errors.Is(err, errNoPrebuiltCompanion):
			_, _ = fmt.Fprintln(out, "! This release ships no built companion for this Mac, so it is built here.")
		case errors.Is(err, errProvenanceFailed) && os.Getenv(verifyAttestationEnv) != "1":
			return fmt.Errorf("download the companion: %w. If gh failed rather than the archive, %s=0 installs it on the checksum alone", err, verifyAttestationEnv)
		case err != nil:
			// A download that does not verify is never replaced by a build.
			return fmt.Errorf("download the companion: %w", err)
		default:
			_, _ = fmt.Fprintln(out, "✓ Downloaded and verified the companion")
		}
	}
	if built == "" {
		if built, err = buildCompanion(ctx, out, live, tools, build); err != nil {
			return err
		}
	}
	target := companionFile(dir, build.goos)
	place := func() error { return tools.rename(built, target) }
	if build.goos == "darwin" {
		app := filepath.Join(staging, companionAppName+".app")
		if err := assembleCompanionApp(ctx, tools, built, app, build.version); err != nil {
			return err
		}
		target = companionBundle(dir)
		place = func() error { return replaceCompanionApp(tools.rename, app, target, staging) }
	}
	// An interrupt stops here, before anything installed changes.
	if err := ctx.Err(); err != nil {
		return err
	}
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

	// Windows does not replace the file of a program that runs.
	ran, err := endCompanion(ctx, tools, installed)
	alone := err == nil
	switch {
	case err != nil && ran:
		// It still runs. A start now would only open its list.
		return err
	case err != nil:
		_, _ = fmt.Fprintf(out, "! Could not tell whether a companion runs (%v). If one does, end it and start it again.\n", err)
	case ran:
		_, _ = fmt.Fprintln(out, "✓ Stopped the running companion")
	}
	err = whileHeld(tools, place)
	var aside companionKeptAside
	if errors.As(err, &aside) {
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
		return err
	}
	// A companion that ran is started again whatever happened since: the
	// one that is installed now, old or new.
	if startErr := startInstalledCompanion(tools, installed); startErr != nil {
		return errors.Join(err, startErr)
	}
	if err == nil {
		_, _ = fmt.Fprintln(out, "✓ Started it again")
	}
	return err
}

// buildCompanion builds the companion into build.into and returns the program.
func buildCompanion(ctx context.Context, out io.Writer, live bool, tools companionTools, build companionBuild) (string, error) {
	_, _ = fmt.Fprintf(out, "Building the companion for agnostic-ai %s on this machine.\n", build.version)
	for _, need := range companionNeeds[build.goos] {
		if !tools.succeeds(ctx, need.check[0], need.check[1:]...) {
			return "", fmt.Errorf("the companion needs %s to build. %s, then run this again", need.what, need.fix)
		}
		_, _ = fmt.Fprintf(out, "✓ %s found\n", need.what)
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

func runCompanion(ctx context.Context, out io.Writer, tools companionTools, dir, goos, version, revision string) error {
	installed, found, err := companionInstalled(dir, goos)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("the companion is not installed yet. Install it with: agnostic-ai companion install")
	}
	if err := recordCompanionCommand(dir); err != nil {
		return err
	}
	if err := startInstalledCompanion(tools, installed); err != nil {
		return err
	}
	builtFor := tools.builtFor(ctx, installed)
	if builtFor == "" {
		return fmt.Errorf("the companion at %s could not run. Install it again with: agnostic-ai companion install", installed)
	}
	_, _ = fmt.Fprintln(out, "The companion is on your screen. Its ⋯ menu hides it or quits it, and its shortcut brings a hidden one back. `agnostic-ai companion stop` ends it.")
	if builtFor != version {
		_, _ = fmt.Fprintf(out, "! It was built for agnostic-ai %s and this is %s. Install it again with: agnostic-ai companion install\n", builtFor, version)
	} else if recorded := installedRevision(dir); revision != "" && recorded != "" && recorded != revision {
		_, _ = fmt.Fprintf(out, "! It was installed by agnostic-ai %s from commit %s, and this one is from commit %s. Install it again with: agnostic-ai companion install\n", version, shortRevision(recorded), shortRevision(revision))
	}
	return nil
}

// installedRevision is the commit recorded by the agnostic-ai that
// installed the companion; empty when none was recorded.
func installedRevision(dir string) string {
	raw, err := os.ReadFile(filepath.Join(dir, companionRevisionFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func shortRevision(revision string) string {
	if len(revision) > 12 {
		return revision[:12]
	}
	return revision
}

func newCompanionCmd() *cobra.Command {
	return newCompanionCmdWith(systemCompanionTools())
}

func newCompanionCmdWith(tools companionTools) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "companion",
		Short: "Show the floating list of AI conversations that need you.",
		Long: "Starts the companion: a small circle that floats over every app and counts the " +
			"Claude Code, Codex and Cursor conversations that wait for you, with a searchable " +
			"list that opens each one where it lives. It asks a server of its own, " +
			"`agnostic-ai serve --global`, which it starts when it needs a list and stops when " +
			"it quits. Install it first with " +
			"`agnostic-ai companion install`. Starting it again brings back a hidden companion. " +
			"`agnostic-ai companion stop` ends it and `agnostic-ai companion uninstall` removes it.",
		Example: `  # Install the companion on this machine, once and after each external upgrade
  agnostic-ai companion install

  # Start it
  agnostic-ai companion

  # End it
  agnostic-ai companion stop

  # Remove it from this machine
  agnostic-ai companion uninstall`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := companionDir()
			if err != nil {
				return err
			}
			return runCompanion(cmd.Context(), cmd.OutOrStdout(), tools, dir, runtime.GOOS, cmd.Root().Version, buildRevision())
		},
	}
	var source string
	install := &cobra.Command{
		Use:   "install",
		Short: "Put the companion on this machine.",
		Long: "On macOS, downloads the companion this release built, checks it against the " +
			"release's checksums and build provenance, and installs it in your configuration " +
			"folder; a download that does not verify stops the install. A release without a built " +
			"companion, an agnostic-ai built from any commit but a release tag, --source, and other systems build it from " +
			"the same commit as this agnostic-ai instead. The build " +
			"needs Go, and on macOS the Xcode Command Line Tools, on Linux a C compiler with the " +
			"GTK 4, WebKitGTK 6.0 and X11 development packages; it names what is missing. On " +
			"macOS it installs an app with the agnostic-ai icon, signed on this machine, and " +
			"replaces a bare program an earlier release installed. `agnostic-ai upgrade` updates " +
			"an installed companion; run this after external upgrades or to retry a failed update. " +
			"A companion that runs is stopped for the install and " +
			"started again after it.",
		Example: `  agnostic-ai companion install

  # Build from a checkout instead
  agnostic-ai companion install --source ~/code/agnostic-ai/companion`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := companionDir()
			if err != nil {
				return err
			}
			build := companionBuild{goos: runtime.GOOS, version: cmd.Root().Version, revision: buildRevision(), module: buildModuleVersion()}
			if source != "" {
				build.source, err = filepath.Abs(source)
				if err != nil {
					return fmt.Errorf("--source: %w", err)
				}
			}
			loginEntries, err := systemCompanionLoginEntries(runtime.GOOS)
			if err != nil {
				return err
			}
			// Ctrl-C ends the install through its context, so its build
			// folder is removed. A second one ends it at once.
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			context.AfterFunc(ctx, stop)
			out := cmd.OutOrStdout()
			file, isFile := out.(*os.File)
			return installCompanion(ctx, out, isFile && term.IsTerminal(file.Fd()), tools, dir, build, loginEntries)
		},
	}
	install.Flags().StringVar(&source, "source", "", "Build from this folder, the companion folder of an agnostic-ai checkout, instead of fetching the source.")
	stop := &cobra.Command{
		Use:   "stop",
		Short: "End the running companion.",
		Long: "Ends the companion and the server it started. Hiding the companion from its menu " +
			"leaves it running, waiting for its shortcut; this ends it. Start at login stays as " +
			"you set it, so the companion starts again at the next login when that is on.",
		Example: `  agnostic-ai companion stop`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := companionDir()
			if err != nil {
				return err
			}
			return stopCompanionCommand(cmd.Context(), cmd.OutOrStdout(), tools, dir, runtime.GOOS)
		},
	}
	uninstall := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the companion from this machine.",
		Long: "Ends the companion, turns off start at login, and deletes the built program, its " +
			"place and its shortcut. The read and archive marks stay, because " +
			"`agnostic-ai serve` uses them too. `agnostic-ai companion install` builds it again.",
		Example: `  agnostic-ai companion uninstall`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := companionDir()
			if err != nil {
				return err
			}
			loginEntries, err := systemCompanionLoginEntries(runtime.GOOS)
			if err != nil {
				return err
			}
			return uninstallCompanion(cmd.Context(), cmd.OutOrStdout(), tools, dir, runtime.GOOS, loginEntries)
		},
	}
	cmd.AddCommand(install, stop, uninstall)
	return cmd
}
