# Project packs for RTK and Caveman

These two local packs (folders of shared specs) work independently with existing agnostic-ai commands on Claude Code using a POSIX shell, such as on macOS or Linux. The RTK pack adds a hook that calls an installed RTK. The Caveman pack adds the default response skill from a fixed revision of the Caveman project. Invoke `/caveman` in Claude before your task. Copying the skill does not prove Claude used it.

<a id="add-a-component"></a>

## Add a pack

Use an existing project configured with `targets: [claude]`. Set `example` to this directory in your agnostic-ai checkout. Run these commands from the project:

```sh
example=/absolute/path/to/agnostic-ai/docs/examples/rtk-and-caveman
agnostic-ai packs add "$example/packs/rtk" --name rtk
agnostic-ai sync --all
agnostic-ai sync --check
```

To add the response skill, independently:

```sh
agnostic-ai packs add "$example/packs/caveman" --name caveman
agnostic-ai sync --all
agnostic-ai sync --check
```

RTK must be installed separately and available on the hook's PATH to rewrite commands. Without RTK, the hook leaves commands unchanged. Missing executables do not change the generated configuration. The response skill needs no Caveman executable or separate compression program. Neither pack installs software, forwards requests through another service, contacts a model provider, or starts the skill automatically.

The RTK hook runs before Claude's `Bash` tool (`PreToolUse`), with this command:

```sh
command -v rtk >/dev/null 2>&1 || exit 0; rtk hook claude
```

Do not add `decision: stdout`: RTK returns the JSON format Claude uses to replace a command, rather than agnostic-ai's shared decision format. The command needs no separate helper script. This recipe includes only the default Caveman skill. Its references in the original skill to `ultracave` and `megacave` do not install those modes.

<a id="keep-or-transfer-existing-ownership"></a>

## Keep or replace an existing installation

Review project and global `.claude/settings.json` hooks before adding RTK. Also review `.claude/skills/caveman/` and `~/.claude/skills/caveman/`. A shell script such as `bash .claude/hooks/rtk-rewrite.sh` and a direct `rtk hook claude` command can both run RTK. Different command text does not mean they do different jobs. agnostic-ai preserves handwritten hooks, so adding the pack beside a hook installed by RTK can run RTK twice.

To keep an RTK-managed hook, leave the RTK pack out and add only Caveman. To keep a Caveman-managed skill, leave the Caveman pack out and add only RTK. Neither choice changes the existing installation.

To let agnostic-ai manage an existing project hook, inspect that hook and copy its exact command. The example helper removes only that selected hook command, keeping unrelated settings and other hook commands. It refuses zero or multiple exact matches. Set `project` and `backup` to absolute paths; the backup must be a new file outside the directories where sync writes files.

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

To transfer a Caveman-managed project skill, move its entire directory outside `.claude/skills/` before installing the Caveman pack. Preserve this directory; it can contain files besides `SKILL.md`.

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

[UPSTREAM.json](packs/caveman/UPSTREAM.json) records the source repository, exact revision, download URLs, and SHA-256 hashes (file-content fingerprints). The original Caveman skill is copied without changes. Its README, license texts, licensing explanation, and notices travel with the skill. agnostic-ai writes the skill's opening settings in each tool's format and adds a header identifying its source while retaining the instruction body.

Local packs work today without a plugin or a new project YAML setting. Their lock records a local path rather than a Git revision. Commit the pack sources when sharing this setup, or publish each pack as its own versioned Git repository and install a fixed tag or commit. Do not publish the combined parent directory as one pack: it holds two separate packs.

At the time of this pack test, a plugin would need installation and removal rules for each coding tool. A new YAML setting would still need separately installed programs and rules for managing existing installations. Neither addressed an unmet need in those local configuration checks. agnostic-ai now also offers separate `rtk` and `caveman` names in its existing `builtins` list. See the [current setup guide](https://agnostic-ai.org/docs/rtk-and-caveman/) and choose one source for each tool. Compressing command output with a separate program, recovering its original text, actual Claude behavior, and model-provider savings require separate evidence.
