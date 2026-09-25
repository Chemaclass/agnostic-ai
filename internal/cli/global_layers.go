package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func globalSourceHome(home string) string {
	if source := os.Getenv("AGNOSTIC_AI_HOME"); source != "" {
		return source
	}
	return filepath.Join(home, ".agnostic-ai")
}

func globalLayers(source string) []spec.Layer {
	sources := config.Sources{Agents: "agents", Skills: "skills", Rules: "rules", Hooks: "hooks"}
	return []spec.Layer{
		{Name: "global", Root: source, Sources: sources},
		{Name: "global-local", Root: filepath.Join(source, "local"), Sources: sources},
	}
}

func globalInstructions(source string, rules []spec.Entry) ([]byte, error) {
	var parts []string
	for i, root := range []string{source, filepath.Join(source, "local")} {
		path := filepath.Join(root, "AGNOSTIC_AI.md")
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		if body := strings.TrimSpace(string(data)); body != "" {
			parts = append(parts, body)
		}
		if i == 0 {
			for _, rule := range rules {
				parts = append(parts, "## "+rule.Name+"\n\n"+strings.TrimSpace(rule.Body))
			}
		}
	}
	return []byte(strings.Join(parts, "\n\n")), nil
}
