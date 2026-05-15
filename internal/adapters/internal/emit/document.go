package emit

// Document renders a markdown document with optional YAML frontmatter
// followed by body. When meta resolves to an empty frontmatter block,
// the document is body-only with no leading blank line. The target
// argument selects target-specific keys via ResolveMeta.
//
// Use this at every adapter call site that writes a `<frontmatter>\n<body>`
// markdown file so empty-meta entries do not leak a blank first line and
// non-empty meta round-trips cleanly back through the spec loader.
func Document(meta map[string]any, body, target string) string {
	return DocumentOrdered(meta, nil, body, target)
}

// DocumentOrdered is Document with a source key-order hint. Adapters
// that load specs from disk pass `entry.MetaKeys` so frontmatter emits
// in author-intended order.
func DocumentOrdered(meta map[string]any, keys []string, body, target string) string {
	rmeta, rkeys := ResolveMetaOrdered(meta, keys, target)
	fm := FrontmatterOrdered(rmeta, rkeys)
	if fm == "" {
		return body
	}
	return fm + "\n" + body
}
