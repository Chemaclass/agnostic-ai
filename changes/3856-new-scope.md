### General

- `new <kind> <name> --scope local` creates a personal spec under `.agnostic-ai/local/`. Over a shared spec of the same kind and name, it starts with the name only. `--scope user` creates one in the user home from any directory (#3856).
- `new --scope local` and `doctor` warn when Git does not ignore the personal layer (#3856).
