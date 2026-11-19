package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// lintMemoryDarkStores reports each memory store whose facts no tool reads
// any longer, and that the personal store lacks by file name (LINT043):
// Claude Code's own store once sync points Claude at personal memory, and
// the personal store of the other memory.personal mode. It counts facts by
// file name, and notes kept in a store's index as one more, leaves out the
// facts declined with the same content, and never prints what a fact says.
func lintMemoryDarkStores(readers memoryReaders, root string) ([]lintFinding, error) {
	if readers.cfg == nil {
		return nil, nil
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", root, err)
	}
	stores, err := memoryStoresAt(root)
	if err != nil {
		return nil, err
	}
	personal := stores[1].dir
	if !filepath.IsAbs(personal) {
		personal = filepath.Join(root, personal)
	}
	have, err := adapters.MemoryFactFiles(personal)
	if err != nil {
		return nil, err
	}
	handled := importedFacts(personal)
	declined := declinedFacts(personal)
	type darkStore struct{ dir, from, why string }
	var candidates []darkStore
	if slices.Contains(readers.cfg.Targets, memorySourceClaude) && claudeReadsPersonalMemory(readers.cfg, root, personal) {
		if native, ok, err := adapters.NativeMemoryDir(memorySourceClaude, root); err == nil && ok {
			candidates = append(candidates, darkStore{native, memorySourceClaude, "Claude Code saved them before sync pointed it at personal memory"})
		}
	}
	if readers.cfg.RepoPersonalMemory() {
		candidates = append(candidates, darkStore{filepath.Join(root, defaultMemoryStores[1].dir), memorySourceCheckout, "they are in the checkout store, and memory.personal is repo"})
	} else if repo, err := adapters.PersonalMemoryDir(&config.Config{Memory: config.MemoryConfig{Personal: config.PersonalMemoryRepo}}, root); err == nil {
		candidates = append(candidates, darkStore{repo, memorySourceRepo, "they are in the repository store, and memory.personal is checkout"})
	}
	var out []lintFinding
	for _, c := range candidates {
		if canonicalDir(c.dir) == canonicalDir(personal) {
			continue
		}
		facts, err := adapters.MemoryFactFiles(c.dir)
		if err != nil {
			return nil, err
		}
		isDeclined := func(record string, body func() ([]byte, error)) bool {
			sum, ok := declined[c.from+"/"+record]
			if !ok || sum == "" {
				return ok
			}
			b, err := body()
			return err == nil && memoryFactSum(b) == sum
		}
		missing, declinedHere := 0, 0
		if notes := sourceIndexNotes(sourceIndex(c.dir)); len(notes) > 0 {
			fact, err := uniqueMemoryNotesFact(c.dir, personal, c.from, notes)
			if err != nil {
				return nil, err
			}
			switch {
			case handled[c.from+"/"+fact.record]:
			case isDeclined(fact.record, func() ([]byte, error) { return fact.body, nil }):
				declinedHere++
			default:
				missing++
			}
		}
		for _, f := range facts {
			switch {
			case slices.Contains(have, f) || handled[c.from+"/"+f]:
			case isDeclined(f, func() ([]byte, error) { return readMemoryFact(filepath.Join(c.dir, f)) }):
				declinedHere++
			default:
				missing++
			}
		}
		if missing == 0 {
			continue
		}
		message := fmt.Sprintf("%d fact(s) here no longer load: %s; copy them into personal memory with `agnostic-ai memory import --from %s`, or decline them with --skip <file> or --skip-rest", missing, c.why, c.from)
		if declinedHere > 0 {
			message += fmt.Sprintf("; %d declined", declinedHere)
		}
		out = append(out, lintFinding{
			Code:     "LINT043",
			Severity: lintWarn,
			Path:     c.dir,
			Message:  message,
		})
	}
	return out, nil
}

// claudeReadsPersonalMemory reports whether the Claude Code settings sync
// writes point auto memory at the personal store, so Claude no longer
// reads its own default store. A value the user set elsewhere keeps it.
func claudeReadsPersonalMemory(cfg *config.Config, root, personal string) bool {
	path := filepath.Join(root, claudeLayoutFor(cfg).dir, "settings.local.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var doc map[string]any
	if json.Unmarshal(data, &doc) != nil {
		return false
	}
	dir, _ := doc["autoMemoryDirectory"].(string)
	return dir != "" && canonicalDir(filepath.FromSlash(dir)) == canonicalDir(personal)
}
