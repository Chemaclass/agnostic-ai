package adapters

import "github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"

// Keywords of the {{$AGENT:<name>}} and {{$SKILL:<name>}} body references.
const (
	RefAgent = emit.RefAgent
	RefSkill = emit.RefSkill
)

// BodyRef is one {{$AGENT:<name>}} or {{$SKILL:<name>}} in a spec body.
type BodyRef struct {
	Token   string
	Keyword string
	Name    string
}

// BodyRefs returns every reference in body, in order.
func BodyRefs(body string) []BodyRef {
	var out []BodyRef
	for _, m := range emit.RefPattern.FindAllStringSubmatch(body, -1) {
		out = append(out, BodyRef{Token: m[0], Keyword: m[1], Name: m[2]})
	}
	return out
}
