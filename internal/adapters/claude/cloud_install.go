package claude

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/claudehooks"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func emitCloudInstall(sess *emit.Session, envs []spec.Entry, cfg *config.Config, dryRun bool) ([]spec.Entry, error) {
	var install string
	for _, e := range envs {
		if raw, ok := emit.ResolveMeta(e.Meta, target)["install"]; ok {
			install, _ = raw.(string)
		}
	}
	if strings.TrimSpace(install) == "" {
		return nil, nil
	}

	path := CloudInstallPath(cfg)
	if err := sess.WriteExecutableFile(path, cloudInstallScript(install), dryRun); err != nil {
		return nil, err
	}
	script := emit.ShellQuote(filepath.ToSlash(path))
	if !filepath.IsAbs(path) {
		script = `"$CLAUDE_PROJECT_DIR"/` + script
	}
	return []spec.Entry{{
		Kind: spec.KindHook,
		Name: "cloud-install",
		Meta: map[string]any{
			"event": "SessionStart", "matcher": claudehooks.CloudInstallMatcher, "shell": "bash",
			"command": claudehooks.CloudInstallCommand(script),
		},
	}}, nil
}

func CloudInstallPath(cfg *config.Config) string {
	return filepath.Join(emit.OutputDir(cfg, target, defaultDir), "hooks", claudehooks.CloudInstallScript)
}

func cloudInstallScript(install string) string {
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(install)))
	end := "AGNOSTIC_AI_INSTALL_" + digest
	for strings.Contains(install, end) {
		end += "_"
	}
	return "#!/bin/sh\n" + emit.HeaderBlock(emit.FormatShell) + `[ "${CLAUDE_CODE_REMOTE:-}" = true ] || exit 0
[ "${` + emit.HookTargetEnv + `:-}" = ` + target + ` ] || exit 0
exec >&2
cd "${CLAUDE_PROJECT_DIR:-.}" || exit 1
root=$(git rev-parse --show-toplevel) || exit 1
git_dir=$(git rev-parse --absolute-git-dir) || exit 1
cd "$root" || exit 1
marker="$git_dir/agnostic-ai-cloud-install"
exec 9>"$marker.lock" || exit 1
if command -v flock >/dev/null 2>&1; then
  flock 9 || exit 1
elif command -v lockf >/dev/null 2>&1; then
  lockf 9 || exit 1
else
  printf '%s\n' 'Claude Code cloud install needs flock or lockf.' >&2
  exit 1
fi
` + "expected=" + digest + "\n" + `if [ -f "$marker" ] && [ "$(cat "$marker")" = "$expected" ]; then
  exit 0
fi
rm -f "$marker" || exit 1
sh -e /dev/fd/8 </dev/null 9>&- 8<<'` + end + `' || exit $?
` + install + "\n" + end + "\n" + `printf '%s\n' "$expected" > "$marker"
`
}
