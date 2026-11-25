---
name: cloud-handoff
description: Push the current task to a branch and continue it in a cloud session. Use when asked to "send this to a remote agent", to "run it in the cloud", or when the user is "closing my laptop".
---

# cloud-handoff

Send the current task to a cloud session that goes on without this computer. Push the work first, then write down the goal, then launch. Claude Code can upload local files and history when the GitHub App is missing, so launch it only from the clean snapshot below.

## When

- Only when the user asks, or agrees after you offer. Never at session start, at session end, or on your own: it pushes code and starts paid work in the cloud.
- To pass the task to the next local agent instead, with no push and no network call, use the `handoff` skill if it is installed.

## Preflight

Check each of these before you change anything. If one fails, stop and say which one and what the user can do.

1. `git rev-parse --is-inside-work-tree` prints `true`. Otherwise there is nothing to push.
2. `git remote get-url origin >/dev/null` succeeds. Without a remote the cloud cannot clone the work. Keep the URL out of the output.
3. `command -v` finds the launch command of the row you will use in the Launch table below.
4. The row's other needs are met: a terminal wrapper for the cloud launch, or an environment id.
5. The push and the launch need network access. In a sandbox without it, ask the user to allow it, or give them the lines to run, and stop.

## Branch, commit, push

1. Never push to the default branch (the one `git symbolic-ref --short refs/remotes/origin/HEAD` names, else `main` or `master`). On the default branch or a detached HEAD, create a new branch first, such as `cloud/<short-task-slug>`, and name it in your reply.
2. Show `git status --short`. Stage only the files of this task, by path. Never stage a `.env` file, a key or certificate file, or anything else that looks like a secret; leave it out and say so.
3. Commit with a short message that names the task. If the commit fails, for example a hook or signing fails, stop and show its output. Do not skip the hook. Do not change Git settings, such as signing, to get the commit through.
4. Immediately before the push, record the branch from `git symbolic-ref --quiet --short HEAD` and its exact commit from `git rev-parse "<recorded-branch>"`. Use `git push -u origin <branch>` with that recorded branch. If it fails, stop and show the error. Never force-push.
5. After the push succeeds, keep those recorded values. Set and export `CLOUD_BRANCH` and `CLOUD_COMMIT` to them in the launch shell. Do not replace them with a later local HEAD. Include the commit in the prompt's Branch section.

## Write the prompt

The prompt is all the cloud session knows. Write it like this:

```markdown
<The goal, as one plain sentence.> [<branch>]

## Done
## Next step
## Branch
## Verify
```

- First line: the goal in one plain sentence, with the branch name at the end in brackets. Some tools use it to make the session title.
- **Done**: what this session finished, with paths relative to the project root.
- **Next step**: the first thing to do, then what remains.
- **Branch**: the pushed branch, that the work is committed on it, and that any pull request targets this branch, not the default branch.
- **Verify**: the checks to run before calling it done.
- Keep it under 60 lines. Never write secrets, tokens, or `.env` values.
- Write it to a file through a quoted heredoc, so the shell expands nothing in it, then read the file into a variable. Keep the heredoc outside `$( )`: an older bash, such as macOS `/bin/bash` and `/bin/sh`, breaks on quotes and parentheses there. Set it and launch in one shell command: a variable does not survive to the next tool call, and the launch refuses an empty prompt. The file stays in the temp folder, which the system clears.

```bash
f=$(mktemp)
cat >"$f" <<'PROMPT'
Finish the parser error messages. [cloud/parser-errors]

## Done
...
PROMPT
CLOUD_PROMPT=$(cat "$f")
export CLOUD_PROMPT
[ -n "$CLOUD_PROMPT" ] || exit 1
```

## Launch

Use the row for the tool this session runs in. Use another row only when the user names that tool.

| Tool | Launch | Needs |
|---|---|---|
| Claude Code | `claude --cloud "$CLOUD_PROMPT"`, wrapped in a terminal as shown below | A claude.ai sign-in, and `script` on macOS or Linux. `--environment <id>` only for a self-hosted environment. |
| Codex | `codex cloud exec --env "<id>" --branch "<branch>" -`, with the prompt on standard input as shown below | An environment id. The subcommand is marked experimental. |
| Any other tool | none | Falls back to a local handoff note. |

### Claude Code

Prepare a disposable snapshot before invoking Claude Code. A missing GitHub App can make the CLI upload local files and branch history. Never launch from the current checkout, even when its ordinary status looks clean. The shallow remote clone excludes other local branches, earlier commits and uncommitted files. `--no-local` also makes depth apply to relative paths and file remotes.

Run the prompt lines, this preparation and the terminal wrapper in one shell command, using the recorded branch and commit whose push succeeded. Any failure stops the launch. Refuse a URL with embedded credentials before cloning: Git would otherwise expose them in process arguments and save them in the clone's configuration. Do not print remote URLs or credentials, copy credential files, or change authentication settings. The clone uses the user's existing Git authentication.

```bash
[ -n "${CLOUD_BRANCH:-}" ] && [ -n "${CLOUD_COMMIT:-}" ] || exit 1
CLOUD_SOURCE=$(git rev-parse --show-toplevel) || exit 1
CLOUD_ORIGIN=$(git remote get-url origin) || exit 1
cloud_url_credentials=no
case "$CLOUD_ORIGIN" in
  *://*)
    cloud_scheme=${CLOUD_ORIGIN%%://*}
    case "$cloud_scheme" in
      ""|[!A-Za-z]*|*[!A-Za-z0-9+.-]*) ;;
      *)
        case "$CLOUD_ORIGIN" in *\?*|*\#*) cloud_url_credentials=yes ;; esac
        cloud_authority=${CLOUD_ORIGIN#*://}
        cloud_authority=${cloud_authority%%/*}
        case "$cloud_authority" in
          *@*)
            cloud_userinfo=${cloud_authority%@*}
            case "$CLOUD_ORIGIN" in
              [sS][sS][hH]://*|git+ssh://*|ssh+git://*)
                case "$cloud_userinfo" in *:*) cloud_url_credentials=yes ;; esac ;;
              *) cloud_url_credentials=yes ;;
            esac ;;
        esac ;;
    esac ;;
esac
[ "$cloud_url_credentials" = no ] || { echo 'Origin URL can carry credentials. Use a credential helper or an SSH remote, then ask again.' >&2; exit 1; }
CLOUD_SCRATCH=$(mktemp -d) || exit 1
git rev-parse --local-env-vars >"$CLOUD_SCRATCH/git-env" || exit 1
while IFS= read -r cloud_git_name; do unset "$cloud_git_name"; done <"$CLOUD_SCRATCH/git-env"
cd "$CLOUD_SOURCE" || exit 1
git clone --quiet --no-local --depth 1 --single-branch --no-tags --no-recurse-submodules --branch "$CLOUD_BRANCH" -- "$CLOUD_ORIGIN" "$CLOUD_SCRATCH/repo" >/dev/null 2>&1 || { echo 'Could not prepare the pushed snapshot.' >&2; exit 1; }
cd "$CLOUD_SCRATCH/repo" || exit 1
[ "$(git symbolic-ref --quiet --short HEAD)" = "$CLOUD_BRANCH" ] && [ "$(git rev-parse HEAD)" = "$CLOUD_COMMIT" ] || { echo 'The snapshot does not match the pushed branch and commit.' >&2; exit 1; }
cloud_status=$(git status --porcelain=v1 --untracked-files=all --ignored) || exit 1
[ -z "$cloud_status" ] || { echo 'The snapshot has local files or changes.' >&2; exit 1; }
```

`claude --cloud` refuses to run without a terminal (`Error: --cloud requires an interactive terminal.`), and a shell tool has none. Give it one with `script`, write its output to a file, wait for the `View: ` line, then kill it: after printing that line it keeps running. Feed it from `sleep`, so its input stays open and sends nothing; `</dev/null` would send it an end of input at once.

On macOS, after the prompt and snapshot preparation above:

```bash
log=$(mktemp)
sleep 150 | script -q /dev/null claude --cloud "$CLOUD_PROMPT" >"$log" 2>&1 &
pid=$!
for _ in $(seq 60); do grep -qE 'View: |Error' "$log" && break; sleep 2; done
sleep 2; kill "$pid" 2>/dev/null; pkill -P $$ -f '^sleep 150$'
cat "$log"
```

Run these lines at the top level of the shell command, not inside `( )`: the last line stops the input `sleep` through this shell's process id.

On Linux, start it with `sleep 150 | script -q -c 'claude --cloud "$CLOUD_PROMPT"' /dev/null >"$log" 2>&1 &` instead; the rest is the same. It reads the exported variable.

On success the output holds three lines: `Created cloud session: <title>`, `View: https://claude.ai/code/session_<id>?...`, and `Resume with: claude --teleport session_<id>`. The output may also hold terminal escape codes; read past them.

Once the wrapper has stopped, remove only the exact temporary snapshot directory recorded in `CLOUD_SCRATCH`, including its `git-env` file. First print its resolved absolute path and confirm it is the directory this preparation created. A failed preparation may also leave that directory behind. Never remove the original checkout.

Without `script`, such as on Windows, do not launch. Show the user the prompt and the recorded branch and commit. Before any clone, have them inspect the origin URL without displaying it. Refuse HTTP(S) userinfo, any URI username with a password, and URLs with a query or fragment. Apply URL checks only to origins with a URI scheme followed by `://`; literal local filenames may contain `?` or `#`. Ordinary username-only SSH URLs and `git@host:path` remotes are allowed. On refusal, stop and ask them to use a credential helper or an SSH remote.

After that check, have them create a fresh temporary folder and clone origin there with `git clone --no-local --depth 1 --single-branch --no-tags --no-recurse-submodules --branch "<recorded-branch>" "<origin>" "<fresh-folder>/repo"`, using their existing Git authentication. Clear repository-scoped variables named by `git rev-parse --local-env-vars` before cloning. From the new clone, require `git symbolic-ref --quiet --short HEAD` and `git rev-parse HEAD` to match the recorded values, and `git status --porcelain=v1 --untracked-files=all --ignored` to succeed with no output. Any error, mismatch or local file stops the launch. Only then may they run `claude --cloud "<prompt>"` in their own terminal. Never run that line from the original checkout. Remove only their recorded temporary snapshot folder once the local launch process has stopped.

### Codex

The command reads the prompt from standard input when the prompt argument is `-`, and needs no terminal. After the prompt lines above:

```bash
printf '%s\n' "$CLOUD_PROMPT" | codex cloud exec --env "<id>" --branch "<branch>" -
```

It prints one line on success: the task URL, `https://chatgpt.com/codex/tasks/<task_id>`.

The environment id is not stored anywhere this skill can read. If the user has not given one in this session, ask the user for it and stop. Tell them they can find or create it at chatgpt.com/codex/settings/environments, or browse it with `codex cloud` in their own terminal.

## After

- Start your last reply with one line that holds the session URL or id from the output, such as "Sent to the cloud: <url>". The user finds the cloud session from this line.
- Say that the cloud session starts from the pushed branch. A Claude Code session puts its work on a new branch of its own, so the user's branch stays theirs; a Codex task returns its work as a diff to apply.
- Name the command that brings the work back: `claude --teleport session_<id>`, or `codex cloud diff <url>` then `codex cloud apply <url>`.
- Say the session shows under Cloud in the companion within about a minute, when the companion lists cloud sessions.

## When it refuses

- Show the tool's message. Do not work around it: no other launch method, no retry with other flags.
- `Cloud sessions need a claude.ai sign-in.` or `Not signed in.`: the user signs in (`claude auth login` or `codex login`) and asks again.
- `no cloud environments are available for this workspace`: the user has no Codex environment yet; point to chatgpt.com/codex/settings/environments.
- ``environment '<id>' not found; run `codex cloud` to list available environments``: the id is wrong; ask the user for it and stop.
- A line starting `Branch <name> is not on GitHub`: the session started from older work. Say so; the push did not reach the remote the cloud reads.
- No row for this tool: say there is no cloud launch for it. Offer a local handoff note through the `handoff` skill if it is installed.
