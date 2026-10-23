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
	ID       string `json:"id"`
	Label    string `json:"label"`
	Question string `json:"question"`
	Order    int    `json:"order"`
	// DeepView is the client route of a dedicated view (Authority), if
	// one exists.
	DeepView string        `json:"deepView,omitempty"`
	Kinds    []KindCount   `json:"kinds"`
	Items    []SectionItem `json:"items"`
	Support  []KindSupport `json:"support"`
	// Findings are the check results in this section that name no
	// configured item, such as a native file doctor reads.
	Findings []ItemFinding `json:"findings"`
}

// KindCount counts the items of one kind in a section.
type KindCount struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Count int    `json:"count"`
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
	Layer       string `json:"layer"`
	Scope       string `json:"scope,omitempty"`
	// Group places the item within its kind; for a hook it is the
	// lifecycle point it runs at.
	Group string `json:"group,omitempty"`
	// Findings are the check results that name this item's file.
	Findings []ItemFinding `json:"findings"`
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
	Path  string
	Layer string
	Words int
}
