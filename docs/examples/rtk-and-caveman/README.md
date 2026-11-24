# Project packs for RTK and Caveman

Use these packs with Claude Code and a shell such as Bash on macOS or Linux. Each pack holds shared specs. The RTK pack adds a hook that calls your RTK installation. The Caveman pack adds a response skill from a pinned upstream commit. Run `/caveman` in Claude before your task to use the skill.

<a id="add-a-component"></a>

## Add a pack

Use an existing project configured with `targets: [claude]`. Set `example` to this directory in your agnostic-ai checkout. Run these commands from the project:

```sh
example=/absolute/path/to/agnostic-ai/docs/examples/rtk-and-caveman
agnostic-ai packs add "$example/packs/rtk" --name rtk
agnostic-ai sync --all
agnostic-ai sync --check
```

To add the response skill on its own:

```sh
agnostic-ai packs add "$example/packs/caveman" --name caveman
agnostic-ai sync --all
agnostic-ai sync --check
```

Install RTK separately and put it on the hook's PATH to rewrite commands. If RTK is missing, the hook leaves commands unchanged and sync still writes the same configuration. The response skill needs no Caveman executable or separate compression program. The packs write configuration only. They install no software, forward no requests, contact no model provider, and leave skill activation to you.

The RTK hook runs before Claude's `Bash` tool (`PreToolUse`), with this command:

```sh
command -v rtk >/dev/null 2>&1 || exit 0; rtk hook claude
```

Leave `decision: stdout` unset. RTK returns Claude's command replacement JSON, so the hook can call it directly. The Caveman pack includes only the default skill. `ultracave` and `megacave` require separate skills.

<a id="keep-or-transfer-existing-ownership"></a>

## Keep or replace an existing installation

Before adding RTK, review project and global `.claude/settings.json` hooks, `.claude/skills/caveman/`, and `~/.claude/skills/caveman/`. A script such as `bash .claude/hooks/rtk-rewrite.sh` and a direct `rtk hook claude` command can both run RTK. agnostic-ai preserves handwritten hooks, so adding the pack alongside an existing RTK hook can run RTK twice.

To keep an RTK-managed hook, leave the RTK pack out and add only Caveman. To keep a Caveman-managed skill, leave the Caveman pack out and add only RTK. Neither choice changes the existing installation.

To let agnostic-ai manage an existing project hook, copy its exact command from the settings file. The helper removes that command and leaves other hooks and settings alone. It requires one exact match. Use absolute paths for `project` and `backup`. Choose a new backup file outside the directories where sync writes files.

```sh
project=/absolute/path/to/your-project
backup=/absolute/path/to/rtk-hook-backup.json
python3 "$example/transfer-hook.py" take --project "$project" \
  --backup "$backup" --command 'bash .claude/hooks/rtk-rewrite.sh'
agnostic-ai packs add "$example/packs/rtk" --name rtk
agnostic-ai sync --all
agnostic-ai sync --check
```

The helper refuses a symlinked settings file or `.claude` directory. It replaces the settings file in one operation and preserves its permissions. Only the file's owner can read the backup.

The backup records only the selected hook command, its group settings, event, and position. Keep it until you decide which tool will manage the hook. Review the resulting `PreToolUse` list and verify that only one RTK hook remains.

To transfer a Caveman-managed project skill, move its entire directory outside `.claude/skills/` before installing the Caveman pack. Keep the whole directory because it can contain files besides `SKILL.md`.

```sh
skill_backup=/absolute/path/to/upstream-caveman-backup
test ! -e "$skill_backup" &&
  mv .claude/skills/caveman "$skill_backup" &&
  agnostic-ai packs add "$example/packs/caveman" --name caveman
agnostic-ai sync --all
agnostic-ai sync --check
```

Global RTK hooks and global Caveman skills are separate installations. These project commands do not remove them. Leave out the matching project pack while a global installation supplies the same hook or skill. To move it to agnostic-ai instead, first remove the global installation using the tool that created it.

<a id="remove-a-component"></a>

## Remove a pack

Each pack can be removed while the other remains:

```sh
agnostic-ai packs remove rtk
agnostic-ai sync --all
agnostic-ai sync --check
```

```sh
agnostic-ai packs remove caveman
agnostic-ai sync --all
agnostic-ai sync --check
```

To return the project hook to its previous owner after removing RTK and syncing:

```sh
python3 "$example/transfer-hook.py" restore --project "$project" --backup "$backup"
agnostic-ai sync --check
```

The helper adds the backed-up hook command to the current settings, preserving other changes. It refuses a backup from another project or a command already present. It does not delete the backup or remove an RTK pack for you.

To return the skill after removing Caveman and syncing:

```sh
mkdir -p .claude/skills
test ! -e .claude/skills/caveman &&
  mv "$skill_backup" .claude/skills/caveman
```

## Reproduce the checks

Use Python 3 and absolute paths to installed executables. Choose a new output directory. The check script tests local configuration and JSON requests and replies in temporary projects. It reads the copied third-party files without downloading them, uses a separate `AGNOSTIC_AI_HOME`, and leaves `HOME` unchanged.

```sh
python3 "$example/verify.py" \
  --agnostic /absolute/path/to/agnostic-ai \
  --rtk /absolute/path/to/rtk \
  --output /tmp/rtk-caveman-check
```

`evidence.json` records command output and each required check. Repeat-sync comparisons include generated files, source files, and pack locks. They record hashes (file-content fingerprints) for internal records separately because `.sync-state` changes its time and changed-file count, and `.command-lock` records the process ID. [Recorded results](RESULTS.md) show one completed run.

<a id="source-and-distribution"></a>

## Source and sharing

[UPSTREAM.json](packs/caveman/UPSTREAM.json) records the source repository, exact revision, download URLs, and SHA-256 hashes (file-content fingerprints). The original Caveman skill is copied without changes. Its license and notices are in the [upstream repository](https://github.com/JuliusBrussee/caveman). agnostic-ai writes the skill's opening settings in each tool's format and adds a header identifying its source while retaining the instruction body.

Local packs need no plugin or new project YAML setting. Their lock records a local path. To share this setup, commit the pack sources or publish each pack as its own Git repository and install a fixed tag or commit. The parent directory holds two packs, so publish the two source directories separately.

The test found no need for a plugin or new YAML setting. Both would still need rules for existing installations; a plugin would also need per-tool installation and removal rules, and a YAML setting would still need separately installed programs. agnostic-ai now also offers separate `rtk` and `caveman` names in its `builtins` list. See the [current setup guide](https://agnostic-ai.org/docs/rtk-and-caveman/) and choose one source for each tool. Compressing command output with a separate program, recovering its original text, actual Claude behavior, and model-provider savings require separate evidence.
