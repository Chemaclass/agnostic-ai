package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUpgradeNPM_TargetsOnlyTheOwningGlobalPrefix(t *testing.T) {
	skipEndToEndInShortMode(t)
	npmOS, npmArch := runtime.GOOS, runtime.GOARCH
	if npmArch == "amd64" {
		npmArch = "x64"
	}
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		npmOS = "win32"
		name += ".exe"
	}
	tools := t.TempDir()
	if runtime.GOOS == "windows" {
		must(t, os.WriteFile(filepath.Join(tools, "npm.cmd"), []byte("@echo npm-target: %npm_config_prefix%\r\n"), 0o755))
	} else {
		const npm = `#!/bin/sh
prefix=$npm_config_prefix
while [ "$#" -gt 0 ]; do
	if [ "$1" = --prefix ]; then
		shift
		prefix=$1
	fi
	shift
done
printf 'npm-target: %s\n' "$prefix"
`
		must(t, os.WriteFile(filepath.Join(tools, "npm"), []byte(npm), 0o755))
	}
	configured := t.TempDir()
	for _, tc := range []struct {
		name       string
		globalTree bool
		globalShim bool
		hoisted    bool
	}{
		{name: "global", globalTree: true, globalShim: true},
		{name: "global hoisted", globalTree: true, globalShim: true, hoisted: true},
		{name: "project"},
		{name: "npx cache"},
		{name: "ambiguous", globalTree: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prefix := filepath.Join(t.TempDir(), "owned prefix")
			if tc.name == "npx cache" {
				prefix = filepath.Join(prefix, "_npx", "cached-package")
			}
			modules, binDir := filepath.Join(prefix, "node_modules"), prefix
			if tc.globalTree && runtime.GOOS != "windows" {
				modules, binDir = filepath.Join(prefix, "lib", "node_modules"), filepath.Join(prefix, "bin")
			}
			pkg := filepath.Join(modules, "agnostic-ai")
			dependencies := filepath.Join(pkg, "node_modules")
			if tc.hoisted {
				dependencies = modules
			}
			binary := filepath.Join(dependencies, "@agnostic-ai", npmOS+"-"+npmArch, name)
			copyBuiltBinary(t, builtCandidateBinary(t, "0.1.0"), binary)
			entryPoint := filepath.Join(pkg, "bin", "agnostic-ai.js")
			must(t, os.MkdirAll(filepath.Dir(entryPoint), 0o755))
			must(t, os.WriteFile(entryPoint, []byte("#!/usr/bin/env node\n"), 0o755))
			if tc.globalShim {
				must(t, os.MkdirAll(binDir, 0o755))
				if runtime.GOOS == "windows" {
					const shim = `@node "%dp0%\node_modules\agnostic-ai\bin\agnostic-ai.js" %*` + "\r\n"
					must(t, os.WriteFile(filepath.Join(binDir, "agnostic-ai.cmd"), []byte(shim), 0o755))
				} else {
					must(t, os.Symlink(entryPoint, filepath.Join(binDir, "agnostic-ai")))
				}
			}
			cmd := exec.Command(binary, "upgrade")
			cmd.Dir = t.TempDir()
			cmd.Env = append(os.Environ(),
				"PATH="+tools+string(os.PathListSeparator)+os.Getenv("PATH"),
				"HTTPS_PROXY=http://127.0.0.1:1", "NO_PROXY=",
				"APPDATA="+t.TempDir(), "XDG_DATA_HOME="+t.TempDir(),
				"NPM_CONFIG_PREFIX="+configured, "npm_config_prefix="+configured,
			)
			out, err := cmd.CombinedOutput()
			if !tc.globalShim {
				if err == nil || !strings.Contains(string(out), "Could not verify a global npm prefix") || strings.Contains(string(out), "npm-target: ") {
					t.Errorf("unverified npm install did not refuse a global upgrade: %v\n%s", err, out)
				}
				return
			}
			resolved, resolveErr := filepath.EvalSymlinks(prefix)
			must(t, resolveErr)
			if err != nil || !strings.Contains(string(out), "npm-target: "+resolved) {
				t.Errorf("upgrade did not use owning prefix %q: %v\n%s", resolved, err, out)
			}
		})
	}
}
