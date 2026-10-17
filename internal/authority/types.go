// Package authority builds the Authority view: which subject (an agent
// or the tool session) may perform which exact action on which exact
// target, under what conditions, and why.
//
// The view is one coherent, versioned document. The server owns
// normalization, precedence, provenance, and impact analysis; a client
// owns rendering, selection, and motion. Every identifier here is
// stable and machine-readable, and every item also carries a
// human-readable label so no client has to decode an internal id.
//
// The JSON contract is documented in docs/internal/authority-view.md and
// mirrored in ui/src/contract.ts. Change all three together.
package authority

// SchemaVersion names the shape of View and ImpactResult. Bump it when a
// field changes meaning; adding an optional field keeps the version.
const SchemaVersion = "authority/v1"

// View is the complete initial state of the Authority view for one
// subject. It answers the acceptance questions directly: concrete
// resources, precise operations, effective effects, conditions,
// provenance, and enforcement, with every uncertainty listed.
type View struct {
	SchemaVersion string   `json:"schemaVersion"`
	Snapshot      Snapshot `json:"snapshot"`
	// Agent is the selected subject, fully resolved.
	Agent Subject `json:"agent"`
	// Agents lists every selectable subject, the selected one included.
	Agents    []SubjectSummary `json:"agents"`
	Domains   []Domain         `json:"domains"`
	Providers []Provider       `json:"providers"`
	Resources []Resource       `json:"resources"`
	// Edges are the permission circuit: subject → operation → resource.
	Edges   []Edge   `json:"edges"`
	Secrets []Secret `json:"secrets"`
	// Uncertainties are everything the view cannot determine. A client
	// shows them; it never presents an unknown as granted or denied.
	Uncertainties []Uncertainty `json:"uncertainties"`
	// SupportedActions are the mutations and queries a client may offer.
	SupportedActions []SupportedAction `json:"supportedActions"`
	// Runtime states that no runtime enforcement or observed use is
	// part of this view. It is always "not-observed" today.
	Runtime      RuntimeStatus `json:"runtime"`
	Explanations Explanations  `json:"explanations"`
}

// Snapshot identifies the configuration the view was built from.
type Snapshot struct {
	// Revision is a short content hash of every loaded spec. Two views
	// with the same revision were built from the same sources.
	Revision    string  `json:"revision"`
	GeneratedAt string  `json:"generatedAt"`
	Project     Project `json:"project"`
	// Targets are the coding tools the project syncs to, in config order.
	Targets []string `json:"targets"`
	// Specs lists every spec that contributed, for the JSON inspector.
	Specs []SpecRef `json:"specs"`
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

// EffectCounts counts operations by effect.
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
	// ResourceCoverage is "declared" when the spec names the resources
	// the provider reaches (MCP roots), "complete" for built-ins whose
	// resources are fully known from rules, or "unresolved" when the
	// credential decides and the view cannot see it.
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
	ResourceMCPServer      = "mcp-server"    // everything a server reaches, when no root is declared
	ResourceDeclaredRoot   = "declared-root" // one MCP root: a named repository, service, database
	ResourceFilePattern    = "file-pattern"  // files matching a rule pattern
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
	// Operations are every action the view knows about on this
	// resource, each with its effective effect.
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

// Effect is the effective decision for one operation.
type Effect string

const (
	EffectAllowed     Effect = "allowed"     // an allow rule wins
	EffectAsk         Effect = "ask"         // an ask rule wins: the tool prompts
	EffectDenied      Effect = "denied"      // a deny rule wins
	EffectUnavailable Effect = "unavailable" // the subject's own scope leaves the tool out
	EffectDefault     Effect = "default"     // no rule: the tool's own default applies
	EffectUnresolved  Effect = "unresolved"  // the view cannot decide
)

// Operation is one action on one resource, resolved for the subject.
type Operation struct {
	ID string `json:"id"`
	// Action is the neutral capability or MCP tool: "read", "edit",
	// "write", "delete", "run", "fetch", "search", or a tool name.
	Action string `json:"action"`
	Label  string `json:"label"`
	Effect Effect `json:"effect"`
	// Decided is the grant that won, when a rule decided the effect.
	Decided *Grant `json:"decided,omitempty"`
	// Grants are every declaration that applied, winner first.
	Grants      []Grant       `json:"grants"`
	Conditions  []string      `json:"conditions"`
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

// Enforcement states how one coding tool carries the decision.
type Enforcement struct {
	Target string `json:"target"`
	Label  string `json:"label"`
	// Status is "native-rule" (the tool's own permission rule),
	// "hook" (a generated hook), "not-expressed" (sync writes nothing
	// for it; the tool's defaults apply), or "not-applicable".
	Status string   `json:"status"`
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
	Effect      Effect `json:"effect"`
	Label       string `json:"label"`
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
	UncertaintyResourcesUnresolved = "provider-resources-unresolved"
	UncertaintyDomainDefaulted     = "domain-defaulted"
	UncertaintyTargetNotExpressed  = "target-not-expressed"
	UncertaintyToolListUnknown     = "tool-list-unknown"
	UncertaintyPatternApproximate  = "pattern-containment-approximate"
	UncertaintySecretReadUnknown   = "secret-read-unknown"
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

// RuntimeStatus states what the view knows about enforcement at run time.
type RuntimeStatus struct {
	// Enforcement is always "not-observed": the view reads declared
	// configuration and what sync generates from it, never a running tool.
	Enforcement string `json:"enforcement"`
	Note        string `json:"note"`
}

// Explanations is a glossary a client shows next to each effect, list,
// and enforcement status.
type Explanations struct {
	Effects     map[string]string `json:"effects"`
	Lists       map[string]string `json:"lists"`
	Enforcement map[string]string `json:"enforcement"`
	Rights      map[string]string `json:"rights"`
}

// Proposal is a client's proposed change to the project's permission
// lists. Changes apply in order to a copy of the specs; nothing is
// written.
type Proposal struct {
	AgentID string `json:"agentId"`
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

// ImpactResult is the server's authoritative impact of a Proposal.
type ImpactResult struct {
	SchemaVersion    string `json:"schemaVersion"`
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

// EffectChange is one operation whose effect changes.
type EffectChange struct {
	SubjectID      string `json:"subjectId"`
	ResourceID     string `json:"resourceId"`
	ResourceLabel  string `json:"resourceLabel"`
	OperationID    string `json:"operationId"`
	OperationLabel string `json:"operationLabel"`
	Before         Effect `json:"before"`
	After          Effect `json:"after"`
	// Direction is "widened", "narrowed", or "moved" (same strength,
	// different decision such as default → ask).
	Direction string `json:"direction"`
}

// ImpactSummary counts the changes.
type ImpactSummary struct {
	Changed  int `json:"changed"`
	Widened  int `json:"widened"`
	Narrowed int `json:"narrowed"`
	Moved    int `json:"moved"`
}

// FileDiff is the exact before and after of one spec file.
type FileDiff struct {
	Path   string `json:"path"`
	Before string `json:"before"`
	After  string `json:"after"`
}
