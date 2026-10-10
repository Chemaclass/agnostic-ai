# Issue #1959: live Claude Code hook evidence

Claude Code 2.1.295 applied RTK 0.51.0's native rewrite, but the original approval requirement did not survive one exact-rule combination: `ask: Bash(git status)` plus an explicit CLI allow for `Bash(rtk git status)` denied the raw command and executed the rewritten one. No broad permission grant was used. With no rules, the direction reversed: plain `git status` ran, while its RTK spelling required approval.

This is live host evidence, not only a synthetic processor probe. Twenty-one original live cases cover the paired permission matrix, missing/unsupported/already-prefixed commands, duplicate registration, failure details, and execution sentinels. A six-call dedicated-profile suite repeated the critical permission pairs and proved actual user/project hook loading and removal. The final eight-call suite repeats those cases with explicit RTK database and tee paths and adds a paired automatically rewritten command failure. Its results are authoritative for storage-isolated permission, scope, and rewritten-failure claims. Claude CLI list-price estimates were **$0.01499958** for the original phase, including its recorded exploratory runs, **$0.00188751** for the first dedicated-profile suite, and **$0.00246653** for the final eight-call suite, including **$0.00062830** for the added failure pair. Exploratory isolation probes are outside the public matrices and have individual costs where recorded; there is no complete combined historical ledger. These are CLI list-cost estimates, not invoices. The final runner enforces its $3 output-root budget prospectively.

## Environment and method

- Read [issue #1959](https://github.com/Chemaclass/agnostic-ai/issues/1959), which had no comments at investigation time.
- macOS, Claude Code `2.1.295`, RTK `0.51.0`, model pinned to `claude-haiku-5-5`, effort low. Result metadata reports provider `firstParty` and cost basis `list`.
- One dedicated project per case under `/tmp/agnostic-ai-1959-live/`. Commands and hooks were sequential across provider calls. No global configuration edits, provider changes, credential copies, GitHub mutations, builds, or full test runs. The durable changes are this report, the runner, and redacted results.
- Startup metadata listed four enabled plugins despite project-only settings; skills and slash commands were empty. Dedicated host profiles reduced this to three plugins with built-in-marked paths. Captured critical-case hook events contained only the fixture PreToolUse and post-tool handlers. Plugin identities are redacted. The dedicated-profile results exclude the personal plugin but retain this fixed bundled inventory; a zero-plugin environment is not claimed.
- Native configuration selection: `--setting-sources project`. Existing authentication remained in place. `--strict-mcp-config --mcp-config '{"mcpServers":{}}'`, `--disable-slash-commands`, custom short system prompt, and `--tools Bash` restricted the fixture surface.
- `--permission-mode manual --permission-prompts none` exercised the actual host's noninteractive denial behavior. Hook payloads identify this mode as `default`. There was no interactive human approval dialog.
- Each process used `--max-budget-usd 0.12 --max-turns 3 --no-session-persistence --output-format stream-json --verbose --include-hook-events`.
- Temporary project allow rules generate an untrusted-workspace warning. The corrected explicit-allow cases also supply the exact original command through `--allowedTools`. The fixture settings are explicitly passed with `--settings`; no global trust entry is changed. There is no `Bash(rtk *)` grant.
- RTK reads Claude permission files independently of Claude's setting-source selection. In the original phase, only the RTK hook subprocess receives `CLAUDE_CONFIG_DIR` pointing to an empty temporary profile. This isolates RTK's reads of Claude permission files. On macOS, RTK 0.51.0 ignores the original XDG overrides for its own configuration and data; the historical runs could read user RTK configuration and write user tracking, recall, or tee state. The final suite sets `RTK_DB_PATH`, `RTK_RECALL_DB`, and `RTK_TEE_DIR` in both hook and host environments, and retains the read-only inherited RTK configuration. RTK configuration itself is not redirected.

The pinned model name is documented in [Claude model configuration](https://code.claude.com/docs/en/model-config#haiku-5-5-context-window-and-pricing). Current [hook documentation](https://code.claude.com/docs/en/hooks#pretooluse-decision-control) says input replacement changes the object used for permission evaluation and must preserve other fields. Current [CLI documentation](https://code.claude.com/docs/en/cli-reference) documents setting-source selection and explicit settings. RTK's [versioned platform directory code](https://github.com/rtk-ai/rtk/blob/v0.51.0/src/core/user_dirs.rs) explains the macOS directory behavior.

## Observed permission matrix

Every row requested the exact command `git status` once. RTK rows use the native processor through a logging wrapper; raw rows log without changing the request.

| Policy | Raw command | With RTK native hook |
|---|---|---|
| Exact original-command allow | Executed `git status` | Hook returned allow and replacement; executed `rtk git status` |
| Exact original-command ask | Denied; command not executed | Rewritten to `rtk git status`, then denied; command not executed |
| Exact original-command deny | Denied; command not executed | RTK returned no replacement; host denied original command |
| No configured rule | Executed `git status` | Rewritten to `rtk git status`, then denied because approval was required |
| Exact original ask plus exact rewritten allow | Denied; command not executed | Executed `rtk git status`, sentinel written |
| Exact original deny plus exact rewritten allow | Denied; command not executed | RTK returned no replacement; denied, no sentinel |

The conflict pair used identical original ask and narrow rewritten allow policies. The native RTK reply replaced the input but omitted `permissionDecision`; the host allowed the rewritten spelling. The original ask requirement therefore did not stop execution in this tested configuration. Raw control remained denied and left its sentinel absent. The corresponding original-deny/exact-wrapper-allow pair denied both forms: RTK preserved the original command and neither sentinel appeared. This is a concrete configuration limit to document before claiming preserved approval semantics. It follows the host's documented evaluation of replacement input; it does not by itself establish a host bug. Interactive approval dialogs, other permission modes, and other hosts were not exercised.

RTK's [versioned native processor](https://github.com/rtk-ai/rtk/blob/v0.51.0/src/hooks/hook_cmd.rs#L632) maps an ask rewrite to a replacement without a permission decision. The live case demonstrates the resulting interaction with Claude's permission evaluation; it does not establish behavior for other RTK versions.

## Evidence that the replacement actually ran

For `v2_allow_rtk`, the input event contains `git status`, `timeout: 10000`, and `description: fixture-metadata-1959`. RTK returns `updatedInput.command: rtk git status` with both other fields intact. A temporary PATH shim records the actual RTK executable arguments as `["git", "status"]`; `PostToolUse` names the rewritten command. RTK then invokes Git and produces filtered output.

The positive sentinel case writes `executed-command.sentinel` only when the Git shim receives exactly `["status"]`. Both denied sentinel cases leave that file absent and record no such invocation. Claude also performs its own Git metadata checks with different arguments; the audit distinguishes those from the requested command.

## Other outcomes

- **Missing RTK:** the hook's availability guard was actually run with a PATH excluding RTK. It returned empty success; Claude executed original `git status`. The host's command environment still had its normal fixture tools. This tests a missing hook dependency without uninstalling anything.
- **Unsupported command:** `printf fixture-unsupported-1959` received no replacement and executed once.
- **Already prefixed:** `rtk git status` received no replacement and executed once through RTK.
- **Malformed payload:** separate local processor probes returned exit 0, empty stdout, and a JSON parse diagnostic on stderr. Empty input and unsupported input returned empty success. These were processor-only cases; no live host was made to emit malformed JSON.
- **Automatically rewritten failure:** both final cases requested `cargo test --test fixture`, with an exact allow rule for that original command. The raw case executed it unchanged. The native RTK hook returned `rtk cargo test --test fixture`, preserving `timeout: 10000` and `description: fixture-metadata-1959`. `PostToolUseFailure` named the rewritten command and retained `Exit code 7`, `tests/payment.rs:42`, `expected 200, got 503`, and `2 passed; 1 failed`. A fake Cargo executable recorded exactly one invocation with `["test", "--test", "fixture"]` in each case; no real Cargo command or build ran. The RTK PATH shim also recorded `["cargo", "test", "--test", "fixture"]`. Historical script failures tested an already-prefixed wrapper; the final pair closes the automatic-rewrite gap. Overall Claude CLI exit 0 means the session completed, not that its shell command succeeded.
- **Duplicate registration:** a project entry plus an additional temporary `--settings` profile used different registration command strings. Both hooks received the original `git status` request and called RTK. The host executed the final RTK command once. This proves duplicate processor invocation, not double command execution or sequential compression. The extra settings profile simulates another owner. The later authenticated profile separately proves actual user and project scopes, as described below.

Claude's [hook configuration reference](https://code.claude.com/docs/en/hooks#configuration) documents parallel hook execution and deduplication for identical handlers across settings files. These different handlers were not deduplicated. A project integration should therefore identify one owner rather than assume equivalent RTK registrations collapse.

## Requirement audit

| Issue criterion | Result | Evidence |
|---|---|---|
| Eligible command rewritten once and executed through RTK | Passed for one normal registration | `v2_allow_rtk/{hooks,hook-replies,executed,post}.jsonl` |
| Explicit allow, ask, deny, and unconfigured outcomes | Recorded in noninteractive manual mode; original ask weakened by exact rewritten allow | `v2_{allow,ask,deny,none}_{raw,rtk}/`, `v4_*` |
| No broad RTK grant | Passed | Exact CLI arguments and settings in each case |
| Denied command not executed; failure not confused with success | Passed | Final permission pairs and `automatic_rewrite_failure_cases` |
| Metadata, status, and essential error preserved | Passed through an automatic native rewrite and a failing subprocess | Final `automatic_failure_{raw,rtk}` records: pre-hook input, native reply, actual argv, and PostToolUseFailure |
| Missing, malformed, unsupported, already-prefixed outcomes | Passed at named layers | `v2_missing`, `processor-only.json`, `v2_unsupported`, `v2_already_rtk` |
| Project/global duplicate registrations | Passed in a dedicated authenticated profile: actual user and project PreToolUse both ran; removing only user registration left project active | `authenticated_profile_cases.actual_global`, `actual_pretooluse_scopes` in the redacted matrix |
| Explain approval changes | Passed | Unconfigured raw allowed, rewritten form denied; original ask plus rewritten allow executed only after rewrite |

Do not claim universal approval parity or tested interactive behavior. The narrowly supported statement is: **Claude Code 2.1.295 on macOS accepted RTK 0.51.0 native rewrites, retained tested metadata and failures, and evaluated the rewritten spelling through its permission flow.**

## Artifacts and reproduction

The durable runner is [trio-live-hooks.py](trio-live-hooks.py). [Final results](trio-live-hooks-final-results.json) contain the eight calls with explicit RTK stores. The separate [historical matrix](trio-live-hooks-results.json) preserves earlier evidence and its original settings; it does not prove RTK storage isolation. Full local artifacts remain under `/tmp/agnostic-ai-1959-live/`:

- `runner.py`: creates a temporary project and runs one named case. It uses `stdin=DEVNULL` and `--` before the positional prompt, and refuses to reuse an existing case directory.
- `batch.py`: fourteen paired and edge-case runs.
- `sentinels.py`: three execution-sentinel runs.
- `ask-conflict.py`: paired original-ask/exact-wrapper-allow runs.
- `deny-conflict.py`: paired original-deny/exact-wrapper-allow runs.
- `analyze.py`, `matrix.json`: observed results and raw usage buckets.
- Each case has `invocation.json`, `.claude/settings.json`, `events.jsonl`, `summary.json`, hook requests/replies, subprocess argument logs, and post-tool events where applicable. The committed matrices and exported event streams redact personal home paths and plugin identities. Local runnable invocation records, hook payloads, and post-tool records remain private and can contain absolute home paths; they are not sanitized exports.
- `processor-only.json`: malformed, empty, and unsupported processor inputs, also retained under `processor_only` in the durable historical matrix.

For one fresh case, run `TRIO_LIVE_ROOT=/tmp/trio-hooks-repeat python3 trio-live-hooks.py allow rtk allow 'git status'`. To repeat the dedicated-profile suite, supply existing authentication privately through the documented `CLAUDE_CODE_OAUTH_TOKEN` child environment, set a fresh `TRIO_LIVE_ROOT`, and run `python3 trio-live-hooks.py --isolated-suite`. The runner creates actual user and project settings inside temporary directories; it does not fetch credentials or modify the user's settings. It refuses to reuse case directories and reserves the $0.12 call maximum in a persistent ledger before each provider process starts, including the removal replay. A final reported cost settles the reservation; any missing final cost leaves the reservation pending and blocks another call. A timeout terminates the process group, including child commands. The $3 ledger applies to one output root; keep the combined budget in view when starting another root. The optional `TRIO_CLAUDE_BIN` and `TRIO_RTK_BIN` variables select installed executables. Review the model and budget before repeating; no provider request is needed to inspect the recorded matrix.

The first exploratory batch inherited a Python here-document on stdin, and variadic `--allowedTools` consumed a positional prompt. Those exploratory cases are retained under names without `v2_`, `v3_`, `v4_`, or `v5_` and excluded from every acceptance conclusion. Their costs remain included. The corrected runner explicitly closes stdin and separates the prompt. Two earlier single-case runs produced valid preliminary rewrite evidence, but the corrected matrix is the authoritative dataset.

## Follow-up isolation and actual scopes

A dedicated `CLAUDE_CONFIG_DIR` with no copied credentials reported `loggedIn: false`, so it could not reuse the existing authentication automatically. However, `claude --init-only --setting-sources user,project` invoked real user and project `Setup` handlers in that profile without a provider request. Removing the isolated user registration and repeating invoked only the project handler. This proves actual scope loading and removal for `Setup`; it does not by itself prove global `PreToolUse` behavior.

The later suite authenticated through the documented `CLAUDE_CODE_OAUTH_TOKEN` child environment, using the existing account. A private launcher read the existing credential into memory and supplied it only to the child process. No token was written to a file, command argument, report, or terminal. The runner never fetches credentials. Each case used a dedicated `CLAUDE_CONFIG_DIR`, actual `user,project` sources, and zero exposed skills.

The four critical permission repeats produced the same result: original ask plus exact rewritten allow denied the raw request but executed the RTK replacement; original deny plus exact rewritten allow denied both. Startup metadata had the same three enabled plugins in all four cases. A follow-up probe confirmed all three had `path: builtin` and a source ending in `@builtin`, outside both the user's home and the fixture. A separate probe with the documented `CLAUDE_CODE_DISABLE_POLICY_SKILLS=1` still listed those three. They were bundled host inventory, not evidence of personal settings leaking into the profile. A zero-plugin host was not established, and skill activation was disabled for every critical case. [Claude's environment reference](https://code.claude.com/docs/en/env-vars) documents both the OAuth environment variable and the managed-skill flag.

The actual scope case registered one `PreToolUse` entry in the isolated profile's `settings.json` and one in the project's `.claude/settings.json`. Their distinct logging commands received the original `git status`; RTK executed once. The runner then replaced only the isolated user settings with `{}` and repeated the same invocation. Only the project hook ran, and RTK still executed successfully. This is a real user/project scope and removal test without modifying the user's real global configuration. The same three bundled plugins and zero skills remained present after removal.

A separate `--restricted` pilot retained Bash when explicitly named through `--tools Bash` and reproduced the ask-rule result, but it also retained plugins. Neither that flag nor disabled plugin entries in project settings provided the isolation needed by itself. Dedicated-profile results are recorded separately from the initial project-only dataset.

Private historical dedicated-profile artifacts remain under `/tmp/agnostic-ai-1959-isolated/`; final artifacts are under `/tmp/agnostic-ai-1959-final/`. The final matrix includes five permission/scope case records, the removal replay, and two failing-command records, with usage, explicit store paths, and the settled cost ledger. Scope execution evidence records one Git invocation before user-hook removal and one after, separately. The rewritten-command cases created tracking databases in their fixture directories. Removing the temporary user registration was the only configuration change between the two scope calls. The final eight-call CLI list-price estimate is $0.00246653. To repeat the failing-command pair in new case directories, use `TRIO_ISOLATED_PROFILE=1` and run `trio-live-hooks.py failure-raw raw allow 'cargo test --test fixture'`, then `trio-live-hooks.py failure-rtk rtk allow 'cargo test --test fixture'` with the same private authentication setup and output-root ledger.

## Configuration finding

Claude documents that `updatedInput` changes the input evaluated by its permission rules. The observed original-ask/exact-wrapper-allow result follows that contract: the replacement matches the allow rule, while the original spelling matched ask. RTK 0.51.0's native processor leaves the decision unspecified for this ask rewrite. The paired original-deny test remained denied because RTK declined the rewrite.

Keep approval rules aligned with the commands that actually run. Do not add automatic wrapper allow rules while claiming they preserve an original ask requirement. This finding needs documentation and a regression fixture for the integration; it does not justify an agnostic-ai permission override, a broad wrapper allowlist, or a new upstream bug report without evidence that RTK promises a stronger contract. [Claude's PreToolUse reference](https://code.claude.com/docs/en/hooks#pretooluse-decision-control) defines the replacement-input behavior.

## Local runner checks

Run `python3 docs/internal/experiments/trio-live-hooks-test.py` for five local checks without provider requests. They resolve relative output paths, execute generated hooks with spaces and apostrophes in the fixture and RTK paths, verify the store variables reach both processors, confirm missing cost blocks another call, check that timeout cleanup prevents a child from writing a delayed sentinel, and prove the fake Cargo command records its exact arguments and exits 7. All five passed.
