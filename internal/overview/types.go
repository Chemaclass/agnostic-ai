// Package overview builds the harness overview and its feed: one
// versioned document with the six harness sections, the configured
// items each holds, which coding tools support them, and a bounded feed
// of evidence-backed cards derived from existing findings.
//
// The feed is derived, never generated. A card exists only because a
// read-only check (lint, doctor, drift, or the authority view's own
// uncertainties) produced a finding with a source path. Deterministic
// derivation does not make a card correct, so every card links to its
// evidence and names the check that produced it.
//
// The JSON contract is mirrored in ui/src/contract.ts. Change both
// together.
package overview

import "github.com/chemaclass/agnostic-ai/internal/authority"

// SchemaVersion names the shape of View and FeedView.
const SchemaVersion = "overview/v1"

// View is the home page and every section overview in one document.
type View struct {
	SchemaVersion string             `json:"schemaVersion"`
	Snapshot      authority.Snapshot `json:"snapshot"`
	Sections      []Section          `json:"sections"`
	Feed          []Card             `json:"feed"`
	Targets       []TargetSupport    `json:"targets"`
	Explanations  map[string]string  `json:"explanations"`
}

// FeedView is the feed alone, for agents that only want the cards.
type FeedView struct {
	SchemaVersion string             `json:"schemaVersion"`
	Snapshot      authority.Snapshot `json:"snapshot"`
	Feed          []Card             `json:"feed"`
}

// Section ids, in harness order.
const (
	SectionContext      = "context"
	SectionMemory       = "memory"
	SectionControl      = "control"
	SectionAuthority    = "authority"
	SectionTools        = "tools"
	SectionVerification = "verification"
)

// Section is one harness domain: what it holds and who supports it.
type Section struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Question    string `json:"question"`
	Description string `json:"description"`
	Order       int    `json:"order"`
	// Status is "deep-view" when a dedicated view exists (Authority) or
	// "overview" when only this summary does.
	Status   string        `json:"status"`
	DeepView string        `json:"deepView,omitempty"`
	Kinds    []KindCount   `json:"kinds"`
	Items    []SectionItem `json:"items"`
	Support  []KindSupport `json:"support"`
	CardIDs  []string      `json:"cardIds"`
}

// KindCount counts the specs of one kind in a section.
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

// SectionItem is one configured spec.
type SectionItem struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Layer       string `json:"layer"`
	Scope       string `json:"scope,omitempty"`
}

// Card kinds, sources, and action kinds.
const (
	CardDecision = "decision"
	CardInsight  = "insight"

	SourceAuthority = "authority"
	SourceDrift     = "drift"
	SourceDoctor    = "doctor"
	SourceLint      = "lint"

	ActionOpen        = "open"
	ActionPropose     = "propose"
	ActionCopyCommand = "copy-command"
	ActionDismiss     = "dismiss"
	ActionSnooze      = "snooze"
)

// Priorities rank cards by consequence, lowest number first.
const (
	PrioritySecurity = 1 // a rule widens access or a deny blocks nothing
	PriorityDrift    = 2 // the tool runs on stale generated config
	PriorityCoverage = 3 // a tool receives none of a declaration
	PriorityHygiene  = 4 // lint and doctor findings
	PriorityInfo     = 5 // uncertainties worth knowing
)

// Card is one feed item.
type Card struct {
	ID string `json:"id"`
	// Fingerprint is stable across revisions for the same finding, so a
	// client can keep a dismissal until the finding changes.
	Fingerprint   string         `json:"fingerprint"`
	Kind          string         `json:"kind"`
	Priority      int            `json:"priority"`
	PriorityLabel string         `json:"priorityLabel"`
	Title         string         `json:"title"`
	Consequence   string         `json:"consequence"`
	SectionID     string         `json:"sectionId"`
	DomainID      string         `json:"domainId,omitempty"`
	Source        string         `json:"source"`
	Code          string         `json:"code,omitempty"`
	Evidence      []CardEvidence `json:"evidence"`
	Actions       []CardAction   `json:"actions"`
	Revision      string         `json:"revision"`
}

// CardEvidence points at the source of a finding.
type CardEvidence struct {
	Path   string `json:"path"`
	Line   int    `json:"line,omitempty"`
	Detail string `json:"detail"`
}

// CardAction is one thing a reader can do with a card. Nothing here
// runs a command or writes a file: "copy-command" hands over the exact
// command, and "propose" hands over an authority.Proposal to validate.
type CardAction struct {
	ID       string              `json:"id"`
	Kind     string              `json:"kind"`
	Label    string              `json:"label"`
	Href     string              `json:"href,omitempty"`
	Command  string              `json:"command,omitempty"`
	Proposal *authority.Proposal `json:"proposal,omitempty"`
}

// TargetSupport lists the spec kinds one configured target emits.
type TargetSupport struct {
	Target string   `json:"target"`
	Label  string   `json:"label"`
	Kinds  []string `json:"kinds"`
}

// Finding is one read-only check result handed in by the caller. The
// overview package cannot run lint, doctor, or drift itself; the CLI
// collects them and passes them here.
type Finding struct {
	// Source is SourceLint, SourceDoctor, or SourceDrift.
	Source   string `json:"source"`
	Code     string `json:"code,omitempty"`
	Severity string `json:"severity,omitempty"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
	Target   string `json:"target,omitempty"`
	// Fix is the command or edit that resolves the finding, when known.
	Fix string `json:"fix,omitempty"`
	// Explanation says why the finding matters, such as the cause
	// `agnostic-ai explain <CODE>` prints for a lint code, when known.
	// A card shows it as its consequence.
	Explanation string `json:"explanation,omitempty"`
	// Section is the harness section the finding belongs to, when the
	// check knows it. Empty lets the overview place it by its path.
	Section string `json:"section,omitempty"`
	// Identity tells apart findings whose Message was redacted to the
	// same text. It is the message as written, so it is never served.
	Identity string `json:"-"`
}
