### General

- `new rule --dir <dir>` replaces `new rule --scope <dir>`, and `doctor --checks <all|project>` replaces `doctor --scope`. The older spellings still work and print a deprecation warning on stderr. `new rule --scope <value>` reads the value as a directory only when it has a path separator or names an existing directory (#3856).
