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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/config"
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
		addr, dir, uiDir, switchBack       string
		open, global, noInstalled, apiOnly bool
		allowedOrigins                     []string
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Open the local web interface and its JSON API.",
		Long: "Serves the bundled web interface and its API under /api. Use --ui for a " +
			"development client or --api-only to serve just the API. Specs load on every request. " +
			"Project built-in changes preview and apply source and generated files, with undo " +
			"for the last change in this server session. Global built-ins and settings edits save source choices " +
			" without syncing generated files. Stops on Ctrl-C.",
		Example: `  # Open the bundled interface
  agnostic-ai serve --open

  # API only, on the default port
  agnostic-ai serve --api-only

  # Override the bundled interface during development
  agnostic-ai serve --ui ui/dist --open

  # The user-level configuration
  agnostic-ai serve --global`,
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
			if global && cmd.Flags().Changed("dir") {
				return errs.Coded(errs.CodeFlagConflict, "--global and --dir cannot be combined")
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
			src, err := openAuthoritySource(dir, global)
			if err != nil {
				return err
			}
			defer src.close()
			peer := switchPeerFor(".", global)
			opts := authority.HandlerOptions{Root: src.root, UIDir: uiDir, ConfigFiles: src.configFiles, Global: global, AllowedOrigins: allowedOrigins}
			if global && peer != nil {
				opts.ProjectRoot = peer.root
			}
			if !apiOnly && uiDir == "" {
				opts.UIFiles = webui.Files()
			}
			if !global && !noInstalled {
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
			if open {
				if err := browser(url); err != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "! could not open a browser: %v\n", err)
				}
			}
			if peer != nil {
				peer.back = ln.Addr().String()
			}
			switches := newSwitchHandler(peer, viewPortCandidates(), probeViewServer, peerStarter)
			switches.back = switchBack
			defer switches.close()
			return serveUntilInterrupted(cmd.Context(), ln, newServeMuxWithSwitch(src.load, opts, !noInstalled, switches))
		},
	}
	cmd.Flags().StringSliceVar(&allowedOrigins, "allow-origin", nil, "Additional exact browser origin allowed to call the API.")
	cmd.Flags().StringVar(&addr, "addr", defaultServeAddr, "Address to listen on.")
	cmd.Flags().StringVar(&dir, "dir", ".", "Project directory to read.")
	cmd.Flags().StringVar(&uiDir, "ui", "", "Override the bundled web interface with this directory.")
	cmd.Flags().BoolVar(&apiOnly, "api-only", false, "Serve the JSON API without the web interface.")
	cmd.Flags().BoolVar(&open, "open", false, "Open the served URL in a browser.")
	cmd.Flags().BoolVar(&global, "global", false, "Serve the user-level configuration sync --global uses, in $AGNOSTIC_AI_HOME (default ~/.agnostic-ai).")
	cmd.Flags().BoolVar(&noInstalled, "no-installed", false, "Skip comparing the generated files with disk.")
	cmd.Flags().StringVar(&switchBack, "switch-back", "", "Loopback address of the server that started this one; its way back tries it first.")
	_ = cmd.Flags().MarkHidden("switch-back")
	return cmd
}

// newServeMux routes the API and the client. More specific patterns win
// over "/", so a view mounted here bypasses the authority handler.
//
// The overview runs the read-only checks (lint, drift, doctor) in the
// working directory, which serve has changed into for the project. For
// --global it runs the global lint and compares the files sync --global
// would write with disk, from any directory.
func newServeMux(load authority.Loader, opts authority.HandlerOptions) http.Handler {
	return newServeMuxWith(load, opts, true)
}

// newServeMuxWith is newServeMux that leaves the generated files
// uncompared in the overview when compareInstalled is false.
func newServeMuxWith(load authority.Loader, opts authority.HandlerOptions, compareInstalled bool) http.Handler {
	return newServeMuxWithSwitch(load, opts, compareInstalled, nil)
}

// newServeMuxWithSwitch also mounts the switch to the other
// configuration when switches is set.
func newServeMuxWithSwitch(load authority.Loader, opts authority.HandlerOptions, compareInstalled bool, switches *switchHandler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", authority.NewHandler(load, opts))
	views := newOverviewHandler(".", compareInstalled)
	if opts.Global {
		views = newGlobalOverviewHandler(compareInstalled)
	}
	mux.Handle(overview.OverviewPath, views)
	edits := newEditHandler(".", false)
	if opts.Global {
		edits = newEditHandler(opts.Root, true)
	}
	mux.Handle(editBuiltinPath, edits)
	mux.Handle(editValuePath, edits)
	mux.Handle(editFieldPath, edits)
	changes := newBuiltinChangeHandler(opts.Root, opts.Global)
	mux.Handle(builtinStatusPath, changes)
	mux.Handle(builtinStatusPath+"/", changes)
	mux.Handle(contextViewPath, newContextHandler(".", opts.Global))
	if switches != nil {
		mux.Handle(switchPath, switches)
	}
	return authority.CORS(bufferAPIBody(serializeAPI(mux)), opts.AllowedOrigins...)
}

// serializeAPI runs one API request at a time. Rendering adapters in
// capture mode swaps package-level warning state, so two views rendering
// at once would interleave it. Health and the switch render nothing, and
// the switch may wait seconds for a server to start.
func serializeAPI(next http.Handler) http.Handler {
	var mu sync.Mutex
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != authority.HealthPath && r.URL.Path != switchPath {
			mu.Lock()
			defer mu.Unlock()
		}
		next.ServeHTTP(w, r)
	})
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
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	fresh := trackFreshConns(srv)
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
	srv.SetKeepAlivesEnabled(false)
	fresh.close()
	if err := srv.Shutdown(shutdown); err != nil {
		return fmt.Errorf("stop the server: %w", err)
	}
	return nil
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
// A connection whose request was just read stays fresh until the state
// hook runs, so one is closed only once it has stayed fresh for
// freshConnGrace.
type freshConns struct {
	mu      sync.Mutex
	conns   map[net.Conn]time.Time
	closing bool
}

const freshConnGrace = 100 * time.Millisecond

func trackFreshConns(srv *http.Server) *freshConns {
	f := &freshConns{conns: map[net.Conn]time.Time{}}
	srv.ConnState = func(c net.Conn, state http.ConnState) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if state != http.StateNew {
			delete(f.conns, c)
			return
		}
		f.conns[c] = time.Now()
		if f.closing {
			f.closeLater(c, freshConnGrace)
		}
	}
	return f
}

// close closes each fresh connection once it has been fresh for
// freshConnGrace, including ones accepted after close.
func (f *freshConns) close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closing = true
	for c, opened := range f.conns {
		f.closeLater(c, freshConnGrace-time.Since(opened))
	}
}

func (f *freshConns) closeLater(c net.Conn, wait time.Duration) {
	time.AfterFunc(max(wait, 0), func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		if _, fresh := f.conns[c]; fresh {
			_ = c.Close()
		}
	})
}
