package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/conversations"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	webui "github.com/chemaclass/agnostic-ai/ui"
)

// defaultServeAddr keeps the server on the loopback interface.
const defaultServeAddr = "127.0.0.1:4310"

func newServeCmd() *cobra.Command {
	return newServeCmdWithBrowser(openBrowser)
}

func newServeCmdWithBrowser(browser func(string) error) *cobra.Command {
	var (
		addr, dir, uiDir, switchBack           string
		open, userLevel, noInstalled, apiOnly  bool
		stopWithStdin, cloudSessions, noUnlock bool
		allowedOrigins                         []string
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Open the local web interface and its JSON API.",
		Long: "Serves the bundled web interface and its API under /api. Use --ui for a " +
			"development client or --api-only to serve just the API. Specs load on every request. " +
			"Project built-in changes preview and apply source and generated files, with undo " +
			"for the last change in this server session. User-level built-ins and settings edits save source choices " +
			" without syncing generated files. Stops on Ctrl-C.",
		Example: `  # Open the bundled interface
  agnostic-ai serve --open

  # API only, on the default port
  agnostic-ai serve --api-only

  # Override the bundled interface during development
  agnostic-ai serve --ui ui/dist --open

  # The user-level configuration
  agnostic-ai serve --scope user

  # Also list Claude Code cloud sessions and Codex cloud threads
  agnostic-ai serve --scope user --cloud-sessions`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if apiOnly && (cmd.Flags().Changed("ui") || open) {
				return errs.Coded(errs.CodeFlagConflict, "--api-only cannot be combined with --ui or --open")
			}
			if err := validateServeAddr(addr); err != nil {
				return err
			}
			if switchBack != "" {
				if err := validateServeAddr(switchBack); err != nil {
					return fmt.Errorf("--switch-back: %w", err)
				}
				if _, port, _ := net.SplitHostPort(switchBack); strings.TrimLeft(port, "0") == "" {
					return fmt.Errorf("--switch-back %s needs a port other than 0", switchBack)
				}
			}
			for i, origin := range allowedOrigins {
				parsed, err := authority.ParseOrigin(origin)
				if err != nil {
					return fmt.Errorf("--allow-origin: %w", err)
				}
				allowedOrigins[i] = parsed
			}
			if userLevel && cmd.Flags().Changed("dir") {
				return errs.Coded(errs.CodeFlagConflict, "--scope user and --dir cannot be combined")
			}
			// A project server lists the conversations started in the
			// project, and a cloud session names no folder.
			if cloudSessions && !userLevel {
				return errors.New("--cloud-sessions needs --scope user: cloud sessions belong to no project folder")
			}
			if uiDir != "" {
				abs, err := filepath.Abs(uiDir)
				if err != nil {
					return fmt.Errorf("resolve --ui %s: %w", uiDir, err)
				}
				if info, err := os.Stat(abs); err != nil || !info.IsDir() {
					return fmt.Errorf("--ui %s is not a directory", uiDir)
				}
				uiDir = abs
			}
			companion := takeCompanionSecret()
			stuckShellAfter := servedShellThreshold(cmd.ErrOrStderr())
			src, err := openAuthoritySource(dir, userLevel)
			if err != nil {
				return err
			}
			defer src.close()
			peer := switchPeerFor(".", userLevel)
			opts := authority.HandlerOptions{Root: src.root, UIDir: uiDir, ConfigFiles: src.configFiles, UserLevel: userLevel, Version: cmd.Root().Version, AllowedOrigins: allowedOrigins}
			if userLevel && peer != nil {
				opts.ProjectRoot = peer.root
			}
			opts.Requires = servedRequiresOf(src.root, userLevel)
			if !apiOnly && uiDir == "" {
				opts.UIFiles = webui.Files()
			}
			if !userLevel && !noInstalled {
				opts.Installed = func(target string, cfg *config.Config, b spec.Bundle) (authority.InstalledState, error) {
					return installedState(target, cfg, b, src.root)
				}
			}
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("listen on %s: %w", addr, err)
			}
			url := "http://" + ln.Addr().String() + "/"
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "Serving agnostic-ai at %s\n", url)
			_, _ = fmt.Fprintf(out, "API: %sapi/views/authority (schema at %sapi/views/authority/schema)\n", url, url)
			if apiOnly {
				_, _ = fmt.Fprintln(out, "API-only mode; no web interface is served.")
			}
			_, _ = fmt.Fprintln(out, "Press Ctrl-C to stop.")
			// A user-level page gets the key that lets it loosen approvals
			// through a file only this account can read. A later view run
			// asks for one, so a server that opened none still has a key.
			var session *webSession
			if userLevel && !apiOnly && unlockFilesPrivate {
				if session, err = newWebSession(ln.Addr().String()); err != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "! the page opens locked: %v\n", err)
				}
				defer session.close()
			}
			if open {
				target := url
				if session != nil && !noUnlock {
					if file, err := session.offer(); err == nil {
						target = file
					} else {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "! the page opens locked: %v\n", err)
					}
				}
				if err := browser(target); err != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "! could not open a browser: %v\n", err)
				}
			}
			if peer != nil {
				peer.back = ln.Addr().String()
			}
			switches := newSwitchHandler(peer, viewPortCandidates(), probeViewServer, peerStarter)
			switches.back = switchBack
			defer switches.close()
			ctx := cmd.Context()
			if stopWithStdin {
				ctx = endedWith(ctx, cmd.InOrStdin())
			}
			if stopWithStdin && userLevel && companion != "" {
				// Only a companion starts a user-level server this way, and
				// only one that passed its secret can answer a held request.
				if source, err := userSourceRoot(); err == nil {
					live, stopLive := context.WithCancel(ctx)
					wait := keepCompanionLive(live, source)
					defer wait()
					defer stopLive()
				}
			}
			return serveUntilInterrupted(ctx, ln, newServeMux(src.load, opts, serveMuxOptions{compareInstalled: !noInstalled, switches: switches, cloudSessions: cloudSessions, stuckShellAfter: stuckShellAfter, companion: companion, session: session}))
		},
	}
	cmd.Flags().StringSliceVar(&allowedOrigins, "allow-origin", nil, "Additional exact browser origin allowed to call the API.")
	cmd.Flags().StringVar(&addr, "addr", defaultServeAddr, "Address to listen on.")
	cmd.Flags().StringVar(&dir, "dir", ".", "Project directory to read.")
	cmd.Flags().StringVar(&uiDir, "ui", "", "Override the bundled web interface with this directory.")
	cmd.Flags().BoolVar(&apiOnly, "api-only", false, "Serve the JSON API without the web interface.")
	cmd.Flags().BoolVar(&open, "open", false, "Open the served URL in a browser.")
	addScopeFlag(cmd, &userLevel, "Serve the user-level configuration sync --scope user uses, in $AGNOSTIC_AI_HOME (default ~/.agnostic-ai).")
	cmd.Flags().BoolVar(&noInstalled, "no-installed", false, "Skip comparing the generated files with disk.")
	cmd.Flags().StringVar(&switchBack, "switch-back", "", "Loopback address of the server that started this one; its way back tries it first.")
	_ = cmd.Flags().MarkHidden("switch-back")
	cmd.Flags().BoolVar(&stopWithStdin, "stop-with-stdin", false, "Stop when standard input closes: a program that started this server ends it by closing the pipe, or by ending itself.")
	_ = cmd.Flags().MarkHidden("stop-with-stdin")
	cmd.Flags().BoolVar(&noUnlock, "no-unlock", false, "With --open, open the plain address, without the unlock file that lets the page loosen approvals.")
	_ = cmd.Flags().MarkHidden("no-unlock")
	cmd.Flags().BoolVar(&cloudSessions, "cloud-sessions", false, "Also list Claude Code cloud sessions and Codex cloud threads: reads each tool's own sign-in on this machine and asks its cloud at most once a minute. Needs --scope user.")
	return cmd
}

// serveMuxOptions are the parts of a served configuration that only
// serve itself turns on.
type serveMuxOptions struct {
	// compareInstalled compares the generated files with disk in the
	// overview.
	compareInstalled bool
	// switches mounts the switch to the other configuration.
	switches *switchHandler
	// cloudSessions also lists Claude Code cloud sessions and Codex cloud
	// threads. Without it nothing reads the sign-in or leaves this machine.
	cloudSessions   bool
	stuckShellAfter *time.Duration
	// companion is the secret of the companion that started the server.
	companion companionSecret
	// session is the web session key of a page this server opened.
	session *webSession
}

// newServeMux routes the API and the client. More specific patterns win
// over "/", so a view mounted here bypasses the authority handler.
//
// The overview runs the read-only checks (lint, drift, doctor) in the
// working directory, which serve has changed into for the project. For
// --scope user it runs the user lint and compares the files sync --scope user
// would write with disk, from any directory.
func newServeMux(load authority.Loader, opts authority.HandlerOptions, muxOpts serveMuxOptions) http.Handler {
	switches := muxOpts.switches
	mux := http.NewServeMux()
	conversationsHandler := newConversationsHandler(opts.Root, opts.UserLevel)
	conversationsHandler.stuckShellAfter = muxOpts.stuckShellAfter
	conversationsHandler.companion = muxOpts.companion
	opts.ScanningSince = conversationsHandler.scanner.ScanningSince
	mux.Handle("/", authority.NewHandler(load, opts))
	views := newOverviewHandler(".", muxOpts.compareInstalled)
	if opts.UserLevel {
		views = newUserOverviewHandler(muxOpts.compareInstalled)
	}
	mux.Handle(overview.OverviewPath, views)
	edits := newEditHandler(".", false)
	if opts.UserLevel {
		edits = newEditHandler(opts.Root, true)
	}
	mux.Handle(editBuiltinPath, edits)
	mux.Handle(editTargetsPath, edits)
	mux.Handle(editValuePath, edits)
	mux.Handle(editFieldPath, edits)
	mux.Handle(editPermissionsPath, edits)
	mux.Handle(editProtectedPath, edits)
	mux.Handle(editMCPPath, edits)
	mux.Handle(editHookPath, edits)
	changes := newBuiltinChangeHandler(opts.Root, opts.UserLevel)
	mux.Handle(builtinStatusPath, changes)
	mux.Handle(builtinStatusPath+"/", changes)
	mux.Handle(contextViewPath, newContextHandler(".", opts.UserLevel))
	if muxOpts.cloudSessions {
		conversationsHandler.scanner.Cloud = conversations.NewClaudeCloud()
		conversationsHandler.scanner.CodexCloud = conversations.NewCodexCloud()
	}
	for _, path := range conversationPaths {
		mux.Handle(path, conversationsHandler)
	}
	approvalsRoutes := approvalsHandler{reader: conversationsHandler.approvals, secret: muxOpts.companion}
	mux.Handle(approvalsViewPath, approvalsRoutes)
	mux.Handle(approvalsAskPath, approvalsRoutes)
	mux.Handle(approvalsDecidePath, approvalsRoutes)
	settings := newSettingsHandler(opts.UserLevel, conversationsHandler.approvals, func() string { return conversationsHandler.scanner.MarksPath }, muxOpts.companion, opts.Version)
	settings.session = muxOpts.session
	conversationsHandler.proven, conversationsHandler.canProve = settings.proven, settings.canProve
	if userEdits, ok := edits.(*editHandler); ok {
		userEdits.proven = settings.proven
	}
	sessions := sessionHandler{session: muxOpts.session}
	mux.Handle(sessionUnlockPath, sessions)
	mux.Handle(sessionOfferPath, sessions)
	mux.Handle(settingsViewPath, settings)
	mux.Handle(settingsWritePath, settings)
	mux.Handle(settingsSyncPath, settings)
	mux.Handle(sourceViewPath, newSourceHandler(opts.UserLevel))
	var peer *switchPeer
	if switches != nil {
		peer = switches.peer
	}
	moveRoot := "."
	if opts.UserLevel {
		moveRoot = opts.Root
	}
	mux.Handle(editSpecMovePath, newSpecMoveHandler(moveRoot, opts.UserLevel, peer))
	if switches != nil {
		mux.Handle(switchPath, switches)
		if switches.peer != nil && switches.peer.userLevel {
			mux.Handle(userCreatePath, newUserCreateHandler(switches.peer.root))
		}
	}
	guarded := authority.CORS(bufferAPIBody(serializeAPI(mux)), opts.AllowedOrigins...)
	if muxOpts.session == nil {
		return guarded
	}
	return unlockHop(guarded)
}

// serializeAPI runs one API request at a time. Rendering adapters in
// capture mode swaps package-level warning state, so two views rendering
// at once would interleave it. Health and the switch render nothing, and
// the switch may wait seconds for a server to start.
func serializeAPI(next http.Handler) http.Handler {
	var mu sync.Mutex
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Conversations read only the coding tools' own stores and lock
		// their own scanner, so a slow scan does not hold up the overview.
		// An answer to a held request renders nothing either, and must not
		// wait behind a sync started from the settings.
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != authority.HealthPath && r.URL.Path != switchPath &&
			r.URL.Path != approvalsDecidePath && !slices.Contains(conversationPaths, r.URL.Path) {
			mu.Lock()
			defer mu.Unlock()
		}
		next.ServeHTTP(w, r)
	})
}

// endedWith returns a context that also ends when in reaches its end:
// the program at the other side of the pipe closed it or is gone.
func endedWith(ctx context.Context, in io.Reader) context.Context {
	ctx, end := context.WithCancel(ctx)
	go func() {
		_, _ = io.Copy(io.Discard, in)
		end()
	}()
	return ctx
}

// serveSignals stops serve on hangup too, unless the hangup was ignored
// at start, as under nohup: naming it would handle it again.
func serveSignals(ignored func(os.Signal) bool) []os.Signal {
	signals := []os.Signal{os.Interrupt, syscall.SIGTERM}
	if !ignored(syscall.SIGHUP) {
		signals = append(signals, syscall.SIGHUP)
	}
	return signals
}

// serveUntilInterrupted serves on ln until the context ends or the
// process receives an interrupt, then shuts down gracefully.
func serveUntilInterrupted(ctx context.Context, ln net.Listener, handler http.Handler) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, stop := signal.NotifyContext(ctx, serveSignals(signal.Ignored)...)
	defer stop()
	srv := newServeServer(handler)
	srv.RegisterOnShutdown(trackFreshConns(srv).close)
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	select {
	case err := <-served:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil {
		return fmt.Errorf("stop the server: %w", err)
	}
	return nil
}

func newServeServer(handler http.Handler) *http.Server {
	return &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
}

// openBrowser opens url with the platform's default handler.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func validateServeAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("listen address %s: %w", addr, err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("listen address %s must use an explicit loopback IP", addr)
	}
	return nil
}

// Read bodies before serializing work that touches process-wide adapter state.
func bufferAPIBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Body != nil && r.Body != http.NoBody {
			data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
			if err != nil {
				http.Error(w, "request body is invalid or exceeds the limit", http.StatusBadRequest)
				return
			}
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(data))
		}
		next.ServeHTTP(w, r)
	})
}

// freshConns are connections that have not sent a request yet.
// Shutdown waits up to 5 seconds for them, and clients such as browsers
// and Go's transport open them ahead of a request that may never come.
// Closing them after shutdown begins drops no request: the server skips
// the handler for a request it reads once shutdown has begun.
type freshConns struct {
	mu      sync.Mutex
	conns   map[net.Conn]struct{}
	closing bool
}

func trackFreshConns(srv *http.Server) *freshConns {
	f := &freshConns{conns: map[net.Conn]struct{}{}}
	srv.ConnState = func(c net.Conn, state http.ConnState) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case state != http.StateNew:
			delete(f.conns, c)
		case f.closing:
			_ = c.Close()
		default:
			f.conns[c] = struct{}{}
		}
	}
	return f
}

func (f *freshConns) close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closing = true
	for c := range f.conns {
		_ = c.Close()
	}
}
