// Package authority builds the Authority view: which subject (an agent
// or the tool session) may perform which exact action on which exact
// target, under what conditions, on which coding tool, and why.
//
// The view is one coherent, versioned document. The server owns
// normalization, precedence, provenance, and impact analysis; a client
// owns rendering, selection, and motion. Every identifier here is
// stable and machine-readable, and every item also carries a
// human-readable label so no client has to decode an internal id.
//
// Four states are kept apart and never collapsed into one badge:
//
//   - declared: what the canonical specs say, before any tool reads them
//   - effective: what the selected coding tool (the harness) will do with
//     that declaration, once its adapter has translated it
//   - installed: whether the generated file that carries the decision
//     matches what is on disk
//   - runtime: what a running tool actually enforced, which this view
//     never observes
//
// The JSON contract is documented in docs/internal/authority-view.md and
// mirrored in ui/src/contract.ts. Change all three together.
package authority

// SchemaVersion names the shape of View and ImpactResult. Bump it when a
// field changes meaning; adding an optional field keeps the version.
const SchemaVersion = "authority/v1"

// View is the complete initial state of the Authority view for one
// subject on one harness. It answers the acceptance questions directly:
// concrete resources, precise operations, effective effects, conditions,
// provenance, and enforcement, with every uncertainty listed.
type View struct {
	SchemaVersion string   `json:"schemaVersion"`
	Snapshot      Snapshot `json:"snapshot"`
	// Harness is the coding tool the effects are resolved for.
	Harness Harness `json:"harness"`
	// Agent is the selected subject, fully resolved.
	Agent Subject `json:"agent"`
	// Agents lists every selectable subject, the selected one included.
	Agents    []SubjectSummary `json:"agents"`
	Domains   []Domain         `json:"domains"`
	Providers []Provider       `json:"providers"`
	Resources []Resource       `json:"resources"`
	// Edges are the permission circuit: subject → operation → resource,
	// carrying the effective effect on the selected harness.
	Edges   []Edge   `json:"edges"`
	Secrets []Secret `json:"secrets"`
	// Uncertainties are everything the view cannot determine. A client
	// shows them; it never presents an unknown as granted or denied.
	Uncertainties []Uncertainty `json:"uncertainties"`
	// SupportedActions are the mutations and queries a client may offer.
	SupportedActions []SupportedAction `json:"supportedActions"`
	Explanations     Explanations      `json:"explanations"`
}

// Snapshot identifies the configuration the view was built from and
// what kinds of evidence went into it.
type Snapshot struct {
	// Revision is a short content hash of every input: the config files
	// and every loaded spec. Two views with the same revision were built
	// from the same sources. A Proposal that names a revision is refused
	// when it no longer matches.
	Revision    string  `json:"revision"`
	GeneratedAt string  `json:"generatedAt"`
	Project     Project `json:"project"`
	// Targets are the coding tools the project syncs to, in config order.
	Targets []string `json:"targets"`
	// ConfigFiles are the config files read, relative to the project root.
	ConfigFiles []string `json:"configFiles"`
	// Specs lists every spec that contributed, for the JSON inspector.
	Specs    []SpecRef `json:"specs"`
	Evidence Evidence  `json:"evidence"`
}

// Evidence states the coverage and freshness of each evidence class.
type Evidence struct {
	// Declared is always "loaded": the specs were read at GeneratedAt.
	Declared string `json:"declared"`
	// Installed is "checked" when generated files were compared with
	// disk, or "not-checked".
	Installed string `json:"installed"`
	// Runtime is always "not-observed".
	Runtime string `json:"runtime"`
	Note    string `json:"note"`
}

// Project names the inspected project without leaking machine paths
// beyond its root.
type Project struct {
	Root string `json:"root"`
	Name string `json:"name"`
}

// SpecRef points at one loaded spec.
type SpecRef struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	Layer string `json:"layer"`
}

// Source is the provenance of one declaration: the spec file and the
// layer it came from. Line is set when known and omitted otherwise.
type Source struct {
	Path  string `json:"path"`
	Layer string `json:"layer"`
	Line  int    `json:"line,omitempty"`
}

// Harness is the coding tool the view resolves effects for.
type Harness struct {
	// Target is the adapter name, such as "claude" or "codex".
	Target string `json:"target"`
	Label  string `json:"label"`
	// Available lists the targets the view can be resolved for.
	Available []HarnessOption   `json:"available"`
	Identity  ExecutionIdentity `json:"identity"`
	Installed InstalledState    `json:"installed"`
	// PermissionSupport is "native-rules" when the adapter writes
	// permission rules, "hooks-only" when it enforces only protected
	// paths through a hook, or "none".
	PermissionSupport string `json:"permissionSupport"`
	Note              string `json:"note,omitempty"`
}

// HarnessOption is one selectable harness.
type HarnessOption struct {
	Target string `json:"target"`
	Label  string `json:"label"`
}

// ExecutionIdentity states who the tool acts as. The view resolves no
// user name: it states the kind of identity and where credentials come
// from.
type ExecutionIdentity struct {
	// Kind is "local-process".
	Kind        string `json:"kind"`
	Description string `json:"description"`
}

// Installed states.
const (
	InstalledInSync       = "in-sync"       // every generated file matches disk
	InstalledStale        = "stale"         // at least one generated file differs from disk
	InstalledNotGenerated = "not-generated" // sync has not written the files yet
	InstalledNotChecked   = "not-checked"   // the caller did not compare
)

// InstalledState compares the files sync would generate for the harness
// with what is on disk.
type InstalledState struct {
	State     string          `json:"state"`
	Files     []InstalledFile `json:"files"`
	CheckedAt string          `json:"checkedAt,omitempty"`
}

// InstalledFile is one generated file and its state.
type InstalledFile struct {
	Path  string `json:"path"`
	State string `json:"state"`
}

// SubjectKind distinguishes a declared agent from the tool session
// itself, which runs with no agent narrowing.
type SubjectKind string

const (
	SubjectAgent   SubjectKind = "agent"
	SubjectSession SubjectKind = "session"
)

// SessionSubjectID is the id of the synthetic session subject.
const SessionSubjectID = "session"

// SubjectSummary is one selectable subject.
type SubjectSummary struct {
	ID          string      `json:"id"`
	Label       string      `json:"label"`
	Description string      `json:"description"`
	Kind        SubjectKind `json:"kind"`
}

// Subject is the selected subject with the narrowing it declares.
type Subject struct {
	SubjectSummary
	Source *Source `json:"source,omitempty"`
	// ToolScope is the agent's own tool list. Mode "unrestricted" means
	// the agent inherits every tool the session permits.
	ToolScope ToolScope `json:"toolScope"`
	// Readonly reports an agent whose spec sets readonly: true.
	Readonly bool `json:"readonly"`
	// MCPServers is the agent's mcpServers list. Mode "all" means the
	// session's full set.
	MCPServers MCPScope `json:"mcpServers"`
}

// ToolScope is an agent's declared tool list.
type ToolScope struct {
	// Mode is "unrestricted" or "listed".
	Mode string `json:"mode"`
	// Field is "can" or "tools", or empty when unrestricted.
	Field string `json:"field,omitempty"`
	// Declared holds the list as written in the spec.
	Declared []string `json:"declared"`
	// Capabilities holds the neutral capability each entry stands for,
	// or the entry itself when no neutral form exists.
	Capabilities []string `json:"capabilities"`
}

// MCPScope is an agent's declared MCP server list.
type MCPScope struct {
	// Mode is "all" or "listed".
	Mode  string   `json:"mode"`
	Names []string `json:"names"`
}

// Domain is one curated capability domain: a navigation group, never
// an enforcement boundary. A provider or resource may belong to several.
type Domain struct {
	ID          string       `json:"id"`
	Label       string       `json:"label"`
	Description string       `json:"description"`
	Examples    []string     `json:"examples"`
	Order       int          `json:"order"`
	ProviderIDs []string     `json:"providerIds"`
	ResourceIDs []string     `json:"resourceIds"`
	Summary     EffectCounts `json:"summary"`
}

// EffectCounts counts operations by effective effect.
type EffectCounts struct {
	Allowed     int `json:"allowed"`
	Ask         int `json:"ask"`
	Denied      int `json:"denied"`
	Unavailable int `json:"unavailable"`
	Default     int `json:"default"`
	Unresolved  int `json:"unresolved"`
}

// Curated domain ids. The set is a navigation default, not a closed
// ontology: an unknown provider still renders, under DomainAgentOps,
// with DomainResolutionDefault set so a client can say so.
const (
	DomainSourceControl = "source-control"
	DomainCloud         = "cloud-deployment"
	DomainData          = "data-storage"
	DomainFilesystem    = "filesystem-execution"
	DomainWeb           = "web-browser"
	DomainCommunication = "communication"
	DomainSecrets       = "secrets-identity"
	DomainAgentOps      = "ai-agent-ops"
)

// How a provider's domains were decided.
const (
	DomainResolutionCurated = "curated" // matched the curated provider table
	DomainResolutionBuiltin = "builtin" // a built-in capability such as local files
	DomainResolutionDefault = "default" // unknown provider, placed under ai-agent-ops
)

// Provider is a system that exposes resources: an MCP server, or a
// built-in capability of the coding tool (local files, shell, web).
type Provider struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	// Kind is "mcp" or "builtin".
	Kind             string   `json:"kind"`
	DomainIDs        []string `json:"domainIds"`
	DomainResolution string   `json:"domainResolution"`
	// Status is "configured", "disabled" (the spec sets disabled: true),
	// or "unavailable-to-agent" (the agent's mcpServers list leaves it out).
	Status      string     `json:"status"`
	Transport   *Transport `json:"transport,omitempty"`
	Source      *Source    `json:"source,omitempty"`
	ResourceIDs []string   `json:"resourceIds"`
	SecretIDs   []string   `json:"secretIds"`
	// ResourceCoverage is "declared" when the spec names the boundaries
	// the server is told to work within (MCP roots), "complete" for
	// built-ins whose resources are fully known from rules, or
	// "undeclared" when the spec names none. Credential reach is never
	// inspected, whatever the value.
	ResourceCoverage string `json:"resourceCoverage"`
	CoverageNote     string `json:"coverageNote,omitempty"`
}

// Transport describes how an MCP server is reached. It never carries a
// secret value: a URL or header that references one keeps the `${NAME}`
// reference as written.
type Transport struct {
	Type     string `json:"type"`
	Endpoint string `json:"endpoint,omitempty"`
}

// Resource kinds.
const (
	// ResourceUndeclaredScope stands for an MCP server whose spec names
	// no roots: the scope is not declared and credential reach is not
	// inspected.
	ResourceUndeclaredScope = "undeclared-scope"
	// ResourceDeclaredRoot is one MCP root with a file URI. The MCP
	// specification defines roots as informational filesystem guidance
	// for a server, not access control, so a root is shown as a named
	// boundary with that caveat. A root whose URI is not a file URI
	// establishes nothing: it is reported through the
	// UncertaintyRootNotFilesystem code and the server keeps an
	// undeclared scope.
	ResourceDeclaredRoot   = "declared-root"
	ResourceFilePattern    = "file-pattern" // files matching a rule pattern
	ResourceCommandPattern = "command-pattern"
	ResourceWeb            = "web"
)

// Resource is one concrete target. Its label is the name a human uses
// for it; Identity carries the machine form.
type Resource struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Kind        string   `json:"kind"`
	ProviderID  string   `json:"providerId"`
	DomainIDs   []string `json:"domainIds"`
	Identity    Identity `json:"identity"`
	// Basis says what establishes the resource: "rule-pattern" (a
	// permission rule names it), "declared-root" (the MCP spec declares
	// it as a boundary), or "undeclared".
	Basis string `json:"basis"`
	// Caveat states what the basis does not prove.
	Caveat string `json:"caveat,omitempty"`
	// Operations are every action the view knows about on this
	// resource, each with its effective effect on the selected harness.
	Operations []Operation `json:"operations"`
	// Matches resolves a file pattern against the project tree, bounded.
	Matches *Matches `json:"matches,omitempty"`
	// LabelResolved is false when the label is a fallback such as a URI,
	// because the spec gave no human name.
	LabelResolved bool `json:"labelResolved"`
	// UnresolvedReason explains a resource the view cannot enumerate.
	UnresolvedReason string `json:"unresolvedReason,omitempty"`
}

// Identity is the machine form of a resource.
type Identity struct {
	URI     string `json:"uri,omitempty"`
	Pattern string `json:"pattern,omitempty"`
	Server  string `json:"server,omitempty"`
}

// Matches is a bounded resolution of a file pattern.
type Matches struct {
	Count     int      `json:"count"`
	Samples   []string `json:"samples"`
	Truncated bool     `json:"truncated"`
}

// Effect is a decision for one operation.
type Effect string

const (
	EffectAllowed     Effect = "allowed"     // an allow rule wins
	EffectAsk         Effect = "ask"         // an ask rule wins: the tool prompts
	EffectDenied      Effect = "denied"      // a deny rule wins
	EffectUnavailable Effect = "unavailable" // the subject's own scope leaves this route out
	EffectDefault     Effect = "default"     // no rule: the tool's own default applies
	EffectUnresolved  Effect = "unresolved"  // the view cannot decide for this harness
)

// Resolution says how the effective effect was reached on the harness.
//
// "translated" is claimed only when the adapter writes the winning rule
// natively AND the tool's matching semantics for that rule form are
// documented in this repository (whole-tool rules and exact MCP tool
// names on every tool that writes them; prefix, glob, and domain forms
// only on tools whose documentation states how they match). A rule the
// adapter writes but whose matching is not documented resolves to
// "translated-unverified", and the effect stays unresolved: output is
// evidence that a rule was written, not that the tool decides as the
// declaration intends.
const (
	ResolutionTranslated           = "translated"            // native rule written and its matching documented
	ResolutionTranslatedUnverified = "translated-unverified" // native rule written, matching not documented: effect unresolved
	ResolutionHook                 = "hook"                  // a generated hook enforces it
	ResolutionNotExpressed         = "not-expressed"         // declared, but the adapter writes nothing for it
	ResolutionAgentScope           = "agent-scope"           // the subject's own spec decides, on a tool that reads agent tool lists
	ResolutionNoRule               = "no-rule"               // nothing declared; the tool's default applies
	ResolutionDisabled             = "disabled"              // the provider is switched off, on a tool that honors disabled
)

// Operation is one action on one resource, resolved for the subject on
// the selected harness.
type Operation struct {
	ID string `json:"id"`
	// Action is the neutral capability or MCP tool: "read", "edit",
	// "write", "delete", "run", "fetch", "search", or a tool name.
	Action string `json:"action"`
	Label  string `json:"label"`
	// Declared is the canonical decision from the specs alone, by
	// neutral precedence (deny over ask over allow), before any tool
	// reads it.
	Declared Effect `json:"declared"`
	// Effect is the effective decision on the selected harness. It
	// equals Declared only when the adapter translates the winning rule
	// (or a hook enforces it); otherwise it is unresolved.
	Effect     Effect `json:"effect"`
	Resolution string `json:"resolution"`
	// Native holds the harness's own rule or hook for the decision.
	Native []string `json:"native,omitempty"`
	// Installed is the state of the generated file that carries the
	// decision on the harness: in-sync, stale, not-generated, not-checked,
	// or not-applicable.
	Installed string `json:"installed"`
	// Hypothetical is true when Effect describes what the harness would
	// do once the generated file is in sync, because Installed is stale,
	// not-generated, or not-checked. A client words the effect
	// conditionally while this is true.
	Hypothetical bool `json:"hypothetical"`
	// Runtime is always "not-observed".
	Runtime string `json:"runtime"`
	// Decided is the grant that won, when a rule decided the effect.
	Decided *Grant `json:"decided,omitempty"`
	// Grants are every declaration that applied, winner first.
	Grants     []Grant  `json:"grants"`
	Conditions []string `json:"conditions"`
	// AlternateRoutes names other routes that could reach the same
	// system and that this operation does not govern, such as shell
	// access to a provider's command-line client.
	AlternateRoutes []string `json:"alternateRoutes"`
	// Enforcement states how every configured target carries the
	// declared decision, the selected harness included.
	Enforcement []Enforcement `json:"enforcement"`
	Explanation string        `json:"explanation"`
}

// Grant is one declaration that applies to an operation.
type Grant struct {
	// Rule is the declaration as written.
	Rule string `json:"rule"`
	// List is "allow", "ask", "deny", "protected", "can", "tools",
	// "readonly", or "mcpServers".
	List string `json:"list"`
	// Scope is "project" for settings specs and "agent" for the
	// subject's own spec.
	Scope  string `json:"scope"`
	Source Source `json:"source"`
	Wins   bool   `json:"wins"`
	Note   string `json:"note,omitempty"`
}

// Enforcement states how one coding tool carries the declared decision.
type Enforcement struct {
	Target string `json:"target"`
	Label  string `json:"label"`
	// Status is "native-rule" (the tool's own permission rule),
	// "hook" (a generated hook), "not-expressed" (sync writes nothing
	// for it; the tool's defaults apply), or "not-applicable".
	Status string `json:"status"`
	// Effect is the effective decision on that tool.
	Effect Effect   `json:"effect"`
	Native []string `json:"native,omitempty"`
	Note   string   `json:"note,omitempty"`
}

// Edge is one subject → operation → resource relationship for the
// permission circuit.
type Edge struct {
	ID          string `json:"id"`
	SubjectID   string `json:"subjectId"`
	ResourceID  string `json:"resourceId"`
	OperationID string `json:"operationId"`
	// Effect is the effective effect on the selected harness; Declared
	// is the canonical decision.
	Effect   Effect `json:"effect"`
	Declared Effect `json:"declared"`
	Label    string `json:"label"`
}

// Secret is a credential the configuration references by name. The
// view never carries a value.
type Secret struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Label        string       `json:"label"`
	ProviderIDs  []string     `json:"providerIds"`
	ReferencedBy []SecretRef  `json:"referencedBy"`
	Rights       SecretRights `json:"rights"`
	Explanation  string       `json:"explanation"`
}

// SecretRef is one place a secret is referenced.
type SecretRef struct {
	Source Source `json:"source"`
	Field  string `json:"field"`
}

// SecretRights separates the four rights over a secret. Each value is
// "allowed", "unavailable", "not-readable", or "unknown".
type SecretRights struct {
	Discover  string `json:"discover"`
	ReadValue string `json:"readValue"`
	Use       string `json:"use"`
	Manage    string `json:"manage"`
}

// Uncertainty is one thing the view cannot determine.
type Uncertainty struct {
	ID string `json:"id"`
	// Code is one of the UncertaintyX constants.
	Code string `json:"code"`
	// Severity is "info" or "warning".
	Severity   string   `json:"severity"`
	Message    string   `json:"message"`
	RelatedIDs []string `json:"relatedIds"`
}

// Uncertainty codes.
const (
	UncertaintyRuntimeNotObserved  = "runtime-not-observed"
	UncertaintyScopeUndeclared     = "provider-scope-undeclared"
	UncertaintyCredentialReach     = "credential-reach-not-inspected"
	UncertaintyDomainDefaulted     = "domain-defaulted"
	UncertaintyTargetNotExpressed  = "target-not-expressed"
	UncertaintyToolListUnknown     = "tool-list-unknown"
	UncertaintyPatternApproximate  = "pattern-containment-approximate"
	UncertaintySecretReadUnknown   = "secret-read-unknown"
	UncertaintyAlternateRoute      = "alternate-route-not-modeled"
	UncertaintyInstalledNotChecked = "installed-not-checked"
	UncertaintyRootNotFilesystem   = "root-not-filesystem"
	UncertaintyMatchingUnverified  = "target-matching-unverified"
)

// SupportedAction is one query or mutation a client may offer.
type SupportedAction struct {
	ID          string `json:"id"`
	Method      string `json:"method,omitempty"`
	Path        string `json:"path,omitempty"`
	Description string `json:"description"`
	// Status is "available" or "unsupported".
	Status string `json:"status"`
}

// Explanations is a glossary a client shows next to each effect, list,
// resolution, and enforcement status.
type Explanations struct {
	Effects     map[string]string `json:"effects"`
	Resolution  map[string]string `json:"resolution"`
	Lists       map[string]string `json:"lists"`
	Enforcement map[string]string `json:"enforcement"`
	Installed   map[string]string `json:"installed"`
	Rights      map[string]string `json:"rights"`
}

// Proposal is a client's proposed change to the project's permission
// lists. Changes apply in order to a copy of the specs; nothing is
// written.
type Proposal struct {
	AgentID string `json:"agentId"`
	// Target is the harness to resolve the impact for. Empty uses the
	// first configured target.
	Target string `json:"target,omitempty"`
	// Revision, when set, must match the current snapshot. A mismatch is
	// reported in ImpactResult.Problems and the impact is not computed.
	Revision string   `json:"revision,omitempty"`
	Changes  []Change `json:"changes"`
}

// Change is one edit to a permission list.
type Change struct {
	// Op is "add" or "remove".
	Op string `json:"op"`
	// List is "allow", "ask", or "deny".
	List string `json:"list"`
	Rule string `json:"rule"`
	// Source is the settings spec path to edit. Empty picks the first
	// project settings spec that has a permissions block, or
	// `settings/permissions.yaml` under the project sources when none has.
	Source string `json:"source,omitempty"`
}

// ImpactResult is the server's authoritative impact of a Proposal. It
// describes a proposed state: nothing is applied, installed, or observed.
type ImpactResult struct {
	SchemaVersion    string `json:"schemaVersion"`
	Status           string `json:"status"` // always "proposed"
	Target           string `json:"target"`
	BaseRevision     string `json:"baseRevision"`
	ProposedRevision string `json:"proposedRevision"`
	// Valid is false when a change cannot be applied; Problems say why.
	Valid    bool           `json:"valid"`
	Problems []string       `json:"problems"`
	Changes  []EffectChange `json:"changes"`
	Summary  ImpactSummary  `json:"summary"`
	// AffectedSubjects lists every subject whose effective permissions
	// change, the selected one included.
	AffectedSubjects []string   `json:"affectedSubjects"`
	Diff             []FileDiff `json:"diff"`
	// After is the view rebuilt with the proposal applied.
	After *View `json:"after,omitempty"`
}

// EffectChange is one operation whose declared or effective effect
// changes.
type EffectChange struct {
	SubjectID       string `json:"subjectId"`
	ResourceID      string `json:"resourceId"`
	ResourceLabel   string `json:"resourceLabel"`
	OperationID     string `json:"operationId"`
	OperationLabel  string `json:"operationLabel"`
	DeclaredBefore  Effect `json:"declaredBefore"`
	DeclaredAfter   Effect `json:"declaredAfter"`
	EffectiveBefore Effect `json:"effectiveBefore"`
	EffectiveAfter  Effect `json:"effectiveAfter"`
	// Direction is judged on the effective effect and only between
	// decisions whose order is known: allowed over ask over denied, with
	// unavailable counted as denied. "widened" and "narrowed" are used
	// only when both sides are among those; "moved" when both sides are
	// known and equal in strength but different; "indeterminate" when
	// either side is default or unresolved, because the tool's default
	// and an unexpressed rule have no established order.
	Direction string `json:"direction"`
	// Reason explains an indeterminate direction.
	Reason string `json:"reason,omitempty"`
}

// ImpactSummary counts the changes.
type ImpactSummary struct {
	Changed       int `json:"changed"`
	Widened       int `json:"widened"`
	Narrowed      int `json:"narrowed"`
	Moved         int `json:"moved"`
	Indeterminate int `json:"indeterminate"`
}

// FileDiff is the exact before and after of one spec file.
type FileDiff struct {
	Path   string `json:"path"`
	Before string `json:"before"`
	After  string `json:"after"`
}
