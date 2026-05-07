// Command schemagen writes docs/schemas/config.schema.json by reflecting
// the Config struct. Run from the repo root: go run ./cmd/schemagen
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/invopop/jsonschema"
)

func main() {
	r := jsonschema.Reflector{}
	schema := r.Reflect(&config.Config{})
	schema.ID = "https://github.com/chemaclass/agnostic-ai/docs/schemas/config.schema.json"
	schema.Title = "agnostic-ai configuration"
	schema.Description = "Configuration file for agnostic-ai (agnostic.config.yaml)."

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
