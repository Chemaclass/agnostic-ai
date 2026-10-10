package preview

import (
	"path/filepath"
	"strings"
)

func frontmatter(path, body string) Result {
	lines := strings.SplitAfter(body, "\n")
	end := 1
	for end < len(lines) && strings.TrimSpace(lines[end]) != "---" {
		end++
	}
	if end == len(lines) {
		return withheld()
	}
	front := structured("yaml", strings.Join(lines[1:end], ""))
	tailBody := strings.Join(lines[end+1:], "")
	tail := markdown(tailBody)
	if strings.ToLower(filepath.Ext(path)) != ".md" {
		tail = Display("preview.txt", tailBody)
	}
	if front.Withheld || tail.Withheld {
		return withheld()
	}
	if !front.Hidden && !tail.Hidden {
		return Result{Text: body}
	}
	return Result{Text: lines[0] + front.Text + lines[end] + tail.Text, Hidden: true}
}

func markdown(body string) Result {
	lines := strings.SplitAfter(body, "\n")
	hidden := false
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "```") && !strings.HasPrefix(line, "~~~") {
			continue
		}
		count := len(line) - len(strings.TrimLeft(line, line[:1]))
		marker := line[:count]
		format := strings.TrimSpace(line[count:])
		switch format {
		case "json", "jsonc", "yaml", "yml", "toml", "sh", "bash":
		default:
			continue
		}
		end := i + 1
		for end < len(lines) {
			closing := strings.TrimSpace(lines[end])
			if strings.HasPrefix(closing, marker) && strings.Trim(closing, marker[:1]) == "" {
				break
			}
			end++
		}
		safe := Display("preview."+format, strings.Join(lines[i+1:end], ""))
		if safe.Withheld {
			return safe
		}
		if safe.Hidden {
			lines[i+1] = safe.Text
			for j := i + 2; j < end; j++ {
				lines[j] = ""
			}
			hidden = true
		}
		i = end
	}
	return Result{Text: strings.Join(lines, ""), Hidden: hidden}
}
