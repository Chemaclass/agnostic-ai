import type { ItemFinding } from '../contract'

const TITLES: Record<string, string> = {
  LINT001: 'This file is empty, so your tools get an empty copy.',
  LINT003: 'Two files share one name, so one of them is lost.',
  LINT004: 'None of your selected tools reads this file.',
  LINT005: 'This hook names a tool to match, but its event never uses one.',
  LINT006: 'The settings block at the top of this file is never closed.',
  LINT007: 'A setting at the top of this file looks misspelled, so it is ignored.',
  LINT008: 'A server is missing the command or address that starts it.',
  LINT009: 'A permission allows more than it appears to.',
  LINT010: 'A rule in the global setup has conditions the global setup cannot use.',
  LINT011: 'Your tools load more text at startup than the startup budget allows.',
  LINT012: 'A description is longer than the description budget.',
  LINT013: 'A rule lists the files it applies to in a form tools cannot read.',
  LINT014: 'A global setting holds a value one of your tools cannot take.',
  LINT015: 'This file points to another file by a path only one tool has.',
  LINT016: 'A development command is missing a name or a way to start it.',
  LINT017: 'A Git setting names a tool you have not selected.',
  LINT018: 'This skill sets a scope, which skills ignore.',
  LINT019: 'This file uses Claude Code syntax that other tools show as plain text.',
  LINT020: 'This rule sits in one folder but says it applies to another.',
  LINT021: 'A shell permission has no matching rule for Codex, so Codex decides differently.',
  LINT022: 'A protected path covers a file agnostic-ai rewrites, not the file you edit.',
  LINT023: 'A protected path block is written in a form agnostic-ai cannot use.',
  LINT024: 'An accepted gap no longer matches anything, or could not be checked.',
  LINT025: 'A model choice has no entry for one of your tools.',
  LINT026: 'A Claude model name reaches a tool that cannot load it.',
  LINT027: 'A server setting holds a value that cannot be saved.',
  LINT028: 'A server address uses a variable form only one tool understands.',
  LINT029: 'A Kiro agent leaves out the shared instructions file.',
  LINT030: 'A hook uses a Gemini CLI variable in a form that can run part of its value as a command.',
  LINT031: 'A description is still the placeholder text.',
  LINT032: 'A hook uses an event or tool one of your tools cannot run.',
  LINT033: 'This file names an agent or skill that does not exist for its tools.',
  LINT034: 'A hook uses one tool\'s event name where a shared form works for every tool.',
  LINT035: 'A server setting may hold a password or key written in plain text.',
  LINT036: 'A permission or agent ability is written in a form agnostic-ai cannot read.',
  LINT037: 'A tool name has a shared form that works for every tool.',
  LINT038: 'A permission allows every use of a tool, not only the paths or commands you meant.',
  LINT039: 'The memory index is too long for some tools to load in full.',
  LINT040: 'The memory index links a file that does not exist.',
  LINT041: 'A memory file is missing from the index, so no tool loads it.',
  LINT042: 'A memory file looks like it holds a password or key.',
  LINT043: 'Some saved memory is in a place no tool reads any more.',
  LINT044: 'A memory file holds a character a reader cannot see.',
  LINT045: 'The number of days to keep conversations is not a whole number of 1 or more.',
  LINT046: 'A setting at the top of this file is written twice.',
  LINT047: 'A skill or command name has a character Claude Code refuses.',
  LINT048: 'This agent has no description, so tools cannot tell when to use it.',
  LINT049: 'A server setting may hold a password or key in a place agnostic-ai cannot protect.',
  LINT050: 'An agent\'s turn limit or background setting is not a valid value.',
  LINT051: 'An extra settings file has a name no tool reads.',
  LINT052: 'This hook has nothing to run.',
  LINT053: 'One of your tools refuses a file or setting, which stops every update.',
  LINT054: 'The voice dictation setting is not in a form tools can read.',
  LINT055: 'An empty value in your personal configuration may not do what you expect.',
  LINT056: 'The block at the top of this file is not a settings block, so it stays as text.',
  LINT057: 'A path pattern uses ** in a way that matches fewer files than it seems.',
  LINT058: 'A server uses an old Codex setting that Codex now ignores.',
  LINT059: 'A server timeout is outside the range a tool accepts.',
  LINT060: 'A server folder entry is missing its address.',
  'lint-incomplete': 'The check stopped before it finished, so some findings may be missing.',
  'nested-claude-copy': 'Claude Code loads this text twice, from this file and from a rule.',
  'merged-claude-hooks': 'Claude Code runs these hook commands twice.',
  'codex-hook-trust': 'Codex skips this hook until you review it, or turn it on, in /hooks.',
  'divergent-hook-script': 'One hook script has different versions in different tools.',
}

// A check that could not run reports under the code of what it checks; its message says why.
const CHECK_FAILED = /^(cannot|could not) /

/** The plain one-sentence title for a finding; its raw checker message when the code has none or the check could not run. */
export function findingTitle(finding: Pick<ItemFinding, 'code' | 'message'>): string {
  if (CHECK_FAILED.test(finding.message)) return finding.message
  return (finding.code && TITLES[finding.code]) || finding.message
}

/** Whether the raw message adds text beyond the title, so it belongs under Details. */
export function findingHasRawMessage(finding: Pick<ItemFinding, 'code' | 'message'>): boolean {
  return findingTitle(finding) !== finding.message
}
