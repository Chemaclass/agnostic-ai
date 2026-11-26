+++
title = "Test a cloud setup from a fresh clone"
description = "Install a pinned agnostic-ai CLI in a small Python project and verify the setup that a cloud coding agent receives."
weight = 81

[extra]
group = "Workflows"
+++

Use a disposable Python project to check what Claude Code or Codex Cloud receives before assigning work. You need Git, Python 3 and Go 1.27.4. The first install needs access to the public Go module proxy and checksum service.

The [example project](https://github.com/Chemaclass/agnostic-ai/tree/main/docs/examples/cloud-consumer) pins a preview of agnostic-ai by commit. Release v0.99.0 predates both the Claude cloud install hook and `doctor environment`. The preview prints the same version number as that release. Use the Go module revision to identify it.

## Prepare the example

In an empty directory, copy the example into its own repository so the cloud files sit at the root. Commit the example with your usual Git identity. Then start from a separate clone, as the hosted agent would.

```bash
git clone --depth 1 https://github.com/Chemaclass/agnostic-ai.git source
mkdir greeting
git -C source archive HEAD:docs/examples/cloud-consumer | tar -x -C greeting
git -C greeting init -q
git -C greeting add .
git -C greeting commit -qm 'Cloud consumer fixture'
git clone --no-local greeting task
cd task
git rev-parse HEAD
git ls-files --error-unmatch AGENTS.md CLAUDE.md .claude/settings.json \
  .claude/hooks/agnostic-ai-cloud-install.sh setup.sh
```

The config sets `gitignore.commit: [cloud]`, so Git carries the instructions and the Claude hook. If your own project ignores its cloud setup, add `cloud` to the managed commit list, run `agnostic-ai sync`, review and commit the outputs. With hand-written ignores, follow the line edits in [the environment checklist](@/docs/cli-reference/doctor.md#doctor-environment).

## Install and verify

The setup script installs the pinned binary under `.tools/` and runs `agnostic-ai project --check`. A second invocation reuses the binary and checks the current checkout again. Run these commands from the project root.

```bash
sh setup.sh
export PATH="$PWD/.tools/47e82b256ad1e73456b50b55734f514eea2a130f:$PATH"
agnostic-ai --version
go version -m "$(command -v agnostic-ai)"
agnostic-ai doctor environment
python3 -m unittest test_greeting
```

The Go module version ends in `47e82b256ad1`. The checklist shows the committed Claude settings and install hook, and gives the Codex UI install command. The test passes for a named person.

In this scratch clone, change the source instructions while leaving the generated files at the old revision. Setup must report drift and exit nonzero. Regenerate the outputs, then check readiness and run the test again.

```bash
printf '\nKeep named greetings compatible.\n' >> .agnostic-ai/AGNOSTIC_AI.md
sh setup.sh
```

```bash
agnostic-ai sync
agnostic-ai project --check && python3 -m unittest test_greeting
```

## Run the same task in the cloud

Push the original `greeting` repository to a disposable remote your provider can access. Choose that repository and the commit printed above. The example needs no credentials of its own.

- **Claude Code.** Use an Anthropic-hosted session with one repository outside a project's cloud threads. Its committed session hook runs `sh setup.sh`. Configure network access for Go downloads in the environment. Put any required toolchain installation in the dashboard setup script, which can be cached. The repository hook runs outside that cache. [Claude cloud environments](https://code.claude.com/docs/en/cloud-environments)
- **Codex Cloud.** In the environment for this repository, enter `sh setup.sh` as the install script. Set up the toolchain and permitted domains there. Run the checkout check again as part of the task when the environment reuses cached setup. [Codex cloud environments](https://learn.chatgpt.com/docs/environments/cloud-environments)

Replace `<base>` with the original fixture commit and send this brief:

> Start at `<base>`. In greeting.py, make an empty name return "Hello, world!". Keep the named greeting unchanged. Add the empty-name case to test_greeting.py. Run `sh setup.sh && python3 -m unittest test_greeting`. Return the commit or diff and the test result. If blocked, include the failing command and error.

To compare runs, start once with a fresh environment and once with reused setup, each from the same base. Record dashboard setup time separately. For each run, record elapsed time to the first passing check and to the reviewable diff, retries, manual interventions, and billed usage when visible. A local clone verifies the files and commands. Hosted timings require a connected disposable repository and an environment for each provider.
