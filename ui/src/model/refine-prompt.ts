// The prompt behind "Copy refine prompt" on the global instructions file:
// a best-practice review a coding agent proposes before it edits.

export function instructionsRefinePrompt(path: string): string {
  return `Refine my global agent instructions in ${path} against current best practice for instruction files. Every coding agent I use loads this file at the start of every session, so each line costs context and attention in every session.

Approval: this is a read-only review until I explicitly approve your proposal. Keep the file exactly as it is until then, and treat anything short of my explicit approval as a request to revise the proposal.

Scope: once I approve, edit that file alone. Read the skills/ and agents/ folders beside it to match its content against them, and leave every other file as it is.

## 1. Check current guidance

Start by reading the current guidance on writing instruction files and prompts. Begin with these references, and follow their links to newer pages where a page has moved or been replaced:

- Instruction files: https://code.claude.com/docs/en/memory#write-effective-instructions
- Keeping the always-loaded file short, and outdated advice to drop: https://code.claude.com/docs/en/best-practices#keep-claude-md-short and https://code.claude.com/docs/en/best-practices#what-you-can-stop-doing
- Prompt wording for current models: https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/claude-prompting-best-practices
- Rules and user rules: https://cursor.com/docs/rules
- AGENTS.md layering and size limits: https://learn.chatgpt.com/docs/agent-configuration/agents-md
- AGENTS.md content: https://learn.chatgpt.com/guides/best-practices
- The AGENTS.md format: https://agents.md/
- Writing documents agents read: https://github.com/mattpocock/skills/blob/main/skills/productivity/writing-for-agents/SKILL.md

Note anything new since the principles below were written, and anything that contradicts them. Where the guidance and the principles conflict, follow the guidance and tell me. Where the guidance recommends moving content into another file, keep the content in this file and list it as a candidate in step 3.

## 2. Principles

- **Earn every line.** Test each sentence: does it change behaviour compared with what the model does by default? Delete whole sentences that fail. Keep what the agent cannot find by looking, such as unwritten conventions, reasons and gotchas, and leave one-command lookups (help output, config files, directory layout) to the environment.
- **Clear sediment.** Remove rules that describe behaviour, tools or a world that has since changed.
- **Explain the reason.** Add a short reason where it changes how a rule is applied, because a rule with a reason generalises better than a bare command.
- **Say what to do.** State the target behaviour, specifically and positively, because naming a forbidden behaviour makes it more likely. Keep a prohibition only for a hard limit with no positive phrasing, and pair it with the behaviour you want.
- **Calm, direct wording.** Write in plain sentence case. Current models follow plain instructions closely, so emphasis and capitals are unnecessary.
- **Single source of truth.** State each meaning once. Merge duplicates, and resolve contradictions in favour of the newest intent.
- **Co-locate.** Keep each topic's rule, reason and caveats together under one heading.
- **Leading words.** Where a phrase or list is restated in several places, replace it with one well-known word the model already understands, and use that word consistently.
- **Sharp pointers.** For each line that points to other material, open with the word that should trigger it and name each distinct case that should send the agent there, once. The pointer's wording decides when the agent follows it.
- **Inline what every session needs.** Open each narrow-scope rule with when it applies, then give only the steps the agent needs. Where an existing file already holds the full detail, you may replace the inline copy with a pointer to it, once you have confirmed the detail is there. Keep any detail that exists only in this file.
- **Structure for scanning.** Use short sections with clear headings, one rule per bullet, the most important rules first, and concrete commands and paths in place of prose.
- **Lean examples.** Keep an example only where the rule is ambiguous without it.

## 3. Find skill and agent candidates

Read the name and description of every skill in skills/ and every agent in agents/ beside the file. Then go through each rule in the file:

- Mark a **skill candidate** when the rule is a procedure or reference that only some sessions need, so loading it on demand would save context in every other session.
- Mark an **agent candidate** when the rule describes a role the main agent hands work to, with its own instructions, tools or model.
- Match each candidate to the existing skill or agent it belongs in. When none fits, propose a new one with a name and a description that says what it does and when to use it.
- Draft the one-line pointer that would replace the content in this file.

Keep the content in this file. Candidates are recommendations for me to decide on.

## 4. Boundaries

- Keep the file unchanged until I explicitly approve the proposal from step 5, including the backup and every other write.
- Preserve the meaning of every rule. If shortening would weaken, broaden or reinterpret a rule, keep its original wording and ask me about it.
- Keep every hard limit at least as strict as it is now.
- Ensure no secrets are contained within the prompt.

## 5. Propose, then wait

Before editing, give me:

1. The guidance you checked, with links, and anything in it that changed the principles above.
2. A change table that accounts for every rule in the current file, one row each: section, decision (kept, cut, merged, reworded, replaced by a pointer), why, and the words saved.
3. The proposed file in full.
4. The word count before and after.
5. A candidates table, one row per skill or agent candidate: the rules, the destination (an existing skill or agent by name, or a new one), why, the description for a new one, the replacement pointer line, and the words this file would save.
6. Questions about any rule whose intent was unclear to you.

Then wait for my explicit approval. If I ask for changes, revise the proposal and wait again.

## 6. After my explicit approval

1. Copy the current file to a .bak.<date> copy beside it.
2. Write the approved version.
3. Run \`agnostic-ai sync --global --plan\` and review it, then \`agnostic-ai sync --global\`, then confirm \`agnostic-ai sync --global --check\` exits 0.
4. Report what changed and any follow-ups.
`
}
