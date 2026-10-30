package cli

import (
	"reflect"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func mcpPreviewTargets(cfg *config.Config, global bool, data []byte) ([]string, error) {
	entry, err := spec.ParseYAMLBytes(spec.KindMCP, data)
	if err != nil {
		return nil, err
	}
	defer adapters.SetAsideNotes()()
	targets := []string{}
	for _, target := range cfg.Targets {
		if !slices.Contains(adapters.Names(), target) {
			continue
		}
		var reaches bool
		if global {
			g, ok := globalTargets[target]
			if !ok || g.mcp.path == "" || g.mcp.retired != "" || g.rootError(target) != nil || unsupportedGlobalTarget("--global", target) != nil {
				continue
			}
			entries := []spec.Entry{entry}
			switch g.mcp.format {
			case "toml-array":
				records, err := adapters.UserMCPArrayRecords(target, entries)
				reaches = err == nil && slices.ContainsFunc(records, func(r map[string]any) bool { return r["name"] == entry.Name })
			case "toml":
				tables, ok := adapters.UserMCPServerTables(target, entries)
				_, reaches = tables[entry.Name]
				reaches = reaches && ok
			default:
				servers, ok, err := adapters.CheckedUserMCPServers(target, entries)
				_, reaches = servers[entry.Name]
				reaches = reaches && ok && err == nil
			}
		} else {
			if _, ok := projectKindSupport(cfg)[spec.KindMCP][target]; !ok {
				continue
			}
			adapter, err := adapters.Resolve(target)
			if err != nil {
				continue
			}
			render := func(bundle spec.Bundle) ([]adapters.CapturedFile, error) {
				sess := adapters.NewSession()
				sess.StartWritesOnlyCapture()
				err := emitAdapter(sess, adapter, bundle, cfg, false)
				return sess.StopCapture(), err
			}
			before, err := render(spec.Bundle{})
			if err != nil {
				continue
			}
			after, err := render(spec.Bundle{MCPs: []spec.Entry{entry}})
			reaches = err == nil && !adapters.OmittedEntry(target, spec.KindMCP, entry.Name) && !reflect.DeepEqual(before, after)
		}
		if reaches {
			targets = append(targets, target)
		}
	}
	return targets, nil
}
