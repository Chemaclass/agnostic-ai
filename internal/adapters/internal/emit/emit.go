package emit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func WriteFile(path, content string, dryRun bool) error {
	if dryRun {
		fmt.Printf("--- %s ---\n%s\n", path, content)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func Frontmatter(meta map[string]any) string {
	if len(meta) == 0 {
		return ""
	}
	data, err := yaml.Marshal(meta)
	if err != nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.Write(data)
	b.WriteString("---\n")
	return b.String()
}
