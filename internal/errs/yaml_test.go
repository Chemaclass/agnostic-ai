package errs

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestYAMLLine_NamesTheLineAnOpenContextStartsOn(t *testing.T) {
	for _, tc := range []struct{ doc, want string }{
		{"version: 1\ntargets: [claude\n", "yaml: line 2: did not find expected ',' or ']'"},
		{"version: 1\nmodel: {a: b\n", "yaml: line 2: did not find expected ',' or '}'"},
		{"version: 1\ntargets: \"claude\n", "yaml: line 2: found unexpected end of stream"},
		{"a: b\n  c: d\n", "yaml: line 2: mapping values are not allowed in this context"},
		{"targets: [claude\n", "yaml: line 1: did not find expected ',' or ']'"},
		{"a: 1\nb: 2\nx: [", "yaml: line 3: did not find expected node content"},
		{"# c\n%YAML 1.1\n%YAML 1.1\n---\na: 1\n", "yaml: line 3: found duplicate %YAML directive"},
	} {
		var out any
		err := YAMLLine(yaml.Unmarshal([]byte(tc.doc), &out), []byte(tc.doc))
		if err == nil || err.Error() != tc.want {
			t.Errorf("%q: got %v, want %q", tc.doc, err, tc.want)
		}
	}
	if YAMLLine(nil, nil) != nil {
		t.Error("nil error changed")
	}
}
