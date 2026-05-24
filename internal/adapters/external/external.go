// Package external implements an Adapter that delegates to an out-of-
// tree binary discovered on PATH. Any executable named
// `agnostic-ai-adapter-<name>` is a candidate adapter; Discover returns
// the wrapper when the binary is on PATH.
//
// The wire protocol is JSON over stdin/stdout. See
// docs/internal/plugin-protocol.md for the format and stability rules.
package external

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// ProtocolVersion is the on-the-wire protocol version. External adapters
// inspect this in the input envelope to decide whether they support the
// host's request shape; the host rejects responses that report an
// unrecognized version.
const ProtocolVersion = 1

// BinaryPrefix is the PATH lookup prefix. A binary named
// `agnostic-ai-adapter-foo` is the adapter for target `foo`.
const BinaryPrefix = "agnostic-ai-adapter-"

// Input is the JSON document the host writes to the adapter's stdin.
type Input struct {
	ProtocolVersion int            `json:"protocol_version"`
	Command         string         `json:"command"`
	Target          string         `json:"target"`
	DryRun          bool           `json:"dry_run"`
	Config          ConfigEnvelope `json:"config"`
	Specs           SpecsEnvelope  `json:"specs"`
}

// ConfigEnvelope mirrors the user's agnostic-ai.yaml fields the
// adapter most commonly needs. The full Config is JSON-marshaled into
// Raw so adapters can read fields the envelope does not surface yet
// without bumping ProtocolVersion.
type ConfigEnvelope struct {
	Sources       config.Sources           `json:"sources"`
	Outputs       map[string]config.Output `json:"outputs,omitempty"`
	OnUnsupported string                   `json:"on_unsupported"`
	Targets       []string                 `json:"targets"`
	Raw           map[string]any           `json:"raw,omitempty"`
}

// SpecsEnvelope buckets spec entries by kind, matching spec.Bundle.
type SpecsEnvelope struct {
	Agents []SpecEntry `json:"agents,omitempty"`
	Skills []SpecEntry `json:"skills,omitempty"`
	Rules  []SpecEntry `json:"rules,omitempty"`
	Hooks  []SpecEntry `json:"hooks,omitempty"`
	MCPs   []SpecEntry `json:"mcps,omitempty"`
}

// SpecEntry is the JSON shape of one spec passed to the adapter. The
// fields mirror spec.Entry but use snake_case for cross-language
// adapter implementations.
type SpecEntry struct {
	Kind     string         `json:"kind"`
	Name     string         `json:"name"`
	Path     string         `json:"path"`
	Scope    string         `json:"scope,omitempty"`
	Layer    string         `json:"layer,omitempty"`
	Meta     map[string]any `json:"meta,omitempty"`
	MetaKeys []string       `json:"meta_keys,omitempty"`
	Body     string         `json:"body,omitempty"`
}

// Output is the JSON document the adapter writes to its stdout.
type Output struct {
	ProtocolVersion int      `json:"protocol_version"`
	Files           []File   `json:"files,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
	Errors          []string `json:"errors,omitempty"`
}

// File is one (path, content) pair the adapter wants the host to write.
// Paths are relative to the project root and must not escape it.
type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Adapter wraps an external binary and satisfies adapters.Adapter.
type Adapter struct {
	name    string
	command func() *exec.Cmd
}

// New returns an Adapter for the named target by looking up
// `agnostic-ai-adapter-<name>` on PATH. An error is returned when no
// such binary is on PATH or when name contains characters that cannot
// be part of a binary name.
func New(name string) (*Adapter, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	bin := BinaryPrefix + name
	path, err := exec.LookPath(bin)
	if err != nil {
		return nil, fmt.Errorf("external adapter %q: %w", name, err)
	}
	return NewWithCommand(name, func() *exec.Cmd { return exec.Command(path) }), nil
}

// NewWithCommand builds an Adapter around an arbitrary command factory.
// Used by tests that exercise the protocol without depending on a real
// binary on PATH.
func NewWithCommand(name string, cmd func() *exec.Cmd) *Adapter {
	return &Adapter{name: name, command: cmd}
}

// Name returns the target identifier.
func (a *Adapter) Name() string { return a.name }

// Emit serializes the bundle and config, runs the external binary, and
// writes any files the adapter returns through the shared emit layer.
// Adapter-reported warnings go to the package warner; adapter errors
// become a returned error.
func (a *Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	in := buildInput(a.name, b, cfg, dryRun)
	out, err := run(a.command(), in)
	if err != nil {
		return fmt.Errorf("%s: %w", a.name, err)
	}
	if out.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("%s: protocol_version %d not supported (host expects %d)", a.name, out.ProtocolVersion, ProtocolVersion)
	}
	for _, w := range out.Warnings {
		_, _ = fmt.Fprintf(emit.Warner, "! %s: %s\n", a.name, w)
	}
	if len(out.Errors) > 0 {
		return fmt.Errorf("%s: %s", a.name, strings.Join(out.Errors, "; "))
	}
	for _, f := range out.Files {
		if err := emit.WriteFile(f.Path, f.Content, dryRun); err != nil {
			return fmt.Errorf("%s: %w", a.name, err)
		}
	}
	return nil
}

func buildInput(target string, b spec.Bundle, cfg *config.Config, dryRun bool) Input {
	in := Input{
		ProtocolVersion: ProtocolVersion,
		Command:         "emit",
		Target:          target,
		DryRun:          dryRun,
		Config: ConfigEnvelope{
			Sources:       cfg.Sources,
			Outputs:       cfg.Outputs,
			OnUnsupported: cfg.OnUnsupported,
			Targets:       cfg.Targets,
		},
	}
	in.Specs.Agents = entriesToWire(b.Agents)
	in.Specs.Skills = entriesToWire(b.Skills)
	in.Specs.Rules = entriesToWire(b.Rules)
	in.Specs.Hooks = entriesToWire(b.HooksFor(target))
	in.Specs.MCPs = entriesToWire(b.MCPs)
	return in
}

func entriesToWire(entries []spec.Entry) []SpecEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]SpecEntry, len(entries))
	for i, e := range entries {
		out[i] = SpecEntry{
			Kind:     string(e.Kind),
			Name:     e.Name,
			Path:     e.Path,
			Scope:    e.Scope,
			Layer:    e.Layer,
			Meta:     e.Meta,
			MetaKeys: e.MetaKeys,
			Body:     e.Body,
		}
	}
	return out
}

// run feeds in over stdin and decodes Output from stdout. Stderr is
// surfaced verbatim on failure so the user sees the adapter's own log.
func run(cmd *exec.Cmd, in Input) (Output, error) {
	payload, err := json.Marshal(in)
	if err != nil {
		return Output{}, fmt.Errorf("marshal input: %w", err)
	}
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return Output{}, fmt.Errorf("run: %w", err)
		}
		return Output{}, fmt.Errorf("run: %w: %s", err, msg)
	}
	var out Output
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		preview := strings.TrimSpace(stdout.String())
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return Output{}, fmt.Errorf("decode output: %w (got %q)", err, preview)
	}
	return out, nil
}

// EncodeOutput is a helper for adapter authors implementing this
// protocol in Go. Writes a properly framed Output document to w.
func EncodeOutput(w io.Writer, out Output) error {
	if out.ProtocolVersion == 0 {
		out.ProtocolVersion = ProtocolVersion
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}

// DecodeInput is a helper for adapter authors implementing this
// protocol in Go. Reads and decodes an Input document from r.
func DecodeInput(r io.Reader) (Input, error) {
	var in Input
	dec := json.NewDecoder(r)
	if err := dec.Decode(&in); err != nil {
		return Input{}, fmt.Errorf("decode input: %w", err)
	}
	return in, nil
}

// validateName rejects names that would produce surprising binary
// lookups (path separators, empty, leading dash).
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("external adapter: empty name")
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("external adapter: name %q contains a path separator", name)
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("external adapter: name %q must not start with '-'", name)
	}
	return nil
}
