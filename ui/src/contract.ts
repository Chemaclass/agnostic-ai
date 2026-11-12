// Authority view contract, schema `authority/v1`, and the overview and
// overview contract, schema `overview/v1`.
//
// This file mirrors internal/authority/types.go and
// internal/overview/types.go field for field. The Go side is canonical:
// the server builds the views, the client renders them. Change both
// together, and docs/internal/authority-view.md with them.
//
// Four states are kept apart and never collapsed into one badge:
// declared (what the specs say), effective (what the selected coding
// tool does with it), installed (whether the generated file matches
// disk), and runtime (what a running tool enforced, never observed here).

export const SCHEMA_VERSION = 'authority/v1'
export const OVERVIEW_SCHEMA_VERSION = 'overview/v1'

export type SubjectKind = 'agent' | 'session'
export const SESSION_SUBJECT_ID = 'session'

export type Effect =
  | 'allowed'
  | 'ask'
  | 'denied'
  | 'unavailable'
  | 'default'
  | 'unresolved'

export type Resolution =
  | 'translated'
  | 'translated-unverified'
  | 'hook'
  | 'not-expressed'
  | 'agent-scope'
  | 'no-rule'
  | 'disabled'

export type InstalledStateValue =
  | 'in-sync'
  | 'stale'
  | 'not-generated'
  | 'not-checked'
  | 'not-applicable'

export type DomainId =
  | 'source-control'
  | 'cloud-deployment'
  | 'data-storage'
  | 'filesystem-execution'
  | 'web-browser'
  | 'communication'
  | 'secrets-identity'
  | 'ai-agent-ops'
  | (string & {})

export type DomainResolution = 'curated' | 'builtin' | 'default'
export type ProviderKind = 'mcp' | 'builtin'
export type ProviderStatus = 'configured' | 'disabled' | 'unavailable-to-agent'
export type ResourceCoverage = 'declared' | 'complete' | 'undeclared'
export type ResourceKind =
  | 'undeclared-scope'
  | 'declared-root'
  | 'file-pattern'
  | 'command-pattern'
  | 'web'
export type ResourceBasis = 'rule-pattern' | 'declared-root' | 'undeclared'
export type GrantList =
  | 'allow'
  | 'ask'
  | 'deny'
  | 'protected'
  | 'can'
  | 'tools'
  | 'readonly'
  | 'mcpServers'
export type GrantScope = 'project' | 'agent'
export type EnforcementStatus = 'native-rule' | 'hook' | 'not-expressed' | 'not-applicable'
export type SecretRight = 'allowed' | 'unavailable' | 'not-readable' | 'unknown'
export type Severity = 'info' | 'warning'
export type UncertaintyCode =
  | 'runtime-not-observed'
  | 'provider-scope-undeclared'
  | 'credential-reach-not-inspected'
  | 'domain-defaulted'
  | 'target-not-expressed'
  | 'tool-list-unknown'
  | 'pattern-containment-approximate'
  | 'secret-read-unknown'
  | 'alternate-route-not-modeled'
  | 'installed-not-checked'
  | 'root-not-filesystem'
  | 'target-matching-unverified'
  | (string & {})

export interface Source {
  path: string
  layer: string
  line?: number
}

export interface SpecRef {
  kind: string
  name: string
  path: string
  layer: string
}

export interface Project {
  root: string
  name: string
}

export interface Evidence {
  declared: 'loaded'
  installed: 'checked' | 'not-checked'
  runtime: 'not-observed'
  note: string
}

export interface Snapshot {
  revision: string
  generatedAt: string
  project: Project
  targets: string[]
  configFiles: string[]
  specs: SpecRef[]
  evidence: Evidence
}

export interface HarnessOption {
  target: string
  label: string
}

export interface ExecutionIdentity {
  kind: 'local-process'
  description: string
}

export interface InstalledFile {
  path: string
  state: InstalledStateValue
}

export interface InstalledState {
  state: InstalledStateValue
  files: InstalledFile[]
  checkedAt?: string
}

export interface Harness {
  target: string
  label: string
  available: HarnessOption[]
  identity: ExecutionIdentity
  installed: InstalledState
  permissionSupport: 'native-rules' | 'hooks-only' | 'none'
  note?: string
}

export interface SubjectSummary {
  id: string
  label: string
  description: string
  kind: SubjectKind
}

export interface ToolScope {
  mode: 'unrestricted' | 'listed'
  field?: 'can' | 'tools' | ''
  declared: string[]
  capabilities: string[]
}

export interface MCPScope {
  mode: 'all' | 'listed'
  names: string[]
}

export interface Subject extends SubjectSummary {
  source?: Source
  toolScope: ToolScope
  readonly: boolean
  mcpServers: MCPScope
}

export interface EffectCounts {
  allowed: number
  ask: number
  denied: number
  unavailable: number
  default: number
  unresolved: number
}

export interface Domain {
  id: DomainId
  label: string
  description: string
  examples: string[]
  order: number
  providerIds: string[]
  resourceIds: string[]
  summary: EffectCounts
}

export interface Transport {
  type: string
  endpoint?: string
}

export interface Provider {
  id: string
  label: string
  description: string
  kind: ProviderKind
  domainIds: DomainId[]
  domainResolution: DomainResolution
  status: ProviderStatus
  transport?: Transport
  source?: Source
  resourceIds: string[]
  secretIds: string[]
  resourceCoverage: ResourceCoverage
  coverageNote?: string
}

export interface Identity {
  uri?: string
  pattern?: string
  server?: string
}

export interface Matches {
  count: number
  samples: string[]
  truncated: boolean
}

export interface Grant {
  rule: string
  list: GrantList
  scope: GrantScope
  source: Source
  wins: boolean
  note?: string
}

export interface Enforcement {
  target: string
  label: string
  status: EnforcementStatus
  effect: Effect
  native?: string[]
  note?: string
}

export interface RouteStatus {
  route: string
  effect: Effect
  /** The route's own flag, from the operation it stands for; never borrow the inspected operation's. */
  hypothetical: boolean
  installed?: InstalledStateValue
  note?: string
}

/** Qualifies alternateRoutes: an empty list means no checked route is open, never that no route exists. */
export interface RouteCoverage {
  checked: RouteStatus[]
  notModeled: string[]
}

export interface Operation {
  id: string
  action: string
  label: string
  declared: Effect
  effect: Effect
  resolution: Resolution
  native?: string[]
  installed: InstalledStateValue
  /** True when the effect describes what the tool would do once synced (installed is stale, not-generated, or not-checked). Word it conditionally. */
  hypothetical: boolean
  runtime: 'not-observed'
  decided?: Grant
  grants: Grant[]
  conditions: string[]
  alternateRoutes: string[]
  routeCoverage: RouteCoverage
  enforcement: Enforcement[]
  explanation: string
}

export interface Resource {
  id: string
  label: string
  description?: string
  kind: ResourceKind
  providerId: string
  domainIds: DomainId[]
  identity: Identity
  basis: ResourceBasis
  caveat?: string
  operations: Operation[]
  matches?: Matches
  labelResolved: boolean
  unresolvedReason?: string
}

export interface Edge {
  id: string
  subjectId: string
  resourceId: string
  operationId: string
  effect: Effect
  declared: Effect
  label: string
}

export interface SecretRef {
  source: Source
  field: string
}

export interface SecretRights {
  discover: SecretRight
  readValue: SecretRight
  use: SecretRight
  manage: SecretRight
}

export interface Secret {
  id: string
  name: string
  label: string
  providerIds: string[]
  referencedBy: SecretRef[]
  rights: SecretRights
  explanation: string
}

export interface Uncertainty {
  id: string
  code: UncertaintyCode
  severity: Severity
  message: string
  relatedIds: string[]
}

export interface SupportedAction {
  id: string
  method?: string
  path?: string
  description: string
  status: 'available' | 'unsupported'
}

export interface Explanations {
  effects: Record<string, string>
  resolution: Record<string, string>
  lists: Record<string, string>
  enforcement: Record<string, string>
  installed: Record<string, string>
  rights: Record<string, string>
}

export interface AuthorityView {
  schemaVersion: string
  snapshot: Snapshot
  harness: Harness
  agent: Subject
  agents: SubjectSummary[]
  domains: Domain[]
  providers: Provider[]
  resources: Resource[]
  edges: Edge[]
  secrets: Secret[]
  uncertainties: Uncertainty[]
  supportedActions: SupportedAction[]
  explanations: Explanations
}

export interface Change {
  op: 'add' | 'remove'
  list: 'allow' | 'ask' | 'deny'
  rule: string
  source?: string
}

export interface Proposal {
  agentId: string
  target?: string
  revision?: string
  changes: Change[]
}

export interface EffectChange {
  subjectId: string
  resourceId: string
  resourceLabel: string
  operationId: string
  operationLabel: string
  declaredBefore: Effect
  declaredAfter: Effect
  effectiveBefore: Effect
  effectiveAfter: Effect
  /** Only known orders are ranked: allowed over ask over denied (unavailable counts as denied). Any default or unresolved side is indeterminate. */
  direction: 'widened' | 'narrowed' | 'moved' | 'indeterminate'
  reason?: string
}

export interface ImpactSummary {
  changed: number
  widened: number
  narrowed: number
  moved: number
  indeterminate: number
}

export interface FileDiff {
  path: string
  before: string
  after: string
}

export interface ImpactResult {
  schemaVersion: string
  status: 'proposed'
  target: string
  baseRevision: string
  proposedRevision: string
  valid: boolean
  problems: string[]
  changes: EffectChange[]
  summary: ImpactSummary
  affectedSubjects: string[]
  diff: FileDiff[]
  after?: AuthorityView
}

// ---------------------------------------------------------------------
// Overview, schema `overview/v1` (internal/overview/types.go).
// ---------------------------------------------------------------------

export type SectionId = 'context' | 'authority' | 'tools' | 'lifecycle' | 'settings'

export interface KindCount {
  kind: string
  label: string
  count: number
  local?: boolean
}

export interface KindSupport {
  kind: string
  label: string
  supported: string[]
  unsupported: string[]
}

/** The code of the finding that stands in for a lint run that stopped early. */
export const LINT_INCOMPLETE = 'lint-incomplete'

/** A read-only check result, shown on what it concerns. */
export interface ItemFinding {
  source: 'lint' | 'doctor'
  code?: string
  severity: string
  message: string
  path?: string
  line?: number
  /** Absolute path of the file it names, when that file is on the server's disk. */
  openPath?: string
  fix?: string
  explanation?: string
  /** `global` for a finding on the global setup in a project's overview; its path is relative to the global source. */
  layer?: 'global'
}

export interface SectionItem {
  kind: string
  name: string
  label: string
  description: string
  path: string
  /** Absolute path of a Markdown file, for opening it in an editor. */
  openPath?: string
  layer: string
  scope?: string
  /** For a hook, the id of the lifecycle stage it runs at. */
  group?: string
  /** What the item sets, key by key. */
  values: Value[]
  /** For a hook: its key fields, such as the command and what it matches. For an MCP item whose spec sets `disabled`: that one field. */
  fields?: Value[]
  /** For a built-in: whether the config turns it on. */
  enabled?: boolean
  /** For a built-in: the personal config turns it on or off against the shared list. */
  local?: boolean
  /** For a built-in: what turning it on does and gives you. */
  about?: BuiltinAbout
  /** For a personal settings file: the dotted keys it sets itself. */
  localKeys?: string[]
  /** For an MCP server: the tools `agnostic-ai mcp tools` last listed. */
  tools?: ToolListing
  findings: ItemFinding[]
}

export interface BuiltinAbout {
  summary: string
  points: string[]
  /** The official page of the tool the built-in wraps. */
  docsUrl?: string
  docsLabel?: string
}

export interface ToolListing {
  state: 'listed' | 'stale' | 'failed' | 'not-listed'
  listedAt?: string
  error?: string
  tools: { name: string; title?: string; description?: string }[]
  /** Refreshes the listing. */
  command: string
}

/** One setting: a list renders as chips, a bool as a switch, text as text. */
export interface Value {
  key: string
  type: 'text' | 'bool' | 'list'
  value?: string
  list?: string[]
}

/** One point in a session a hook can run at. */
export interface Stage {
  id: string
  label: string
  description: string
  /** The configured coding tools that run a hook here, with their own event name. */
  targets: { target: string; label: string; event: string }[]
}

export interface Section {
  id: SectionId
  label: string
  question: string
  order: number
  kinds: KindCount[]
  items: SectionItem[]
  support: KindSupport[]
  /** Findings that name no configured item. */
  findings: ItemFinding[]
  /** Lifecycle only: every stage, in session order, even when empty. */
  stages?: Stage[]
  /** Authority and Settings: every settings field, even when unset. */
  fields?: SettingField[]
  /** Tools only: MCP servers found in a tool's own config that no spec covers. Names and paths only, never a server's values. */
  native?: NativeServer[]
}

/** One MCP server name found in tools' own config files. `import` is absent when no import reads the file that holds it. */
export interface NativeServer {
  name: string
  holders: NativeHolder[]
  import?: string
}

/** One tool config file that defines a native server. */
export interface NativeHolder {
  target: string
  label: string
  path: string
  scope: 'project' | 'user'
  transport?: 'stdio' | 'http' | 'sse' | 'streamable-http' | 'remote'
}

export interface MCPSpecPreview {
  file: string
  spec: string
  variables: string[]
  targets: string[]
  syncCommand: string
  expected: string
}

export interface HookSpecRequest {
  action: 'preview' | 'apply'
  audience: 'shared' | 'personal'
  name: string
  on: string
  match?: string
  command: string
  timeout?: number
  description?: string
  expected?: string
}

export interface HookSpecPreview {
  file: string
  spec: string
  syncCommand: string
  expected: string
}

export type SpecLevel = 'project' | 'global'

export interface SpecMoveRequest {
  action: 'preview' | 'apply'
  kind: string
  name: string
  from: SpecLevel
  to: SpecLevel
  mode: 'copy' | 'move'
  overwrite: boolean
  expected?: string
}

export interface SpecMovePreview {
  source: string
  destination: string
  /** The destination already holds this spec; apply needs overwrite. */
  exists: boolean
  /** The files written, relative to the spec's file or skill folder. */
  files: string[]
  syncCommands: string[]
  expected: string
  /** Set on an apply that worked but left a leftover folder to delete by hand. */
  warning?: string
}

export type PermissionKey = 'permissions.allow' | 'permissions.ask' | 'permissions.deny'

export interface PermissionEditRequest {
  action: 'preview' | 'apply'
  audience: 'shared' | 'personal'
  path?: string
  key: PermissionKey
  rules: string[] | null
  before: string[] | null
  expected?: string
}

export interface PermissionEditPreview {
  schemaVersion: 'permission-edit/v1'
  file: string
  key: PermissionKey
  audience: 'shared' | 'personal'
  before: string[] | null
  rules: string[] | null
  diff: string
  expected: string
  syncCommand: string
}

/** A settings spec's protected block as written; an empty decision or reason is one the file leaves out. */
export interface ProtectedBlock {
  paths: string[]
  decision: string
  reason: string
}

export interface ProtectedEditRequest {
  action: 'preview' | 'apply'
  audience: 'shared' | 'personal'
  path?: string
  protected: ProtectedBlock | null
  before: ProtectedBlock | null
  /** For a local edit, the project block the draft started from, or null when there was none. */
  inherited?: ProtectedBlock | null
  expected?: string
}

export interface ProtectedEditPreview {
  schemaVersion: 'protected-edit/v1'
  file: string
  audience: 'shared' | 'personal'
  before: ProtectedBlock | null
  protected: ProtectedBlock | null
  /** The file writes `protected: null`, which turns the inherited paths off. */
  cleared?: boolean
  diff: string
  expected: string
  syncCommand: string
}

/** One field a settings spec can set, with what each spec sets it to. */
export interface SettingField {
  key: string
  label: string
  description: string
  /** Each spec that sets the field; `local` marks a personal file Git ignores. */
  set: { spec: string; path: string; values: Value[]; local?: boolean }[]
  /** `localPath` is the personal file a personal edit of `path` writes. `toolAllowed` lists the values one tool accepts, for tools with a fixed list. */
  editable?: { path?: string; paths?: string[]; sources?: { path: string; spec: string; localPath: string; disabledReason?: string }[]; kind: 'text' | 'number' | 'boolean' | 'permission-list' | 'protected-list'; allowed?: string[]; toolAllowed?: Record<string, string[]>; disabledReason?: string; syncCommand?: string; localPath?: string }
  /** For model and effort: the value sync writes for each configured tool, and why. */
  resolved?: TargetValue[]
}

/** Where a value comes from: a settings spec, or a config key when `spec` is absent. `reason` says why a losing value does not win. */
export interface ValueOrigin {
  spec?: string
  path: string
  value?: string
  tier?: string
  local?: boolean
  /** The value is the tool's own entry in the per-tool map, such as `model.codex`, not the value for every tool. */
  perTool?: boolean
  reason?: string
}

/**
 * The value sync writes for one tool. An absent `value` means sync writes none and the tool keeps a default agnostic-ai does not know.
 * `dropped` is the sync note when the tool's files have no place for the value; `checked` is false when sync did not render the tool.
 */
export interface TargetValue {
  target: string
  label: string
  value?: string
  alias?: string
  tier?: string
  from?: ValueOrigin
  dropped?: string
  checked: boolean
  others: ValueOrigin[]
  /** Specs that also set `x-<target>.model`, a native key sync writes as is. */
  native?: ValueOrigin[]
}

export type SyncState = 'in-sync' | 'drifted' | 'not-checked'

export interface TargetSync {
  state: SyncState
  files: { path: string; state: string }[]
}

export interface TargetSupport {
  target: string
  label: string
  kinds: string[]
  sync: TargetSync
}

export interface ToolChoice { target: string; label: string; description: string; enabled: boolean; local: boolean; detected: boolean }
export type ToolCatalogSource = 'default' | 'shared' | 'personal'
export interface ToolCatalog { source: ToolCatalogSource; items: ToolChoice[] }

export interface OverviewView {
  schemaVersion: string
  snapshot: Snapshot
  sections: Section[]
  targets: TargetSupport[]
  tools: ToolCatalog
  health: Health
  /** True for user configuration, which has no project to size. */
  global?: boolean
  /** The project's GitHub repository, read from its origin remote. Absent for user configuration and other hosts. */
  repository?: Repository
}

export interface Repository {
  /** The repository's https page, with no credentials. */
  url: string
  /** owner/repo */
  name: string
}

/** What the page suggests beyond the lint and doctor findings. */
export interface Health {
  gaps: Gap[]
  /** What the gap check considered; each improvement not applied is a gap. Absent for user configuration. */
  improvements?: Improvements
}

export interface Improvements {
  applied: number
  total: number
}

/** Something the configured tools support that the project does not use yet. A suggestion, never a finding. */
export interface Gap {
  /** `kind:<kind>`, `stage:<stage>` or `builtin:<name>`. */
  id: string
  section: string
  label: string
  /** The configured targets that would use it. */
  supported: string[]
  /** What the project lacks, such as "no skill yet". */
  missing: string
  /** A prompt a coding agent can run to fill the gap. */
  fix: string
}

// ---------------------------------------------------------------------
// Context size, schema `1`: the `explain --context` report for one target.
// Words and bytes measure planned instruction text, not model tokens.
// ---------------------------------------------------------------------

export const CONTEXT_SCHEMA_VERSION = '1'

export type ContextLoad = 'startup' | 'file-scope' | 'on-demand'

export interface ContextSize {
  words: number
  bytes: number
}

export interface ContextContribution extends ContextSize {
  /** Project-relative path with forward slashes. */
  source: string
  category: string
  load: ContextLoad
}

export interface ContextOpportunity {
  kind: string
  target: string
  load: ContextLoad
  passage: string
  copies: number
  /** One copy. */
  size: ContextSize
  /** (copies - 1) * size. */
  reduction: ContextSize
  sources: { source: string; category: string }[]
  suggestion: string
}

export interface ContextReport {
  version: string
  command: string
  target: string
  /** The project-relative file the report counted file-scoped rules for, when one was given. */
  file?: string
  note: string
  startup: ContextSize
  file_scope: ContextSize
  on_demand: ContextSize
  /** Startup first, then file-scope, then on-demand; largest first within each. */
  contributions: ContextContribution[]
  /** Largest reduction first. */
  opportunities: ContextOpportunity[]
}

export interface ContextView {
  schemaVersion: string
  /** False for user configuration (`serve --global`), with the reason. */
  supported: boolean
  reason: string
  root?: string
  targets: string[]
  /** Targets that accept a file, so scoped rules can count as file-scoped. */
  fileTargets: string[]
  target: string
  /** lint.instructions-words, the startup words LINT011 allows; absent for user configuration or an older server. */
  startupWordBudget?: number
  report?: ContextReport
}

// ── Conversations ───────────────────────────────────────────────────
// Recent chats from the coding tools' own stores, newest first. The
// server reads them; the page never sees a transcript, only a title,
// the opening lines, the latest prompt and the agent's latest reply,
// each withheld when it may hold a credential. Mirrors internal/conversations/types.go.

export type ConversationTool = 'claude' | 'codex' | 'cursor'

/** Where the chat lives: a terminal, an editor extension or panel, a desktop app, a background job, or the tool's cloud, such as Claude Code at claude.ai/code or a Codex cloud thread (listed only by a server started with --cloud-sessions). Runs a program started (SDK sessions, Codex exec) are not listed. */
export type ConversationApp = 'cli' | 'ide' | 'desktop' | 'background' | 'cloud'

/** What an open conversation is doing, when its tool says: the agent is working, it waits for the person, or it sits idle. */
export type ConversationActivity = 'working' | 'waiting' | 'idle'

/** Where a conversation stands for the person. The first that holds wins: `working` (open, and its tool says the agent is busy), `blocked` (open, its tool says it waits for the person, and the person has not looked at it since its last write), `completed` (marked completed, and since then nothing written and never reported working), `unread` (written after the person last saw it), `active` (open, or written in the last 24 hours), `dormant` (closed and quiet for longer). */
export type ConversationState = 'blocked' | 'working' | 'completed' | 'unread' | 'active' | 'dormant'

export interface ConversationSource {
  tool: ConversationTool
  label: string
  /** False when the tool's store is missing or unreadable; note says why. */
  found: boolean
  /** Conversations the store holds after hiding subagent, review and scripted runs. */
  count: number
  /** How many of them this page holds. */
  shown: number
  note?: string
}

/** How the server reopens a conversation. `terminal` runs `command` in a new terminal window, `focus` switches to the terminal tab that already runs it, `link` opens `link` in its app. */
export interface ConversationRoute {
  kind: 'focus' | 'terminal' | 'link'
  /** Button text, such as "Open in Terminal" or "Open in Cursor". */
  label: string
  /** The shell command that resumes it; set for terminal and focus routes. A focus route's command only resumes a session that ended since the scan, so rows read it through `rowCommand`, which offers none on a focus route. */
  command?: string
  link?: string
}

export interface Conversation {
  tool: ConversationTool
  app: ConversationApp
  id: string
  title: string
  folder?: string
  branch?: string
  startedAt?: string
  /** When the person or the agent last wrote in it; the list is newest first. */
  updatedAt: string
  /** Up to three lines from the start of the first prompt. */
  opening?: string[]
  /** The latest prompt typed into the conversation. */
  prompt?: string
  /** The agent's latest message since the latest prompt, as one line: its question, or the start of the message. Claude Code and Codex conversations on this machine only. */
  reply?: string
  /** Fields left out because they may hold a credential. */
  withheld?: Array<'title' | 'opening' | 'prompt' | 'reply'>
  /** A process or editor window has it open right now. */
  live?: boolean
  /** Set for a live conversation whose tool reports it. */
  activity?: ConversationActivity
  /** Set with a `waiting` activity that the server read from silence, not from the tool: when the tool last wrote. Local Codex never says it waits, so a turn that wrote nothing for five minutes is taken to wait. Absent for a wait the tool reported. */
  quietSince?: string
  /** Absent from a server older than the marks. */
  state?: ConversationState
  /** When the person last saw it: opening it through the server, or a `seen` or `completed` mark. */
  seenAt?: string
  /** When the person marked it completed. The server clears it once something is written after it or its tool reports it working. */
  completedAt?: string
  /** Set only on a `completed` or `dormant` row: why this conversation cannot be deleted from here, such as one stored in its tool's cloud. Absent on an archived local Claude Code session, which can be. */
  deleteUnsupported?: string
  /** The conversation this one continues, read from the line `Continues <tool>:<id>` that starts its first prompt. That line is cut from the title, opening and prompt. The parent may be on no page: hidden, older, filtered out or deleted. */
  parent?: { tool: ConversationTool; id: string }
  /** Set on a live local Claude Code session that Remote Control also shows on claude.ai: that cloud session's id and its page. Its cloud row is not listed apart. Open its page with `POST /api/conversations/open` and the cloud id. */
  cloud?: { id: string; link: string }
  open: ConversationRoute
}

/** One page of the list. The first page holds every open conversation, every blocked or unread one, and the past week; each later page, asked for with `before`, the week before it, reaching past weeks with no conversation. */
export interface ConversationsView {
  schemaVersion: 'conversations/v1'
  generatedAt: string
  /** The page covers conversations last written from `from` up to `to`, plus, on the first page, every open, blocked or unread one. */
  from: string
  to: string
  /** The `before` value of the next page; absent when no older conversation remains. */
  next?: string
  /** How many conversations of any age the search matched, across all pages; set only for a request with `q`. */
  matches?: number
  /** How many conversations are `blocked` or `unread`, across every age and tool the server lists, whatever the page or the search. */
  needsYou?: number
  /** Why the marks file could not be read or written; rows then carry no marks. */
  marksNote?: string
  /** Why a tool's cloud conversations are missing, on a server started with --cloud-sessions; rows then hold none of them. */
  cloudNote?: string
  /** count is every conversation the tool holds; shown is how many this page holds. */
  tools: ConversationSource[]
  conversations: Conversation[]
}

export interface ConversationOpenRequest {
  tool: ConversationTool
  id: string
  /** The editor chosen in Settings, so editor links use its scheme. */
  editor?: string
}

/** A mark on one conversation, kept in a file on this machine: `seen` (read up to now), `completed` (done until something new is written or it starts working; also marks it seen), `open` (takes a completion back) or `hidden` (out of every list and search until something new is written or it starts working; also marks it seen). */
export interface ConversationMarkRequest {
  tool: ConversationTool
  id: string
  as: 'seen' | 'completed' | 'open' | 'hidden'
}

/** Deletes one archived conversation from this machine for good. Only a `completed` local Claude Code session that no process holds can be deleted. */
export interface ConversationDeleteRequest {
  tool: ConversationTool
  id: string
}

/** Sent with 200 only once the transcript is gone. freedBytes is what its files held; note says what could not be tidied after the data went, such as its entry in the marks file. */
export interface ConversationDeleteReply {
  deleted: true
  freedBytes?: number
  note?: string
}

/** A refused delete: 409 for `not-archived`, `running` or `changed`, 422 for `unsupported`, 500 for `failed` with the paths already removed and those still on disk. error is a complete sentence. */
export interface ConversationDeleteRefusal {
  error: string
  reason: 'not-archived' | 'running' | 'changed' | 'unsupported' | 'failed'
  removed?: string[]
  remaining?: string[]
}

/** opened is false when the server could not open it here; message says why and command, when set, resumes it by hand; a Claude Code, Codex or cursor-agent session that still runs in a terminal gets none. */
export interface ConversationOpenReply {
  opened: boolean
  message: string
  command?: string
}

// HTTP API served by `agnostic-ai serve`.
//
//   GET  /api/views/authority?agentId=<id>&target=<tool>  → AuthorityView
//   POST /api/views/authority/impact                      → ImpactResult (body: Proposal)
//   GET  /api/views/authority/schema                      → { schemaVersion, endpoints }
//   GET  /api/views/overview                              → OverviewView
//   GET  /api/views/context?target=<tool>&file=<path>     → ContextView
//   GET  /api/views/source?path=<openPath>               → SourceView; 404 for a file the configuration does not list
//   GET  /api/views/conversations?editor=<id>&before=<time>&q=<words>&tool=<tool> → ConversationsView, one week per page; q searches every conversation; a project server lists the project's own conversations; the reply's ETag sent back as If-None-Match answers 304 while the list holds
//   POST /api/conversations/open                          → ConversationOpenReply (body: ConversationOpenRequest); an opened conversation is marked seen and its completion taken back
//   POST /api/conversations/mark                          → 204 (body: ConversationMarkRequest)
//   POST /api/conversations/delete                        → ConversationDeleteReply (body: ConversationDeleteRequest); 404 when not listed, else ConversationDeleteRefusal
//   GET  /api/health                                      → { ok, root, global, ui, projectRoot?, version?, requires?, requiresMet? }
//   POST /api/switch                                      → SwitchReply (body: {}); 409 { missing, root } when there is no global setup
//   POST /api/global/create                               → GlobalCreateReply (body: { git })
//   POST /api/edit/spec/move                              → SpecMovePreview (body: SpecMoveRequest); 409 { missing, root } when there is no global setup
export const API = {
  view: '/api/views/authority',
  impact: '/api/views/authority/impact',
  schema: '/api/views/authority/schema',
  overview: '/api/views/overview',
  context: '/api/views/context',
  source: '/api/views/source',
  editBuiltin: '/api/edit/builtin',
  editTargets: '/api/edit/targets',
  editValue: '/api/edit/value',
  editField: '/api/edit/field',
  editPermissions: '/api/edit/permissions',
  editProtected: '/api/edit/protected',
  editMCP: '/api/edit/mcp',
  editHook: '/api/edit/hook',
  editSpecMove: '/api/edit/spec/move',
  conversations: '/api/views/conversations',
  conversationOpen: '/api/conversations/open',
  health: '/api/health',
  switch: '/api/switch',
  globalCreate: '/api/global/create',
} as const

/** One file a row lists, as GET /api/views/source returns it. */
export interface SourceView {
  path: string
  kind: string
  /** Frontmatter keys in file order, each value as text. */
  frontmatter: { key: string; value: string }[]
  body: string
  /** The file size; the body stops at 256 KB when truncated is set. */
  bytes: number
  truncated: boolean
  /** The text looks like it holds a credential, so it is not sent. */
  withheld?: boolean
}

/** The global setup folder the page just created. */
export interface GlobalCreateReply {
  root: string
  created: string[]
  git: string
}

/** The server for the other configuration, found running or just started. */
export interface SwitchReply {
  url: string
  root: string
  global: boolean
  started: boolean
}
