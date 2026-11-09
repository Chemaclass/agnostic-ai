package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
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
	// start runs the companion apart from this terminal.
	start func(path string, env []string) error
	// builtFor asks a companion which agnostic-ai release it was built for.
	builtFor func(ctx context.Context, path string) string
}

func systemCompanionTools() companionTools {
	return companionTools{
		succeeds: func(ctx context.Context, name string, args ...string) bool {
			return exec.CommandContext(ctx, name, args...).Run() == nil
		},
		build:    runGoBuild,
		start:    startCompanion,
		builtFor: companionBuiltFor,
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

// companionDir is where the built companion and its notes live: the
// user's configuration folder, beside the conversation marks.
func companionDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate the companion: %w", err)
	}
	return filepath.Join(dir, "agnostic-ai"), nil
}

func companionFile(dir, goos string) string {
	name := companionName
	if goos == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, "companion", name)
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
		return append(append([]string{"build"}, flags...), "-o", companionFile(filepath.Dir(b.into), b.goos), "."), b.source
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

func installCompanion(ctx context.Context, out io.Writer, live bool, tools companionTools, dir string, build companionBuild) error {
	_, _ = fmt.Fprintf(out, "Building the companion for agnostic-ai %s on this machine.\n", build.version)
	needs, known := companionNeeds[build.goos]
	if !known {
		return fmt.Errorf("the companion does not build on %s", build.goos)
	}
	for _, need := range needs {
		if !tools.succeeds(ctx, need.check[0], need.check[1:]...) {
			return fmt.Errorf("the companion needs %s to build. %s, then run this again", need.what, need.fix)
		}
		_, _ = fmt.Fprintf(out, "✓ %s found\n", need.what)
	}
	if build.source == "" && build.revision == "" {
		return errors.New("this agnostic-ai does not record the commit it was built from, so it cannot fetch the companion's source; pass --source with the companion folder of an agnostic-ai checkout")
	}
	if build.source != "" {
		if _, err := os.Stat(filepath.Join(build.source, "go.mod")); err != nil {
			return fmt.Errorf("--source %s is not the companion folder of an agnostic-ai checkout: %w", build.source, err)
		}
	}

	// The program is built in a folder of its own and moved into place
	// at the end, so a build that fails leaves the installed one alone.
	if err := os.MkdirAll(filepath.Dir(companionFile(dir, build.goos)), 0o700); err != nil {
		return fmt.Errorf("install the companion: %w", err)
	}
	staging, err := os.MkdirTemp(dir, ".companion-build-*")
	if err != nil {
		return fmt.Errorf("install the companion: %w", err)
	}
	defer func() { _ = os.RemoveAll(staging) }()
	build.into = filepath.Join(staging, "companion")
	if err := os.MkdirAll(build.into, 0o700); err != nil {
		return fmt.Errorf("install the companion: %w", err)
	}

	_, _ = fmt.Fprintln(out, "→ Fetching and compiling. The first build takes a few minutes.")
	progress := &buildProgress{out: out, live: live}
	args, runIn := build.goArgs()
	buildErr := tools.build(ctx, runIn, build.goEnv(os.Environ()), progress.line, args...)
	progress.done()
	if buildErr != nil {
		return fmt.Errorf("the build failed (%w):\n%s", buildErr, strings.Join(progress.last, "\n"))
	}
	built, err := builtCompanion(build)
	if err != nil {
		return err
	}
	installed := companionFile(dir, build.goos)
	if err := os.Rename(built, installed); err != nil {
		return fmt.Errorf("install the companion (quit a running companion first): %w", err)
	}
	if err := recordCompanionCommand(dir); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "✓ Installed %s\n", installed)
	_, _ = fmt.Fprintln(out, "Start it with: agnostic-ai companion")
	return nil
}

// builtCompanion finds the program the build wrote. `go install` names
// it after the module's folder; `go build -o` uses the name it was given.
func builtCompanion(build companionBuild) (string, error) {
	candidates := []string{companionFile(filepath.Dir(build.into), build.goos), filepath.Join(build.into, "companion"), filepath.Join(build.into, "companion.exe")}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
	}
	return "", errors.New("the build finished without writing the companion")
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

func runCompanion(ctx context.Context, out io.Writer, tools companionTools, dir, goos, version string) error {
	installed := companionFile(dir, goos)
	if _, err := os.Stat(installed); errors.Is(err, fs.ErrNotExist) {
		return errors.New("the companion is not installed yet. Build it for this machine with: agnostic-ai companion install")
	} else if err != nil {
		return fmt.Errorf("the companion: %w", err)
	}
	if err := recordCompanionCommand(dir); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find the agnostic-ai binary: %w", err)
	}
	if err := tools.start(installed, append(os.Environ(), "AGNOSTIC_AI_BIN="+self)); err != nil {
		return fmt.Errorf("start the companion: %w", err)
	}
	_, _ = fmt.Fprintln(out, "The companion is on your screen. Its ⋯ menu hides it, and its shortcut brings it back.")
	if builtFor := tools.builtFor(ctx, installed); builtFor != version {
		_, _ = fmt.Fprintf(out, "! It was built for agnostic-ai %q and this is %s. Build it again with: agnostic-ai companion install\n", builtFor, version)
	}
	return nil
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
			"you hide it. The companion is built on your machine first, with " +
			"`agnostic-ai companion install`. Starting it again brings a hidden companion back.",
		Example: `  # Build the companion for this machine, once and after each upgrade
  agnostic-ai companion install

  # Start it
  agnostic-ai companion`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := companionDir()
			if err != nil {
				return err
			}
			return runCompanion(cmd.Context(), cmd.OutOrStdout(), tools, dir, runtime.GOOS, cmd.Root().Version)
		},
	}
	var source string
	install := &cobra.Command{
		Use:   "install",
		Short: "Build the companion on this machine.",
		Long: "Builds the companion from the same commit as this agnostic-ai and installs it in " +
			"your configuration folder. Nothing is downloaded but source code, through Go. It " +
			"needs Go, and on macOS the Xcode Command Line Tools, on Linux a C compiler with the " +
			"GTK 4, WebKitGTK 6.0 and X11 development packages; it names what is missing. Run it " +
			"again after each upgrade of agnostic-ai.",
		Example: `  agnostic-ai companion install

  # Build from a checkout instead
  agnostic-ai companion install --source ~/code/agnostic-ai/companion`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := companionDir()
			if err != nil {
				return err
			}
			build := companionBuild{goos: runtime.GOOS, version: cmd.Root().Version, revision: buildRevision()}
			if source != "" {
				build.source, err = filepath.Abs(source)
				if err != nil {
					return fmt.Errorf("--source: %w", err)
				}
			}
			out := cmd.OutOrStdout()
			file, isFile := out.(*os.File)
			return installCompanion(cmd.Context(), out, isFile && term.IsTerminal(file.Fd()), tools, dir, build)
		},
	}
	install.Flags().StringVar(&source, "source", "", "Build from this folder, the companion folder of an agnostic-ai checkout, instead of fetching the source.")
	cmd.AddCommand(install)
	return cmd
}
