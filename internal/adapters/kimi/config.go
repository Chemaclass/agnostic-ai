package kimi

import (
	"bytes"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func emitConfig(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	return emitConfigForPlatform(sess, b, cfg, dryRun, runtime.GOOS)
}
func emitConfigForPlatform(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool, platform string) error {
	path := emit.OutputConfFile(cfg, target, "")
	if path == "" {
		emit.NoteFieldNoOp(target, spec.KindSettings, "project config", len(b.Settings), "set outputs.kimi.conf-file to a selected data-root config.toml")
		emit.NoteFieldNoOp(target, spec.KindHook, "project config", len(b.HooksFor(target)), "hooks require global sync or an explicit conf-file profile")
		return nil
	}
	if filepath.Base(path) != "config.toml" {
		return fmt.Errorf("kimi: conf-file %s must name config.toml in a selected KIMI_CODE_HOME directory", path)
	}
	doc := map[string]any{}
	if m := emit.SettingsModel(b.Settings, target); m != "" {
		doc["default_model"] = m
	}
	if effort := emit.SettingsEffort(b.Settings, target); effort != nil {
		doc["thinking"] = map[string]any{"effort": effort}
	}
	emit.MergeSettingsCustomKeys(doc, b.Settings, target)
	emit.NoteFieldNoOp(target, spec.KindSettings, "permissions", emit.SpecsWithPermissions(b.Settings), "only native x-kimi.permission is supported in an explicit profile")
	records, err := (Adapter{}).UserHookRecordsForPlatform(b.HooksFor(target), emit.HookScriptsDir(target), platform)
	if err != nil {
		return err
	}
	if len(records) > 0 {
		doc["hooks"] = records
	}
	if err := sess.MaterializeNeutralHookScripts(b.HooksFor(target), target, emit.HookScriptsDir(target), dryRun); err != nil {
		return err
	}
	if len(doc) == 0 {
		return sess.RemoveGenerated(path, dryRun)
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(doc); err != nil {
		return fmt.Errorf("kimi: encode %s: %w", path, err)
	}
	return sess.WriteFile(path, emit.WithHeader(buf.String(), emit.FormatTOML), dryRun)
}
