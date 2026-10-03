package crush

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// HooksFilePath is the file sync writes Crush's hooks to.
func HooksFilePath(cfg *config.Config) string {
	return emit.OutputMCPFile(cfg, target, defaultMCPFile)
}

// HookDoc renders the crush.json hooks sync writes for h alone, or nil
// when h produces no handler.
func HookDoc(h spec.Entry) ([]byte, error) {
	block := buildHooksBlock([]spec.Entry{h})
	if block == nil {
		return nil, nil
	}
	return emit.MarshalJSONIndent(map[string]any{"hooks": block})
}
