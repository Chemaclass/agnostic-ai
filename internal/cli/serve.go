package cli

import (
	"bytes"
	"context"
	"encoding/json"
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
)

// defaultServeAddr keeps the server on the loopback interface.
const defaultServeAddr = "127.0.0.1:4310"

func newServeCmd() *cobra.Command {
	var (
		addr, dir, uiDir          string
		allowOrigins              []string
		open, global, noInstalled bool
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the Authority view and its JSON API on a local port.",
		Long: "Serves the Authority view API under /api, and the built client from --ui when " +
			"given. Specs load on every request, so an edited spec shows on the next refresh. " +
			"Read-only: the API proposes changes but never writes them. Stops on Ctrl-C.",
		Example: `  # API only, on the default port
  agnostic-ai serve

  # With the built client, opened in a browser
  agnostic-ai serve --ui ui/dist --open

  # The user-level configuration
  agnostic-ai serve --global`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if global && cmd.Flags().Changed("dir") {
				return errs.Coded(errs.CodeFlagConflict, "--global and --dir cannot be combined")
			}
			if err := requireLoopbackAddr(addr); err != nil {
				return err
			}
			origins := make([]string, 0, len(allowOrigins))
			for _, raw := range allowOrigins {
				origin, err := authority.ParseOrigin(raw)
				if err != nil {
					return errs.Coded(errs.CodeFlagConflict, "--allow-origin: %v", err)
				}
				origins = append(origins, origin)
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
			opts := authority.HandlerOptions{Root: src.root, UIDir: uiDir, ConfigFiles: src.configFiles, Global: global}
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
			_, _ = fmt.Fprintf(out, "Serving the Authority view at %s\n", url)
			_, _ = fmt.Fprintf(out, "API: %sapi/views/authority (schema at %sapi/views/authority/schema)\n", url, url)
			if uiDir == "" {
				_, _ = fmt.Fprintln(out, "No --ui directory given, so only the API is served.")
			}
			if len(origins) > 0 {
				_, _ = fmt.Fprintf(out, "Pages from %s may call the API.\n", strings.Join(origins, ", "))
			}
			_, _ = fmt.Fprintln(out, "Press Ctrl-C to stop.")
			if open {
				if err := openBrowser(url); err != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "! could not open a browser: %v\n", err)
				}
			}
			return serveUntilInterrupted(cmd.Context(), ln, requireLocalHost(newServeMux(src.load, opts, origins), ln.Addr()))
		},
	}
	cmd.Flags().StringVar(&addr, "addr", defaultServeAddr, "Loopback address to listen on, such as 127.0.0.1:4310.")
	cmd.Flags().StringArrayVar(&allowOrigins, "allow-origin", nil, "Origin, such as http://localhost:5173, whose pages may call the API from another port; repeat for more. The Vite dev server needs none, since it proxies /api.")
	cmd.Flags().StringVar(&dir, "dir", ".", "Project directory to read.")
	cmd.Flags().StringVar(&uiDir, "ui", "", "Directory of the built client to serve, such as ui/dist.")
	cmd.Flags().BoolVar(&open, "open", false, "Open the served URL in a browser.")
	cmd.Flags().BoolVar(&global, "global", false, "Serve the user-level configuration sync --global uses, in $AGNOSTIC_AI_HOME (default ~/.agnostic-ai).")
	cmd.Flags().BoolVar(&noInstalled, "no-installed", false, "Skip comparing the generated files with disk.")
	return cmd
}

// newServeMux routes the API and the client. More specific patterns win
// over "/", so a view mounted here bypasses the authority handler.
//
// The overview and feed views run the read-only checks (lint, drift,
// doctor) in the working directory, which serve has changed into for
// the project. They are not available for --global, whose source
// directory is not a project.
//
// Browser pages from origins other than the server's own and allowed
// are refused before any API work runs.
func newServeMux(load authority.Loader, opts authority.HandlerOptions, allowed []string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", authority.NewHandler(load, opts))
	feed := newOverviewHandler(".")
	if opts.Global {
		feed = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"the overview and feed views read a project; they are not available with --global"}`))
		})
	}
	mux.Handle(overview.OverviewPath, feed)
	mux.Handle(overview.FeedPath, feed)
	return authority.OriginGuard(serializeAPI(mux), allowed)
}

// requireLoopbackAddr refuses a listen address other than localhost or a
// loopback IP: serve is for this machine only, and its API has no login.
func requireLoopbackAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return errs.Coded(errs.CodeFlagConflict, "--addr %s: %v", addr, err)
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return errs.Coded(errs.CodeFlagConflict, "--addr %s is not a loopback address; serve answers only this machine, so use 127.0.0.1, ::1, or localhost", addr)
	}
	return nil
}

// requireLocalHost answers 421 to a request whose Host is not a loopback
// name or the listen address. A browser page whose own name resolves to
// 127.0.0.1 (DNS rebinding) is same-origin to itself, so CORS never
// applies and only the Host header gives it away.
func requireLocalHost(next http.Handler, listen net.Addr) http.Handler {
	allowed := map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true}
	if tcp, ok := listen.(*net.TCPAddr); ok && tcp.IP != nil && !tcp.IP.IsUnspecified() {
		allowed[tcp.IP.String()] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if !allowed[strings.ToLower(strings.Trim(host, "[]"))] {
			writeServeError(w, http.StatusMisdirectedRequest, "this server answers only requests addressed to localhost")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// serializeAPI runs one API request at a time. Rendering adapters in
// capture mode swaps package-level warning state, so two views rendering
// at once would interleave it. A request body is read in full before the
// lock is taken, so a client that stalls mid-body holds up no one else.
func serializeAPI(next http.Handler) http.Handler {
	var mu sync.Mutex
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if r.Body != nil && r.Body != http.NoBody {
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, authority.MaxProposalBytes))
			if err != nil {
				writeServeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("the request body could not be read within %d bytes: %v", authority.MaxProposalBytes, err))
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		mu.Lock()
		defer mu.Unlock()
		next.ServeHTTP(w, r)
	})
}

// writeServeError answers with a JSON error, like the API's own errors.
func writeServeError(w http.ResponseWriter, status int, message string) {
	data, _ := json.Marshal(struct {
		Error string `json:"error"`
	}{Error: message})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(data, '\n'))
}

// serveUntilInterrupted serves on ln until the context ends or the
// process receives an interrupt, then shuts down gracefully.
func serveUntilInterrupted(ctx context.Context, ln net.Listener, handler http.Handler) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second}
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
