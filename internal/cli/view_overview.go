package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/overview"
)

func newViewCmd() *cobra.Command {
	return newViewCmdWithBrowser(openBrowser)
}

func newViewCmdWithBrowser(browser func(string) error) *cobra.Command {
	return newViewCmdWithBrowserAndProbe(browser, probeViewServer)
}

type viewServer struct {
	OK     bool   `json:"ok"`
	Root   string `json:"root"`
	Global bool   `json:"global"`
	UI     bool   `json:"ui"`
}

const (
	viewProbeTimeout     = 400 * time.Millisecond
	viewBusyProbeTimeout = 5 * time.Second
)

// probeViewServer treats absent, unhealthy, and unrelated services alike.
// The listen error remains the source of truth when the port is occupied.
func probeViewServer(addr string, timeout time.Duration) *viewServer {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get("http://" + addr + authority.HealthPath)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var server viewServer
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 4096)).Decode(&server); err != nil || !server.OK || server.Root == "" {
		return nil
	}
	return &server
}

func newViewCmdWithBrowserAndProbe(browser func(string) error, probe func(string, time.Duration) *viewServer) *cobra.Command {
	return newViewCmdWithPorts(browser, probe, viewPortCandidates())
}

// viewPortScan bounds how far view looks past the default port for a
// running server or a free port.
const viewPortScan = 10

func viewPortCandidates() []string {
	host, port, _ := net.SplitHostPort(defaultServeAddr)
	first, _ := strconv.Atoi(port)
	candidates := make([]string, viewPortScan)
	for i := range candidates {
		candidates[i] = net.JoinHostPort(host, strconv.Itoa(first+i))
	}
	return candidates
}

// serveView blocks while the server runs; a listen error means the port
// was taken between the probe and the bind.
func serveView(cmd *cobra.Command, browser func(string) error, addr, root string, global bool) error {
	serve := newServeCmdWithBrowser(browser)
	serve.SetContext(cmd.Context())
	serve.SetOut(cmd.OutOrStdout())
	serve.SetErr(cmd.ErrOrStderr())
	serve.SilenceErrors = true
	serve.SilenceUsage = true
	args := []string{"--open", "--addr", addr}
	if global {
		args = append(args, "--global")
	} else {
		args = append(args, "--dir", root)
	}
	serve.SetArgs(args)
	return serve.Execute()
}

func newViewCmdWithPorts(browser func(string) error, probe func(string, time.Duration) *viewServer, ports []string) *cobra.Command {
	var dir, addr string
	var asJSON, global bool
	cmd := &cobra.Command{
		Use:     "view",
		Aliases: []string{"ui"},
		Short:   "Open your configuration in the local web interface (alias: ui).",
		Long: "Opens the bundled web interface for the project in the current directory " +
			"or a parent directory. Outside a project, reads the user-level configuration. " +
			"Use --json to print the overview/v1 document without starting a server. " +
			"When the default port serves another project or scope, opens on the next free " +
			"loopback port up to 127.0.0.1:4319 and prints it; a later run for the same " +
			"configuration reuses that server. The interface reloads configuration on every " +
			"request. Stops on Ctrl-C.",
		Example: `  # Open the project, or user configuration outside a project
  agnostic-ai view

  # Print the overview document of another project
  agnostic-ai view --dir ../service --json

  # Open the user-level configuration
  agnostic-ai view --global`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if global && cmd.Flags().Changed("dir") {
				return errs.Coded(errs.CodeFlagConflict, "--global and --dir cannot be combined")
			}
			root, useGlobal, err := resolveViewSource(dir, global, cmd.Flags().Changed("dir"))
			if err != nil {
				return err
			}
			if asJSON {
				in, err := loadOverviewInputs(root, useGlobal)
				if err != nil {
					return err
				}
				return writeIndentedJSON(cmd, overview.Build(in))
			}
			src, err := openAuthoritySource(root, useGlobal)
			if err != nil {
				return err
			}
			_, _, err = src.load()
			src.close()
			if err != nil {
				return err
			}
			if err := validateServeAddr(addr); err != nil {
				return err
			}
			wantRoot := root
			if useGlobal {
				wantRoot, err = globalSourceRoot()
				if err != nil {
					return err
				}
			}
			candidates := ports
			explicit := cmd.Flags().Changed("addr")
			if explicit {
				candidates = []string{addr}
			}
			mismatch := func(addr string, running *viewServer) error {
				if !sameProjectRoot(running.Root, wantRoot) || running.Global != useGlobal {
					return fmt.Errorf("%s serves %s (global: %t); use --addr for another project or scope", addr, running.Root, running.Global)
				}
				if !running.UI {
					return fmt.Errorf("%s serves %s without the web interface; stop that server or use --addr", addr, running.Root)
				}
				return nil
			}
			reuse := func(addr string) error {
				url := "http://" + addr + "/"
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Using agnostic-ai at %s\n", url)
				if err := browser(url); err != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "! could not open a browser: %v\n", err)
				}
				return nil
			}
			var free []string
			for _, candidate := range candidates {
				running := probe(candidate, viewProbeTimeout)
				if running == nil {
					free = append(free, candidate)
					continue
				}
				err := mismatch(candidate, running)
				if err == nil {
					return reuse(candidate)
				}
				if explicit {
					return err
				}
			}
			// A failed bind means something listens, possibly a server still
			// starting. Rechecks share one budget, so silent programs on
			// several ports cannot multiply the wait.
			budget := viewBusyProbeTimeout
			var bindErr error
			for _, candidate := range free {
				err := serveView(cmd, browser, candidate, root, useGlobal)
				var listenErr *net.OpError
				if !errors.As(err, &listenErr) || listenErr.Op != "listen" {
					return err
				}
				bindErr = err
				recheck := max(budget, viewProbeTimeout)
				running := probe(candidate, recheck)
				if running == nil {
					budget -= recheck
				}
				if running != nil {
					taken := mismatch(candidate, running)
					if taken == nil {
						return reuse(candidate)
					}
					if explicit {
						return taken
					}
				}
				if explicit {
					return err
				}
			}
			first, last := candidates[0], candidates[len(candidates)-1]
			if bindErr != nil {
				return fmt.Errorf("no port from %s to %s could be opened; use --addr to choose one: %w", first, last, bindErr)
			}
			return fmt.Errorf("every port from %s to %s serves another configuration; use --addr to choose one", first, last)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "Project directory to read.")
	cmd.Flags().StringVar(&addr, "addr", defaultServeAddr, "Loopback address to listen on; when set, view never moves to another port.")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the overview as JSON without opening the web interface.")
	cmd.Flags().BoolVar(&global, "global", false, "Read the user-level configuration in $AGNOSTIC_AI_HOME (default ~/.agnostic-ai).")
	return cmd
}

func sameProjectRoot(a, b string) bool {
	infoA, errA := os.Stat(a)
	infoB, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return canonicalDir(a) == canonicalDir(b)
	}
	return os.SameFile(infoA, infoB)
}

func resolveViewSource(dir string, global, explicitDir bool) (string, bool, error) {
	if global {
		return "", true, nil
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return "", false, fmt.Errorf("resolve project directory %s: %w", dir, err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", false, fmt.Errorf("project directory %s: %w", root, err)
	}
	if !info.IsDir() {
		return "", false, fmt.Errorf("project directory %s is not a directory", root)
	}
	if explicitDir {
		return root, false, nil
	}
	root = canonicalDir(root)
	var globalRoots []os.FileInfo
	if source, err := globalSourceRoot(); err == nil {
		for _, path := range globalConfigPaths(source) {
			if info, err := os.Stat(filepath.Dir(path)); err == nil {
				globalRoots = append(globalRoots, info)
			}
		}
	}
	for candidate := root; ; candidate = filepath.Dir(candidate) {
		for _, globalRoot := range globalRoots {
			if info, err := os.Stat(candidate); err == nil && os.SameFile(info, globalRoot) {
				return "", true, nil
			}
		}
		for _, name := range []string{config.ConfigFileName, config.LegacyConfigFileName, config.LocalOverrideFileName} {
			path := filepath.Join(candidate, name)
			if _, err := os.Lstat(path); err == nil {
				return candidate, false, nil
			} else if !os.IsNotExist(err) {
				return "", false, fmt.Errorf("inspect %s: %w", path, err)
			}
		}
		if filepath.Dir(candidate) == candidate {
			return "", true, nil
		}
	}
}

func loadOverviewInputs(dir string, global bool) (overview.Inputs, error) {
	if global {
		return globalOverviewInputs()
	}
	src, err := openAuthoritySource(dir, false)
	if err != nil {
		return overview.Inputs{}, err
	}
	defer src.close()
	return overviewInputs(".")
}
