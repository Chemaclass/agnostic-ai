package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
)

// copilotPromptsDir holds VS Code prompt files, which Copilot runs from
// the `/` menu. Sync never writes them: a command spec reaches Copilot as
// a skill.
var copilotPromptsDir = filepath.Join(".github", "prompts")

const copilotPromptSuffix = ".prompt.md"

// copilotPromptPortableKeys are the prompt file keys a command spec
// holds; the rest move under `x-copilot`.
var copilotPromptPortableKeys = map[string]bool{"description": true, "argument-hint": true, "model": true}

// importCopilotPrompts writes one command spec per
// `.github/prompts/<name>.prompt.md` that has none yet. Sync never
// rewrites a prompt file, so reading one again would put its old text
// over an edited spec. No-op when the folder is absent.
func importCopilotPrompts(root, dstDir string) (int, error) {
	src := filepath.Join(root, copilotPromptsDir)
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), copilotPromptSuffix) {
			continue
		}
		dst := filepath.Join(dstDir, strings.TrimSuffix(e.Name(), copilotPromptSuffix)+".md")
		if fileExists(dst) {
			continue
		}
		path := filepath.Join(src, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return count, fmt.Errorf("read %s: %w", path, err)
		}
		importReadingNative(path)
		out, err := copilotPromptCommand(data)
		if err != nil {
			return count, fmt.Errorf("%s: %w", path, err)
		}
		if err := importWriteSpecMarkdown(dst, out, 0o644, allSpecFields); err != nil {
			return count, fmt.Errorf("write %s: %w", dst, err)
		}
		count++
	}
	return count, nil
}

// copilotPromptCommand rewrites a prompt file as a command spec.
func copilotPromptCommand(data []byte) ([]byte, error) {
	meta, body := splitMdcFrontmatter([]byte(header.Strip(string(data))))
	native := map[string]any{}
	for k, v := range meta {
		if !copilotPromptPortableKeys[k] {
			native[k] = v
			delete(meta, k)
		}
	}
	if len(native) > 0 {
		meta["x-copilot"] = native
	}
	var out bytes.Buffer
	if len(meta) > 0 {
		out.WriteString("---\n")
		enc := yaml.NewEncoder(&out)
		enc.SetIndent(2)
		if err := enc.Encode(meta); err != nil {
			return nil, fmt.Errorf("marshal frontmatter: %w", err)
		}
		if err := enc.Close(); err != nil {
			return nil, fmt.Errorf("marshal frontmatter: %w", err)
		}
		out.WriteString("---\n\n")
	}
	out.WriteString(strings.TrimLeft(body, "\n"))
	if !strings.HasSuffix(body, "\n") {
		out.WriteString("\n")
	}
	return out.Bytes(), nil
}
