package spec

import "strings"

// FilterFences materializes body for readers, processing
// `::target <name>` / `::targets <a> <b>` / `::end` fences:
//
//   - Lines outside any fence stay for every reader.
//   - A fenced block stays when any reader is in its allow-list; the
//     marker lines themselves never emit.
//   - Fences do not nest: a second opener before ::end replaces the
//     allow-list, and ::end always closes.
//   - An unterminated fence runs to end-of-body so a missing ::end does
//     not silently drop the tail of the file.
//   - An empty readers list returns body unchanged (the source view).
//
// Entry.BodyFor is the one-reader form. The entry-point body
// (.agnostic-ai/AGNOSTIC_AI.md) passes every target reading one file as
// readers, so a shared AGENTS.md keeps a block when any of its readers is
// listed. Bodies without fences pass through unchanged.
func FilterFences(body string, readers []string) string {
	if len(readers) == 0 || !strings.Contains(body, targetFenceOpen) {
		return body
	}
	lines := strings.Split(body, "\n")
	var out strings.Builder
	out.Grow(len(body))
	keep := true // outside a fence: every reader sees the line
	for _, line := range lines {
		marker, allow := parseFenceMarker(line)
		switch marker {
		case fenceTargetOpen:
			keep = anyInAllowList(allow, readers)
			continue
		case fenceTargetClose:
			keep = true
			continue
		}
		if keep {
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	// strings.Split appends an empty trailing element for the final '\n',
	// and a kept blank connector line ahead of a dropped trailing fence
	// can stack more onto that. Trim every trailing newline the render
	// accumulated, then restore exactly one if the source itself ended in
	// one, so the result never carries more trailing newlines than body
	// had regardless of how many fences collapsed at the end.
	s := strings.TrimRight(out.String(), "\n")
	if s != "" && strings.HasSuffix(body, "\n") {
		s += "\n"
	}
	// The same stacking happens at the top: a dropped fence that opens the
	// body leaves the blank line after it as a leading one. Keep only the
	// leading newlines the source itself had (#790).
	if lead := leadingNewlines(body); leadingNewlines(s) > lead {
		s = strings.Repeat("\n", lead) + strings.TrimLeft(s, "\n")
	}
	// Dropped fences leave the surrounding blank lines stacked. Collapse
	// any run of 3 or more newlines back down to a paragraph break so
	// the rendered body reads cleanly for the active readers.
	return collapseBlankRuns(s)
}

// leadingNewlines counts the '\n' bytes s starts with.
// SplitReaderOnly splits body into the text every reader in others also
// sees and the ::target fences only reader sees (fences that allow reader
// and none of others), each rendered for reader. It lets a file that
// imports the others' file add just what differs.
func SplitReaderOnly(body, reader string, others []string) (rest, only string) {
	if !strings.Contains(body, targetFenceOpen) {
		return body, ""
	}
	var restB, onlyB strings.Builder
	inOnly := false
	for _, line := range strings.Split(body, "\n") {
		marker, allow := parseFenceMarker(line)
		switch marker {
		case fenceTargetOpen:
			inOnly = anyInAllowList(allow, []string{reader}) && !anyInAllowList(allow, others)
		case fenceTargetClose:
			if inOnly {
				inOnly = false
				onlyB.WriteString("\n")
				continue
			}
		}
		if inOnly {
			if marker != fenceTargetOpen {
				onlyB.WriteString(line)
				onlyB.WriteByte('\n')
			}
			continue
		}
		restB.WriteString(line)
		restB.WriteByte('\n')
	}
	rest = strings.TrimSuffix(restB.String(), "\n")
	return rest, strings.TrimSpace(collapseBlankRuns(onlyB.String()))
}

func leadingNewlines(s string) int {
	return len(s) - len(strings.TrimLeft(s, "\n"))
}

// FenceTargets returns every name used by a `::target` / `::targets`
// opener in body, deduplicated in first-seen order. validate uses this
// to flag a name that matches no known target.
func FenceTargets(body string) []string {
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.Split(body, "\n") {
		marker, allow := parseFenceMarker(line)
		if marker != fenceTargetOpen {
			continue
		}
		for _, name := range allow {
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// collapseBlankRuns rewrites runs of 3+ consecutive '\n' bytes as
// exactly 2 so a dropped fence does not leave a wider-than-paragraph
// gap. 2 newlines (one paragraph break) is the maximum.
func collapseBlankRuns(s string) string {
	if !strings.Contains(s, "\n\n\n") {
		return s
	}
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}

const (
	targetFenceOpen  = "::target"
	fenceTargetOpen  = 1
	fenceTargetClose = 2
)

// parseFenceMarker classifies a line as a fence opener, closer, or
// regular content. Returns the marker kind and the allow-list parsed
// off an opener. Marker lines must start at column 0; leading whitespace
// is treated as regular content so authors do not accidentally fence
// indented code blocks.
func parseFenceMarker(line string) (int, []string) {
	if line == "::end" || strings.HasPrefix(line, "::end ") {
		return fenceTargetClose, nil
	}
	switch {
	case strings.HasPrefix(line, "::target "):
		return fenceTargetOpen, strings.Fields(line[len("::target "):])
	case strings.HasPrefix(line, "::targets "):
		return fenceTargetOpen, strings.Fields(line[len("::targets "):])
	case line == "::target" || line == "::targets":
		// Bare opener with no targets — degenerate; treat as "no allow",
		// so the fence body is dropped for every target.
		return fenceTargetOpen, nil
	}
	return 0, nil
}

// inAllowList reports whether target appears in names. An empty list
// means "no target allowed" so a degenerate fence drops its body.
func inAllowList(names []string, target string) bool {
	for _, n := range names {
		if n == target {
			return true
		}
	}
	return false
}

// anyInAllowList reports whether any reader appears in names.
func anyInAllowList(names, readers []string) bool {
	for _, r := range readers {
		if inAllowList(names, r) {
			return true
		}
	}
	return false
}
