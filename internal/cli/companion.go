package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
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
	// companionBuildAge is how long an install may use its build folder.
	// The next install leaves a younger one alone.
	companionBuildAge = time.Hour
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
	download func(ctx context.Context, out io.Writer, version, into string) (string, error)
	// start runs the companion apart from this terminal.
	start func(path string, env []string) error
	// builtFor asks a companion which agnostic-ai release it was built
	// for. Its error wraps errCompanionCannotRun when the program cannot run.
	builtFor func(ctx context.Context, path string) (string, error)
	// quit asks the running companion to end and reports whether one ran.
	quit func(ctx context.Context, path string) (running bool, err error)
	// loginOff removes the companion from what starts at login.
	loginOff func(ctx context.Context, path string) error
	// loginOn adds the companion to what starts at login.
	loginOn func(ctx context.Context, path string) error
	// runValueOff deletes the Windows registry value that starts the
	// companion at login and reports whether one was there.
	runValueOff func() (bool, error)
	// noSessionBus reports whether this shell has no D-Bus session bus,
	// the only way to reach a running companion on Linux.
	noSessionBus func() bool
	sign         func(ctx context.Context, app string) error
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
		download: func(ctx context.Context, out io.Writer, version, into string) (string, error) {
			return downloadCompanion(ctx, out, &http.Client{Timeout: companionDownloadWait}, releasesBaseURL, version, machineArch(), into, ghProvenance)
		},
		start:       startCompanion,
		builtFor:    companionBuiltFor,
		quit:        quitCompanion,
		loginOff:    func(ctx context.Context, path string) error { return askCompanion(ctx, path, companionLoginOffFlag) },
		loginOn:     func(ctx context.Context, path string) error { return askCompanion(ctx, path, companionLoginOnFlag) },
		runValueOff: removeCompanionRunValue,
		noSessionBus: func() bool {
			return runtime.GOOS == "linux" && sessionBusMissing(os.Getenv, strconv.Itoa(os.Getuid()), func(path string) bool {
				_, err := os.Stat(path)
				return err == nil
			})
		},
		sign:   signCompanion,
		rename: os.Rename,
		macOS:  macOSVersion,
		pause:  time.Sleep,
	}
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

// oldWebviewFolder is where a Windows companion that did not name its
// web view data folder left it: beside dir, named after the program.
func oldWebviewFolder(dir string) string {
	return filepath.Join(filepath.Dir(dir), companionProgramName("windows"))
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
	// A first launch can be too slow to say its version; it still starts.
	builtFor, err := tools.builtFor(ctx, installed)
	if errors.Is(err, errCompanionCannotRun) {
		return fmt.Errorf("the companion at %s could not run (%w). Install it again with: agnostic-ai companion install", installed, err)
	}
	if err := startInstalledCompanion(tools, installed); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, "The companion is on your screen. Its ⋯ menu hides it or quits it, and its shortcut brings a hidden one back. `agnostic-ai companion stop` ends it.")
	if err != nil {
		return nil
	}
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
	cmd.AddCommand(companionAppCommands(tools)...)
	return cmd
}
