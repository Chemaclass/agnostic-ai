package config

import (
	"fmt"
	"os"
	"strings"
)

// PersistAutoSync writes autoSync: true/false to the project's base
// config file (preferring agnostic-ai.yaml, falling back to the legacy
// agnostic.config.yaml). Replaces the existing line when present,
// otherwise appends. The rest of the file is preserved as-is. The
// local-override file is never touched.
func PersistAutoSync(root string, enabled bool) error {
	path, _, err := ResolveConfigPath(root)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	val := "false"
	if enabled {
		val = "true"
	}
	newLine := "autoSync: " + val
	content := string(data)

	if strings.Contains(content, "autoSync:") {
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "autoSync:") {
				lines[i] = newLine
				break
			}
		}
		content = strings.Join(lines, "\n")
	} else {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += newLine + "\n"
	}

	return os.WriteFile(path, []byte(content), 0o644)
}
