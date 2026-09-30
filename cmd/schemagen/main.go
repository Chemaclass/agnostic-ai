// Command schemagen writes docs/schemas/config.schema.json by reflecting
// the Config struct. Run from the repo root: go run ./cmd/schemagen
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"

	"github.com/invopop/jsonschema"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func main() {
	r := jsonschema.Reflector{Mapper: func(t reflect.Type) *jsonschema.Schema {
		if s := mapType(t); s != nil {
			return s
		}
		if t != reflect.TypeFor[config.CoverageTargets]() {
			return nil
		}
		// One target name, or a list of them.
		return &jsonschema.Schema{OneOf: []*jsonschema.Schema{
			{Type: "string"},
			{Type: "array", Items: &jsonschema.Schema{Type: "string"}},
		}}
	}}
	schema := r.Reflect(&config.Config{})
	schema.ID = "https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/docs/schemas/config.schema.json"
	schema.Title = "agnostic-ai configuration"
	schema.Description = "Configuration file for agnostic-ai (agnostic-ai.yaml; legacy: agnostic.config.yaml)."

	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "marshal:", err)
		os.Exit(1)
	}

	const out = "docs/schemas/config.schema.json"
	if err := os.MkdirAll("docs/schemas", 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "mkdir:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, append(data, '\n'), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	fmt.Println("wrote", out)
}

// mapType describes the types whose YAML shape reflection cannot read: a
// model tier inlines its target map next to `effort`.
func mapType(t reflect.Type) *jsonschema.Schema {
	if t != reflect.TypeOf(config.ModelTier{}) {
		return nil
	}
	scalar := func() []*jsonschema.Schema {
		return []*jsonschema.Schema{{Type: "string"}, {Type: "integer"}}
	}
	props := jsonschema.NewProperties()
	props.Set("effort", &jsonschema.Schema{
		Description: "Effort the tier carries: a scalar, or a map of target name (or default) to a scalar.",
		OneOf: append(scalar(), &jsonschema.Schema{
			Type:                 "object",
			AdditionalProperties: &jsonschema.Schema{OneOf: scalar()},
		}),
	})
	return &jsonschema.Schema{
		Type:                 "object",
		Description:          "A model tier: target name (or default) to that target's model id, plus an optional effort.",
		Properties:           props,
		AdditionalProperties: &jsonschema.Schema{Type: "string", MinLength: ptr(uint64(1))},
	}
}

func ptr[T any](v T) *T { return &v }
