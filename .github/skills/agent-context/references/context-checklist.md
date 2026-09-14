# Context inventory checklist

For each file that can shape an agent's behavior, record:

| Field | Questions |
|---|---|
| Path | Where does the tool read it? |
| Owner | Is it user-owned, generated, or source-controlled? |
| Audience | Which agent or target receives it? |
| Scope | Does it apply to the whole repository or one directory? |
| Purpose | What decision or workflow does it support? |
| Evidence | Which current repository file proves it remains true? |

Review these failure modes:

- The same instruction appears in more than one source.
- A generated entry point was edited instead of its source spec.
- A command, path, tool, or version is no longer valid.
- A root file contains task detail that belongs in a skill or scoped rule.
- A shared file contains private, credential-bearing, or machine-specific data.
- Two instructions give incompatible directions for the same work.
