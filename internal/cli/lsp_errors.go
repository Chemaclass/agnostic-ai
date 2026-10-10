package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/lsp"
)

var (
	lspSpecPosition = regexp.MustCompile(`^(.+):(\d+):(\d+):`)
	lspLinePosition = regexp.MustCompile(`^(.+):(\d+):`)
	lspSourcePath   = regexp.MustCompile(`^(?:parse |read )?(.+\.(?:yaml|yml|md|mdc|json)):\s`)
	lspYAMLPosition = regexp.MustCompile(`(?:yaml: )?line (\d+)(?:: column (\d+))?:`)
)

func lspLoadFailure(root string, err error) (map[string][]lsp.Diagnostic, error) {
	path, position := lspErrorLocation(err)
	if path == "" {
		return nil, fmt.Errorf("load project for diagnostics: %w", err)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	if abs, absErr := filepath.Abs(path); absErr == nil {
		path = abs
	}
	diagnostic := lsp.Diagnostic{
		Range:    lsp.Range{Start: position, End: lsp.Position{Line: position.Line, Character: position.Character + 1}},
		Severity: lsp.SeverityError,
		Code:     string(errs.CodeOf(err)),
		Source:   "agnostic-ai",
		Message:  err.Error(),
	}
	return map[string][]lsp.Diagnostic{path: {diagnostic}}, fmt.Errorf("load project for diagnostics: %w", err)
}

func lspErrorLocation(err error) (string, lsp.Position) {
	body := err.Error()
	if code := errs.CodeOf(err); code != "" {
		prefix := "[" + string(code) + "] "
		for current := err; current != nil; current = errors.Unwrap(current) {
			if strings.HasPrefix(current.Error(), prefix) {
				body = strings.TrimPrefix(current.Error(), prefix)
				break
			}
		}
	}
	if parts := lspSpecPosition.FindStringSubmatch(body); parts != nil {
		return parts[1], lspErrorPosition(parts[2], parts[3])
	}
	if parts := lspLinePosition.FindStringSubmatch(body); parts != nil {
		return parts[1], lspErrorPosition(parts[2], "")
	}
	if parts := lspSourcePath.FindStringSubmatch(body); parts != nil && !strings.Contains(parts[1], " + ") && !strings.Contains(parts[1], ": ") {
		position := lsp.Position{}
		if yamlPosition := lspYAMLPosition.FindStringSubmatch(body); yamlPosition != nil {
			position = lspErrorPosition(yamlPosition[1], yamlPosition[2])
		}
		return parts[1], position
	}
	var pathError *os.PathError
	if errors.As(err, &pathError) {
		return pathError.Path, lsp.Position{}
	}
	return "", lsp.Position{}
}

func lspErrorPosition(line, column string) lsp.Position {
	lineNumber, _ := strconv.Atoi(line)
	columnNumber, _ := strconv.Atoi(column)
	return lsp.Position{Line: max(0, lineNumber-1), Character: max(0, columnNumber-1)}
}
