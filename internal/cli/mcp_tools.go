package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/mcpprobe"
	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// newMCPCmd groups the MCP server commands.
func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Work with the configured MCP servers.",
	}
	cmd.AddCommand(newMCPToolsCmd())
	return cmd
}

// newMCPToolsCmd is `mcp tools`: connect to each configured MCP server,
// list its tools, and cache the result for view and serve.
func newMCPToolsCmd() *cobra.Command {
	var dir string
	var global, asJSON bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "tools",
		Short: "List each MCP server's tools and cache them for the overview.",
		Long: "Connects to every configured MCP server, the way a coding tool does, and asks it " +
			"for its tools: stdio servers are started with their command, args, and env; http " +
			"servers are called with their headers and headersHelper. The result is cached in " +
			"the user cache directory, and view and serve show it without connecting. " +
			"This is the only command that connects to MCP servers. sse and ws servers are not probed.",
		Example: `  # The project in the working directory
  agnostic-ai mcp tools

  # The user-level configuration
  agnostic-ai mcp tools --global`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if global && cmd.Flags().Changed("dir") {
				return errs.Coded(errs.CodeFlagConflict, "--global and --dir cannot be combined")
			}
			root, entries, err := mcpToolsScope(dir, global)
			if err != nil {
				return err
			}
			cache := listMCPTools(cmd.Context(), root, entries, timeout, runningVersion)
			if err := mcpprobe.SaveCache(root, cache); err != nil {
				return fmt.Errorf("cache MCP tools: %w", err)
			}
			if asJSON {
				return writeIndentedJSON(cmd, cache)
			}
			printMCPTools(cmd, cache)
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "Project directory to read.")
	cmd.Flags().BoolVar(&global, "global", false, "Read the user-level configuration sync --global uses, in $AGNOSTIC_AI_HOME (default ~/.agnostic-ai).")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the cached result as JSON.")
	cmd.Flags().DurationVar(&timeout, "timeout", 20*time.Second, "How long to wait for each server.")
	return cmd
}

// mcpToolsScope loads the MCP specs of the project in dir, or of the
// user-level configuration, with the root the cache is keyed by: the
// same root the overview reads.
func mcpToolsScope(dir string, global bool) (string, []spec.Entry, error) {
	if global {
		scope, err := loadCheckScope(true)
		if err != nil {
			return "", nil, err
		}
		return scope.source, scope.bundle.MCPs, nil
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return "", nil, err
	}
	leave, err := overviewEnterDir(dir)
	if err != nil {
		return "", nil, err
	}
	defer leave()
	_, b, err := loadProject(".")
	if err != nil {
		return "", nil, err
	}
	return root, b.MCPs, nil
}

const mcpProbeWorkers = 4

// listMCPTools limits active connections and bounds each whole probe.
func listMCPTools(ctx context.Context, root string, entries []spec.Entry, timeout time.Duration, client string) mcpprobe.Cache {
	if ctx == nil {
		ctx = context.Background()
	}
	cache := mcpprobe.Cache{Version: mcpprobe.CacheVersion, Root: root, Servers: map[string]mcpprobe.ServerResult{}}
	var mu sync.Mutex
	var wg sync.WaitGroup
	jobs := make(chan spec.Entry)
	for range min(mcpProbeWorkers, len(entries)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for e := range jobs {
				res := mcpprobe.ServerResult{Spec: mcpprobe.SpecFingerprint(e.Meta), ListedAt: time.Now().UTC(), Tools: []mcpprobe.Tool{}}
				c, cancel := context.WithTimeout(ctx, timeout)
				server, err := mcpServerContext(c, e, root)
				if err == nil {
					res.Tools, err = mcpprobe.ListTools(c, server, client)
				}
				cancel()
				if err != nil {
					res.Error = preview.RedactText(err.Error(), server.SensitiveValues...)
					res.Tools = []mcpprobe.Tool{}
				}
				if res.Tools == nil {
					res.Tools = []mcpprobe.Tool{}
				}
				slices.SortFunc(res.Tools, func(a, b mcpprobe.Tool) int { return strings.Compare(a.Name, b.Name) })
				mu.Lock()
				cache.Servers[e.Name] = res
				mu.Unlock()
			}
		}()
	}
	for _, e := range entries {
		jobs <- e
	}
	close(jobs)
	wg.Wait()
	return cache
}

// mcpServer turns a spec into a server to probe, resolving ${NAME}
// references from the environment and running headersHelper.
func mcpServer(e spec.Entry, root string) (mcpprobe.Server, error) {
	return mcpServerContext(context.Background(), e, root)
}

func mcpServerContext(ctx context.Context, e spec.Entry, root string) (mcpprobe.Server, error) {
	if err := ctx.Err(); err != nil {
		return mcpprobe.Server{}, err
	}
	if disabled, _ := e.Meta["disabled"].(bool); disabled {
		return mcpprobe.Server{}, errors.New("disabled in its spec")
	}
	str := func(key string) string { s, _ := e.Meta[key].(string); return s }
	var problems []string
	s := mcpprobe.Server{Env: map[string]string{}, Headers: map[string]string{}}
	resolve := func(field, key, value string) string {
		if e.MarkedLiteral(field, key) {
			return value
		}
		out, err := resolveEnvRefs(value, root)
		if err != nil {
			problems = append(problems, err.Error())
		}
		for _, token := range spec.EnvRefTokens(value) {
			if preview.CredentialKey(token.Name) {
				resolved, _ := resolveEnvRefs(token.Text, root)
				s.SensitiveValues = append(s.SensitiveValues, resolved)
			}
		}
		return out
	}
	s.Type = str("type")
	if s.Type != "" && s.Type != "stdio" && s.Type != "http" {
		return s, fmt.Errorf("%w: %s", mcpprobe.ErrUnsupported, s.Type)
	}
	s.Command = resolve("command", "", str("command"))
	s.Cwd = resolve("cwd", "", str("cwd"))
	s.URL = resolve("url", "", str("url"))
	if s.Cwd == "" {
		s.Cwd = root
	} else if !filepath.IsAbs(s.Cwd) {
		s.Cwd = filepath.Join(root, s.Cwd)
	}
	if s.Cwd != "" {
		s.Cwd = filepath.Clean(s.Cwd)
	}
	if args, ok := e.Meta["args"].([]any); ok {
		for _, a := range args {
			if v, ok := a.(string); ok {
				s.Args = append(s.Args, resolve("args", "", v))
			}
		}
	}
	for field, into := range map[string]map[string]string{"env": s.Env, "headers": s.Headers} {
		values, _ := e.Meta[field].(map[string]any)
		for k, v := range values {
			if text, ok := v.(string); ok {
				into[k] = resolve(field, k, text)
			}
		}
	}
	helper := resolve("headersHelper", "", str("headersHelper"))
	if len(problems) > 0 {
		slices.Sort(problems)
		return s, errors.New(strings.Join(slices.Compact(problems), "; "))
	}
	if helper != "" {
		headers, err := mcpprobe.Headers(ctx, helper, s.Cwd)
		if err != nil {
			return s, err
		}
		for k, v := range headers {
			s.Headers[k] = v
		}
	}
	return s, nil
}

// resolveEnvRefs replaces each ${NAME} with the variable's value, a
// ${NAME:-default} with its default when unset, and ${workspaceFolder}
// with root. An unset variable with no default is an error naming it.
func resolveEnvRefs(value, root string) (string, error) {
	var missing []string
	out := value
	for _, t := range spec.EnvRefTokens(value) {
		if !t.Known() {
			continue
		}
		v, ok := os.LookupEnv(t.Name)
		switch {
		case t.Name == "workspaceFolder":
			v = root
		case ok:
		case t.HasDefault:
			v = t.Default
		default:
			missing = append(missing, t.Name)
			continue
		}
		out = strings.Replace(out, t.Text, v, 1)
	}
	if len(missing) > 0 {
		return out, fmt.Errorf("%s is not set", strings.Join(missing, ", "))
	}
	return out, nil
}

func printMCPTools(cmd *cobra.Command, c mcpprobe.Cache) {
	names := make([]string, 0, len(c.Servers))
	for name := range c.Servers {
		names = append(names, name)
	}
	slices.Sort(names)
	if len(names) == 0 {
		cmd.Println("No MCP servers are configured.")
		return
	}
	for _, name := range names {
		r := c.Servers[name]
		if r.Error != "" {
			cmd.Printf("%s: not listed: %s\n", name, r.Error)
			continue
		}
		tools := make([]string, 0, len(r.Tools))
		for _, t := range r.Tools {
			tools = append(tools, t.Name)
		}
		cmd.Printf("%s: %d tool(s): %s\n", name, len(r.Tools), strings.Join(tools, ", "))
	}
}
