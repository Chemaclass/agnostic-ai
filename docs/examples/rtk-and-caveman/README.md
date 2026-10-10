# Project packs for RTK and Caveman

These two local packs work independently with existing agnostic-ai interfaces on POSIX Claude Code. The RTK pack registers its installed native processor. The Caveman pack delivers the default response skill from a pinned upstream revision. Invoke `/caveman` in Claude before your task. Delivering the skill does not prove it activated.

## Add a component

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

RTK must be installed separately and available on the hook's PATH to rewrite commands. Its absence produces no rewrite. Missing binaries do not change the generated configuration. The response skill needs no Caveman CLI or runtime. No pack launches an installer, proxy, provider request, or automatic skill activation.

The RTK hook uses native `PreToolUse` and `Bash`, with this command:

```sh
command -v rtk >/dev/null 2>&1 || exit 0; rtk hook claude
```

Do not add `decision: stdout`: RTK returns Claude's native input replacement JSON rather than agnostic-ai's portable decision object. The inline command needs no bundled hook helper. This recipe includes only the default Caveman skill. Its upstream references to `ultracave` and `megacave` do not install those modes.

## Keep or transfer existing ownership

Review project and global `.claude/settings.json` hooks before adding RTK. Also review `.claude/skills/caveman/` and `~/.claude/skills/caveman/`. A shell script such as `bash .claude/hooks/rtk-rewrite.sh` and the direct processor can both implement RTK. Different strings do not establish different roles. agnostic-ai preserves handwritten hooks, so adding the pack beside an upstream hook can run RTK twice.

To keep an upstream-owned RTK hook, leave the RTK pack out and add only Caveman. To keep an upstream-owned Caveman skill, leave the Caveman pack out and add only RTK. Neither choice changes the upstream installation.

For a deliberate project hook transfer, inspect the existing handler and copy its exact command. The example helper removes only that selected handler, retaining unrelated settings and sibling handlers. It refuses zero or multiple exact matches. Set `project` and `backup` to absolute paths; the backup must be a new file outside the managed output directories.

```sh
project=/absolute/path/to/your-project
backup=/absolute/path/to/rtk-hook-backup.json
python3 "$example/transfer-hook.py" take --project "$project" \
  --backup "$backup" --command 'bash .claude/hooks/rtk-rewrite.sh'
agnostic-ai packs add "$example/packs/rtk" --name rtk
agnostic-ai sync --all
agnostic-ai sync --check
```

The backup records only the selected handler, its group metadata, event, and position. Keep it until you decide which integration will own the hook. Review the resulting `PreToolUse` list and verify that only one RTK integration remains.

To transfer an upstream-owned project skill, move its entire directory outside `.claude/skills/` before installing the Caveman pack. Preserve this directory; it can contain assets besides `SKILL.md`.

```sh
skill_backup=/absolute/path/to/upstream-caveman-backup
test ! -e "$skill_backup" &&
  mv .claude/skills/caveman "$skill_backup" &&
  agnostic-ai packs add "$example/packs/caveman" --name caveman
agnostic-ai sync --all
agnostic-ai sync --check
```

Global RTK hooks and global Caveman skills are separate installations. These project commands do not remove them. Keep the corresponding project pack out while an active global installation supplies the same integration, or separately remove that global installation through its owner before transferring responsibility.

## Remove a component

Each component can be removed while the other remains:

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

The helper adds the backed-up handler to the current settings, preserving other changes. It refuses a backup from another project or a command already present. It does not delete the backup or remove an RTK pack for you.

To return the skill after removing Caveman and syncing:

```sh
mkdir -p .claude/skills
test ! -e .claude/skills/caveman &&
  mv "$skill_backup" .claude/skills/caveman
```

## Reproduce the checks

Use Python 3 and existing absolute binary paths. Choose a new output directory. The verifier runs only local configuration and JSON protocol checks in temporary projects. It reads bundled upstream files without downloading them, isolates `AGNOSTIC_AI_HOME`, and leaves `HOME` unchanged.

```sh
python3 "$example/verify.py" \
  --agnostic /absolute/path/to/agnostic-ai \
  --rtk /absolute/path/to/rtk \
  --output /tmp/rtk-caveman-check
```

`evidence.json` records command output and each acceptance check. Repeat-sync comparisons include native output, source files, and pack locks. They record bookkeeping hashes separately because `.sync-state` changes its time and changed-file count, and `.command-lock` records the process ID. [Recorded results](RESULTS.md) show one completed run.

## Source and distribution

[UPSTREAM.json](packs/caveman/UPSTREAM.json) records the source repository, exact revision, download URLs, and SHA-256 digests. The upstream skill is copied without changes. Its README, license texts, licensing explanation, and notices travel with the skill. agnostic-ai renders native frontmatter and adds a provenance header while retaining the instruction body.

Local packs work today without a plugin or a new project YAML setting. Their lock records a local path rather than a Git revision. Commit the pack sources when sharing this setup, or publish each pack as its own versioned Git repository and install a pinned tag or commit. Do not publish the combined parent directory as one pack: it holds two separate pack roots.

A plugin would need its own host installation and removal rules. A new YAML setting would still need external binaries and ownership handling. Neither addresses an unmet need in these local configuration checks. Runtime compression, exact recovery, real Claude behavior, and provider savings require separate evidence.
