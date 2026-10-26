// Package overview builds the harness overview: one versioned document
// with the harness sections, the configured items each holds, which
// coding tools support them, and what the read-only checks report.
//
// A check result is shown on what it concerns: a lint or doctor finding
// on the item whose file it names, or on its section when it names no
// configured item, and drift on the target whose generated files differ.
//
// The JSON contract is mirrored in ui/src/contract.ts. Change both
// together.
package overview

import "github.com/chemaclass/agnostic-ai/internal/authority"

// SchemaVersion names the shape of View.
const SchemaVersion = "overview/v1"

// View is every section overview in one document.
type View struct {
	SchemaVersion string             `json:"schemaVersion"`
	Snapshot      authority.Snapshot `json:"snapshot"`
	Sections      []Section          `json:"sections"`
	Targets       []TargetSupport    `json:"targets"`
}

// Section ids, in harness order.
const (
	SectionContext   = "context"
	SectionAuthority = "authority"
	SectionTools     = "tools"
	SectionLifecycle = "lifecycle"
	SectionSettings  = "settings"
)

// Section is one harness domain: what it holds and who supports it.
type Section struct {
	ID       string        `json:"id"`
	Label    string        `json:"label"`
	Question string        `json:"question"`
	Order    int           `json:"order"`
	Kinds    []KindCount   `json:"kinds"`
	Items    []SectionItem `json:"items"`
	Support  []KindSupport `json:"support"`
	// Findings are the check results in this section that name no
	// configured item, such as a native file doctor reads.
	Findings []ItemFinding `json:"findings"`
	// Stages are the points in a session a hook can run at, in session
	// order, each listed even when no hook runs there. Only Lifecycle has
	// them; a hook item's Group is the ID of its stage.
	Stages []Stage `json:"stages,omitempty"`
	// Fields are every settings field this section's specs can set, each
	// listed even when no spec sets it. Authority and Settings have them.
	Fields []SettingField `json:"fields,omitempty"`
}

// SettingField is one field a settings spec can set, with the value each
// spec that sets it gives.
type SettingField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	// Set lists each spec that sets the field, with its values under the
	// field's key. Empty when nothing sets it.
	Set      []FieldSource `json:"set"`
	Editable *FieldEdit    `json:"editable,omitempty"`
}

// FieldEdit describes the source-only edit available for a scalar field.
type FieldEdit struct {
	Path           string   `json:"path,omitempty"`
	Paths          []string `json:"paths,omitempty"`
	Kind           string   `json:"kind"`
	Allowed        []string `json:"allowed,omitempty"`
	DisabledReason string   `json:"disabledReason,omitempty"`
	SyncCommand    string   `json:"syncCommand,omitempty"`
	// LocalPath is the personal, git-ignored file a "just me" edit of
	// Path writes.
	LocalPath string `json:"localPath,omitempty"`
}

// FieldSource is one spec's values for a field.
type FieldSource struct {
	Spec   string  `json:"spec"`
	Path   string  `json:"path"`
	Values []Value `json:"values"`
	// Local marks a value from a personal, git-ignored file.
	Local bool `json:"local,omitempty"`
}

// Stage is one point in a session a hook can run at.
type Stage struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	// Targets are the configured coding tools that run a hook here, each
	// with its own name for the event. A native stage lists none.
	Targets []StageTarget `json:"targets"`
}

// StageTarget is one coding tool's name for a lifecycle stage.
type StageTarget struct {
	Target string `json:"target"`
	Label  string `json:"label"`
	Event  string `json:"event"`
}

// KindCount counts the items of one kind in a section.
type KindCount struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Count int    `json:"count"`
	// Local is set on the built-ins row when the personal config replaces
	// the team list.
	Local bool `json:"local,omitempty"`
}

// KindSupport lists which configured targets emit a spec kind natively,
// from the adapters' own capability declarations.
type KindSupport struct {
	Kind        string   `json:"kind"`
	Label       string   `json:"label"`
	Supported   []string `json:"supported"`
	Unsupported []string `json:"unsupported"`
}

// SectionItem is one configured thing: a spec, the instructions file,
// a config file, or an enabled built-in.
type SectionItem struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Path        string `json:"path"`
	// OpenPath is the absolute path of a Markdown or YAML file, so a local
	// client can open it in an editor. Empty for anything else.
	OpenPath string `json:"openPath,omitempty"`
	Layer    string `json:"layer"`
	Scope    string `json:"scope,omitempty"`
	// Group places the item within its kind; for a hook it is the
	// lifecycle point it runs at.
	Group string `json:"group,omitempty"`
	// Values are what the item sets, key by key: a settings spec's
	// fields or the config's keys. A value that names or looks like a
	// credential is shown as <redacted>.
	Values []Value `json:"values"`
	// Enabled is set for a built-in: whether the config turns it on.
	Enabled *bool `json:"enabled,omitempty"`
	// Local is set for a built-in the personal config turns on or off
	// against the team's list.
	Local bool `json:"local,omitempty"`
	// LocalKeys are the dotted keys a personal settings file sets itself,
	// so each can be reset.
	LocalKeys []string `json:"localKeys,omitempty"`
	// Tools is set for an MCP server: what `agnostic-ai mcp tools` last
	// listed. The overview reads that cache and never connects.
	Tools *ToolListing `json:"tools,omitempty"`
	// Findings are the check results that name this item's file.
	Findings []ItemFinding `json:"findings"`
}

// Value types, which tell a client how to render a value.
const (
	ValueText = "text"
	ValueBool = "bool"
	ValueList = "list"
)

// Value is one setting an item makes: a dotted key with a single value
// or a list of values. Type is ValueList for a list, ValueBool for a
// boolean ("true" or "false"), and ValueText for anything else.
type Value struct {
	Key   string   `json:"key"`
	Type  string   `json:"type"`
	Value string   `json:"value,omitempty"`
	List  []string `json:"list,omitempty"`
}

// Tool listing states.
const (
	ToolsListed    = "listed"
	ToolsStale     = "stale"
	ToolsFailed    = "failed"
	ToolsNotListed = "not-listed"
)

// ToolListing is one MCP server's cached tools. State is listed, stale
// when the spec changed after the listing, failed with Error, or
// not-listed when the server was never asked.
type ToolListing struct {
	State    string     `json:"state"`
	ListedAt string     `json:"listedAt,omitempty"`
	Error    string     `json:"error,omitempty"`
	Tools    []ToolInfo `json:"tools"`
	// Command refreshes the listing.
	Command string `json:"command"`
}

// ToolInfo is one tool a server offers.
type ToolInfo struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

// ItemFinding is one check result shown where it applies.
type ItemFinding struct {
	// Source is SourceLint or SourceDoctor.
	Source   string `json:"source"`
	Code     string `json:"code,omitempty"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
	// OpenPath is the absolute path of the file Path names, when that
	// file is on disk, so a local client can open it in an editor.
	OpenPath string `json:"openPath,omitempty"`
	// Fix is the command or edit that resolves it, when known.
	Fix string `json:"fix,omitempty"`
	// Explanation says why it matters, when known.
	Explanation string `json:"explanation,omitempty"`
}

// TargetSupport lists the spec kinds one configured target emits, and
// whether its generated files match what sync would write.
type TargetSupport struct {
	Target string     `json:"target"`
	Label  string     `json:"label"`
	Kinds  []string   `json:"kinds"`
	Sync   TargetSync `json:"sync"`
}

// Sync states of a target.
const (
	SyncInSync     = "in-sync"
	SyncDrifted    = "drifted"
	SyncNotChecked = "not-checked"
)

// TargetSync is the drift state of one target: in-sync when every file
// sync would write matches disk, drifted with the files that differ, or
// not-checked when the comparison did not run for it.
type TargetSync struct {
	State string     `json:"state"`
	Files []SyncFile `json:"files"`
}

// SyncFile is one generated file that differs from what sync would
// write: "stale" when it differs, "not generated" when it is missing.
type SyncFile struct {
	Path  string `json:"path"`
	State string `json:"state"`
}

// Finding sources.
const (
	SourceLint   = "lint"
	SourceDoctor = "doctor"
	SourceDrift  = "drift"
)

// Finding is one read-only check result handed in by the caller. The
// overview package cannot run lint, doctor, or drift itself; the CLI
// collects them and passes them here.
type Finding struct {
	// Source is SourceLint, SourceDoctor, or SourceDrift.
	Source   string
	Code     string
	Severity string
	Message  string
	Path     string
	Line     int
	// OpenPath is the absolute path of the file Path names, when it is on disk.
	OpenPath string
	// Target names the coding tool a drift finding concerns.
	Target string
	// Fix is the command or edit that resolves the finding, when known.
	Fix string
	// Explanation says why the finding matters, when known.
	Explanation string
	// Section is the harness section a finding that names no configured
	// item belongs to, when the check knows it.
	Section string
}

// Document is an instructions file the caller found, such as
// AGNOSTIC_AI.md, with its path as the view shows it.
type Document struct {
	Path string
	// AbsPath is where the file is on disk.
	AbsPath string
	Layer   string
	Words   int
}
