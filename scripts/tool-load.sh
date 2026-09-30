#!/usr/bin/env bash
#
# tool-load.sh - sync a probe project for one tool, then ask that tool, at
# its latest release, what it loaded. A docs page or a schema says where a
# tool reads; only the tool itself shows that it reads what sync wrote.
#
# Each tool gets its own project synced with only that target, so a check
# cannot pass on a file another target wrote. The tool runs with an empty
# home directory and the project marked trusted, so no user config and no
# login is involved. Every probe spec carries an AAI-PROBE-* marker or a
# probe-* name that a check then looks for in the tool's own output.
#
# Usage:
#   scripts/tool-load.sh [--bin <agnostic-ai>] [codex|gemini|opencode...]
#
# Prints "<ok|fail>\t<tool>\t<version>\t<check>" per check and exits 1 when
# any check fails. Needs node and npm; the tools install into a temp dir.

set -euo pipefail

TOOL_LOAD_ROOT=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
TOOL_LOAD_TOOLS="codex gemini opencode"
TOOL_LOAD_TIMEOUT="${TOOL_LOAD_TIMEOUT:-120}"

# tool_load_project <dir> <target> writes and syncs the probe project.
tool_load_project() {
  local dir="$1" target="$2" spec="$1/.agnostic-ai"
  mkdir -p "$spec/rules" "$spec/skills/probe-skill" "$spec/mcps" "$spec/agents" "$spec/commands"
  printf 'version: 1\ntargets: [%s]\n' "$target" >"$dir/agnostic-ai.yaml"
  printf '# Probe project\n\nProbe instruction AAI-PROBE-INSTRUCTION.\n' >"$spec/AGNOSTIC_AI.md"
  printf -- '---\nname: probe-rule\ndescription: Probe rule.\nalwaysApply: true\n---\n\nProbe rule AAI-PROBE-RULE.\n' \
    >"$spec/rules/probe-rule.md"
  printf -- '---\nname: probe-skill\ndescription: Probe skill AAI-PROBE-SKILL. Use when asked to probe.\n---\n\nSay probe.\n' \
    >"$spec/skills/probe-skill/SKILL.md"
  printf 'name: probe-mcp\ntype: stdio\ncommand: node\nargs: ["-e", "0"]\n' >"$spec/mcps/probe-mcp.yaml"
  printf -- '---\nname: probe-agent\ndescription: Probe agent AAI-PROBE-AGENT.\n---\n\nYou probe.\n' \
    >"$spec/agents/probe-agent.md"
  printf -- '---\nname: probe-command\ndescription: Probe command AAI-PROBE-COMMAND.\n---\n\nRun the probe.\n' \
    >"$spec/commands/probe-command.md"
  (cd "$dir" && git init -q && "$TOOL_LOAD_BIN" sync -q)
}

# tool_load_trust <tool> <home> <project> marks the project trusted, since
# every tool here ignores project MCP servers, skills, or agents otherwise.
tool_load_trust() {
  case "$1" in
    codex) printf '[projects."%s"]\ntrust_level = "trusted"\n' "$3" >"$2/.codex/config.toml" ;;
    gemini) printf '{"%s": "TRUST_FOLDER"}\n' "$3" >"$2/.gemini/trustedFolders.json" ;;
  esac
}

# tool_load_run <out> <tool> <args...> runs the tool in the probe project
# with the isolated home, stdout and stderr into <out>. A tool that fails
# still leaves its output for the checks to report against.
tool_load_run() {
  local out="$1" tool="$2"
  shift 2
  local cmd=("$TOOL_LOAD_PREFIX/node_modules/.bin/$tool" "$@")
  command -v timeout >/dev/null 2>&1 && cmd=(timeout "$TOOL_LOAD_TIMEOUT" "${cmd[@]}")
  (
    cd "$TOOL_LOAD_PROJECT"
    HOME="$TOOL_LOAD_HOME" CODEX_HOME="$TOOL_LOAD_HOME/.codex" \
      XDG_CONFIG_HOME="$TOOL_LOAD_HOME/.config" XDG_DATA_HOME="$TOOL_LOAD_HOME/.local/share" \
      XDG_CACHE_HOME="$TOOL_LOAD_HOME/.cache" XDG_STATE_HOME="$TOOL_LOAD_HOME/.local/state" \
      "${cmd[@]}"
  ) >"$out" 2>&1 || true
}

# tool_load_expect <tool> <version> <check> <needle> <file> prints one row.
tool_load_expect() {
  if grep -F -- "$4" "$5" >/dev/null 2>&1; then
    printf 'ok\t%s\t%s\t%s\n' "$1" "$2" "$3"
  else
    printf 'fail\t%s\t%s\t%s\n' "$1" "$2" "$3"
    TOOL_LOAD_FAILED=1
  fi
}

# tool_load_checks <tool> <version> <work dir> runs one tool's checks.
tool_load_checks() {
  local tool="$1" v="$2" w="$3"
  case "$tool" in
    codex)
      tool_load_run "$w/prompt" codex debug prompt-input
      tool_load_expect codex "$v" "AGENTS.md instructions reach the prompt" AAI-PROBE-INSTRUCTION "$w/prompt"
      tool_load_expect codex "$v" "rules reach the prompt" AAI-PROBE-RULE "$w/prompt"
      tool_load_expect codex "$v" "skills reach the prompt" AAI-PROBE-SKILL "$w/prompt"
      tool_load_run "$w/mcp" codex mcp list --json
      tool_load_expect codex "$v" "MCP servers load from .codex/config.toml" '"probe-mcp"' "$w/mcp"
      ;;
    gemini)
      tool_load_run "$w/mcp" gemini mcp list
      tool_load_expect gemini "$v" "MCP servers load from .gemini/settings.json" probe-mcp "$w/mcp"
      tool_load_run "$w/skills" gemini skills list
      tool_load_expect gemini "$v" "skills load from .gemini/skills" probe-skill "$w/skills"
      ;;
    opencode)
      tool_load_run "$w/config" opencode debug config
      tool_load_expect opencode "$v" "agents load from .opencode/agents" '"probe-agent"' "$w/config"
      tool_load_expect opencode "$v" "commands load from .opencode/commands" '"probe-command"' "$w/config"
      tool_load_expect opencode "$v" "MCP servers load from opencode.json" '"probe-mcp"' "$w/config"
      tool_load_run "$w/skills" opencode debug skill
      tool_load_expect opencode "$v" "skills load from .opencode/skills" '"probe-skill"' "$w/skills"
      ;;
    *)
      echo "unknown tool: $tool (want one of: $TOOL_LOAD_TOOLS)" >&2
      return 2
      ;;
  esac
}

tool_load_package() {
  case "$1" in
    codex) echo "@openai/codex@latest" ;;
    gemini) echo "@google/gemini-cli@latest" ;;
    opencode) echo "opencode-ai@latest" ;;
  esac
}

tool_load_main() {
  TOOL_LOAD_BIN=""
  if [ "${1:-}" = "--bin" ]; then
    TOOL_LOAD_BIN="$2"
    shift 2
  fi
  if [ -z "$TOOL_LOAD_BIN" ]; then
    TOOL_LOAD_BIN="$(mktemp -d)/agnostic-ai"
    (cd "$TOOL_LOAD_ROOT" && go build -o "$TOOL_LOAD_BIN" ./cmd/agnostic-ai)
  fi
  local tools="${*:-$TOOL_LOAD_TOOLS}" tool work pkgs=() v
  for tool in $tools; do pkgs+=("$(tool_load_package "$tool")"); done
  work=$(mktemp -d)
  TOOL_LOAD_PREFIX="$work/tools"
  npm install --prefix "$TOOL_LOAD_PREFIX" --no-audit --no-fund --silent "${pkgs[@]}" >/dev/null
  # A version manager's node shim reads the real home; the tools run with
  # an empty one, so they get the node binary itself on PATH.
  PATH="$(dirname "$(node -e 'process.stdout.write(process.execPath)')"):$PATH"
  export PATH
  TOOL_LOAD_FAILED=0
  for tool in $tools; do
    TOOL_LOAD_PROJECT="$work/$tool-project"
    TOOL_LOAD_HOME="$work/$tool-home"
    mkdir -p "$TOOL_LOAD_PROJECT" "$TOOL_LOAD_HOME/.codex" "$TOOL_LOAD_HOME/.gemini" "$work/$tool-out"
    tool_load_project "$TOOL_LOAD_PROJECT" "$tool"
    TOOL_LOAD_PROJECT=$(cd "$TOOL_LOAD_PROJECT" && pwd -P)
    tool_load_trust "$tool" "$TOOL_LOAD_HOME" "$TOOL_LOAD_PROJECT"
    tool_load_run "$work/$tool-out/version" "$tool" --version
    v=$(head -n 1 "$work/$tool-out/version" | tr -d '\r')
    tool_load_checks "$tool" "${v:-unknown}" "$work/$tool-out"
  done
  [ "$TOOL_LOAD_FAILED" = 0 ]
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  tool_load_main "$@"
fi
