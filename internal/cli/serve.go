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
		addr, dir, uiDir                   string
		open, global, noInstalled, apiOnly bool
		allowedOrigins                     []string
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Open the local web interface and its JSON API.",
		Long: "Serves the bundled web interface and its API under /api. Use --ui for a " +
			"development client or --api-only to serve just the API. Specs load on every request. " +
			"Project built-in changes preview and apply source and generated files, with undo " +
			"for the last change in this server session. Global built-ins and boolean settings save source choices " +
			"without syncing generated files. A project server also serves the user-level configuration, " +
			"which the page switches to with scope=global. Stops on Ctrl-C.",
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
			if err := pinGlobalRoot(); err != nil {
				return err
			}
			src, err := openAuthoritySource(dir, global)
			if err != nil {
				return err
			}
			defer src.close()
			base := authority.HandlerOptions{UIDir: uiDir, AllowedOrigins: allowedOrigins}
			if !apiOnly && uiDir == "" {
				base.UIFiles = webui.Files()
			}
			scopes, err := serveScopes(src, base, !noInstalled)
			if err != nil {
				return err
			}
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("listen on %s: %w", addr, err)
			}
			url := "http://" + ln.Addr().String() + "/"
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "Serving agnostic-ai at %s\n", url)
			if len(scopes) > 1 {
				_, _ = fmt.Fprintf(out, "The page switches to the global configuration in %s with ?scope=global.\n", scopes[1].Root)
			}
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
			return serveUntilInterrupted(cmd.Context(), ln, newServeMux(scopes...))
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
	return cmd
}

// The configurations serve reads. Every /api request may name one with
// the scope query parameter; without it the server's default applies.
const (
	scopeProject = "project"
	scopeGlobal  = "global"
	scopesPath   = "/api/scopes"
)

// serveScope is one configuration serve reads: the project it started
// in, or the user-level configuration.
type serveScope struct {
	ID string `json:"id"`
	// Name is the project directory's name; empty for the global scope.
	Name string `json:"name,omitempty"`
	// Root is the absolute project directory or global source root.
	Root string `json:"root"`
	load authority.Loader
	opts authority.HandlerOptions
}

// serveScopes lists what serve reads, the default first. A project is
// served with the user-level configuration beside it, so the page can
// switch back to the global settings; the global configuration is
// served alone. base carries the options every scope shares.
func serveScopes(src *authoritySource, base authority.HandlerOptions, installed bool) ([]serveScope, error) {
	global, err := globalServeScope(base)
	if src.global {
		return []serveScope{global}, err
	}
	root, rootErr := os.Getwd()
	if rootErr != nil {
		return nil, fmt.Errorf("resolve working directory: %w", rootErr)
	}
	opts := base
	opts.Root, opts.ConfigFiles = src.root, src.configFiles
	if installed {
		opts.Installed = func(target string, cfg *config.Config, b spec.Bundle) (authority.InstalledState, error) {
			return installedState(target, cfg, b, src.root)
		}
	}
	scopes := []serveScope{{ID: scopeProject, Name: filepath.Base(root), Root: root, load: src.load, opts: opts}}
	if err == nil {
		scopes = append(scopes, global)
	}
	return scopes, nil
}

// pinGlobalRoot makes a relative $AGNOSTIC_AI_HOME absolute. It names a
// path from where serve started, and the global scope reads it again on
// every request, after serve has changed into the project.
func pinGlobalRoot() error {
	source := os.Getenv(envUserGlobalRoot)
	if source == "" || filepath.IsAbs(source) {
		return nil
	}
	abs, err := filepath.Abs(source)
	if err != nil {
		return fmt.Errorf("resolve %s %s: %w", envUserGlobalRoot, source, err)
	}
	return os.Setenv(envUserGlobalRoot, abs)
}

// globalServeScope is the user-level configuration. Its handlers read
// absolute paths, so they work from the project serve changed into.
func globalServeScope(base authority.HandlerOptions) (serveScope, error) {
	src, err := openAuthoritySource("", true)
	if err != nil {
		return serveScope{}, err
	}
	root, err := filepath.Abs(src.root)
	if err != nil {
		return serveScope{}, fmt.Errorf("resolve %s: %w", src.root, err)
	}
	opts := base
	opts.Root, opts.ConfigFiles, opts.Global = src.root, src.configFiles, true
	return serveScope{ID: scopeGlobal, Root: root, load: src.load, opts: opts}, nil
}

// newServeMux serves each scope through its own handlers, so a Caveman
// undo never crosses scopes, and dispatches each /api request by its
// scope parameter. The first scope is the default and serves the
// client. One guard and one API lock cover every scope.
func newServeMux(scopes ...serveScope) http.Handler {
	def := scopes[0]
	handlers := make(map[string]http.Handler, len(scopes))
	for _, s := range scopes {
		handlers[s.ID] = newScopeHandler(s.load, s.opts)
	}
	listing := struct {
		Default string       `json:"default"`
		Scopes  []serveScope `json:"scopes"`
	}{Default: def.ID, Scopes: scopes}
	dispatch := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			handlers[def.ID].ServeHTTP(w, r)
			return
		}
		if r.URL.Path == scopesPath {
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", http.MethodGet)
				writeEditJSON(w, http.StatusMethodNotAllowed, editError{Error: "use GET"})
				return
			}
			writeEditJSON(w, http.StatusOK, listing)
			return
		}
		id := r.URL.Query().Get("scope")
		if id == "" {
			id = def.ID
		}
		h, ok := handlers[id]
		if !ok {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: scopeRefusal(id)})
			return
		}
		h.ServeHTTP(w, r)
	})
	return authority.CORS(bufferAPIBody(serializeAPI(dispatch)), def.opts.AllowedOrigins...)
}

// scopeRefusal explains a scope this server does not read.
func scopeRefusal(id string) string {
	if id == scopeProject {
		return "this server reads the global configuration only; start serve in a project to read one"
	}
	return fmt.Sprintf("unknown scope %q; use project or global", id)
}

// newScopeHandler routes the API and the client of one configuration:
// a project, or the user-level configuration with opts.Global set. More
// specific patterns win over "/", so a view mounted here bypasses the
// authority handler.
//
// The project overview runs the read-only checks (lint, drift, doctor)
// in the working directory, which serve has changed into. The global
// overview runs the global lint and compares the files sync --global
// would write with disk, from any directory.
func newScopeHandler(load authority.Loader, opts authority.HandlerOptions) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", authority.NewHandler(load, opts))
	views := newOverviewHandler(".")
	if opts.Global {
		views = newGlobalOverviewHandler()
	}
	mux.Handle(overview.OverviewPath, views)
	edits := newEditHandler(".", false)
	if opts.Global {
		edits = newEditHandler(opts.Root, true)
	}
	mux.Handle(editBuiltinPath, edits)
	mux.Handle(editValuePath, edits)
	changes := newBuiltinChangeHandler(opts.Root, opts.Global)
	mux.Handle(builtinStatusPath, changes)
	mux.Handle(builtinStatusPath+"/", changes)
	return mux
}

// serializeAPI runs one API request at a time. Rendering adapters in
// capture mode swaps package-level warning state, so two views rendering
// at once would interleave it.
func serializeAPI(next http.Handler) http.Handler {
	var mu sync.Mutex
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			mu.Lock()
			defer mu.Unlock()
		}
		next.ServeHTTP(w, r)
	})
}

// serveUntilInterrupted serves on ln until the context ends or the
// process receives an interrupt, then shuts down gracefully.
func serveUntilInterrupted(ctx context.Context, ln net.Listener, handler http.Handler) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
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
