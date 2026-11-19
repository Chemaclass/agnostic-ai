package errs

import (
	"errors"
	"regexp"
	"slices"
	"strconv"
)

// contextParserProblems are the gopkg.in/yaml.v3 parser errors raised
// inside an open context, such as an unclosed `[`. For these the library
// names a 0-based line: where the context starts or, when that is the
// first line, where the problem was found. Scanner errors name the right
// line. A context still open at the end of the file names the line after
// the last one.
var contextParserProblems = []string{
	"did not find expected <document start>",
	"did not find expected key",
	"did not find expected '-' indicator",
	"did not find expected node content",
	"did not find expected ',' or ']'",
	"did not find expected ',' or '}'",
	"found undefined tag handle",
	"found duplicate %YAML directive",
	"found incompatible YAML document",
	"found duplicate %TAG directive",
}

var yamlLinePrefix = regexp.MustCompile(`^yaml: line (\d+): (.+)$`)

// YAMLLine returns err with the line a yaml.v3 context parser error names
// made 1-based. Any other error comes back unchanged.
func YAMLLine(err error) error {
	if err == nil {
		return nil
	}
	m := yamlLinePrefix.FindStringSubmatch(err.Error())
	if m == nil || !slices.Contains(contextParserProblems, m[2]) {
		return err
	}
	line, convErr := strconv.Atoi(m[1])
	if convErr != nil {
		return err
	}
	return errors.New("yaml: line " + strconv.Itoa(line+1) + ": " + m[2])
}
