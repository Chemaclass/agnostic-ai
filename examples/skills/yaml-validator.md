---
name: yaml-validator
description: Validate YAML files against a schema. Use when user asks to lint or validate yaml.
---

# YAML Validator

Reads a YAML file, parses it, and reports schema violations.

## Steps

1. Read target file
2. Parse YAML
3. Compare against schema in `schema.yaml`
4. Report violations as `path: message`
