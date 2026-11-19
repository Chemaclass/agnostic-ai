package emit

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// BenchmarkFrontmatter measures one frontmatter render.
//
//	status-quo: one Node.Encode per key (frontmatterStyledReference)
//	proposed:   FrontmatterStyled, one Node.Encode for the whole map
//	baseline:   plain yaml.Marshal, which drops key order and styles
func BenchmarkFrontmatter(b *testing.B) {
	inputs := []struct {
		name   string
		meta   map[string]any
		keys   []string
		styles map[string]yaml.Style
	}{
		{
			name: "agent",
			meta: map[string]any{"name": "reviewer", "description": "Reviews diffs for defects.", "model": "sonnet"},
			keys: []string{"name", "description", "model"},
		},
		{
			name: "skill",
			meta: map[string]any{
				"name":          "gh-issue",
				"description":   "Use to work one GitHub issue by number",
				"allowed-tools": []any{"Read", "Edit", "Bash(gh *)", "Bash(go *)"},
				"metadata":      map[string]any{"version": "1.0", "owners": []any{"a", "b"}, "internal": true},
			},
			keys: []string{"name", "description", "allowed-tools", "metadata"},
		},
		{
			name: "styled",
			meta: map[string]any{
				"name":        "release",
				"description": "Ship a <ver> tag",
				"argument":    "[version]",
				"model":       "1.0",
				"globs":       "*.go",
			},
			keys: []string{"name", "description", "argument", "model", "globs"},
			styles: map[string]yaml.Style{
				"description": yaml.DoubleQuotedStyle,
				"argument":    yaml.SingleQuotedStyle,
				"model":       yaml.DoubleQuotedStyle,
			},
		},
	}
	for _, in := range inputs {
		b.Run(in.name+"/status-quo", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				frontmatterStyledReference(in.meta, in.keys, in.styles)
			}
		})
		b.Run(in.name+"/proposed", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				FrontmatterStyled(in.meta, in.keys, in.styles)
			}
		})
		b.Run(in.name+"/baseline", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := yaml.Marshal(in.meta); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
