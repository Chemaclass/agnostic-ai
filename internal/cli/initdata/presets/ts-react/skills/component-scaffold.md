---
name: component-scaffold
description: Scaffold a new React component file with the project's naming and import conventions. Trigger when the user says "create a component" or "new component".
---

# Component scaffold

Creates a single `.tsx` file under `src/components/<Name>/` with:

1. A named `Props` type (no inline prop types).
2. A function component with explicit return type.
3. Co-located styles file when the project uses CSS modules.

## Steps

1. Ask for the component name (PascalCase).
2. Ask whether the component owns state or is purely presentational.
3. Write the file using the project's path-alias imports.
4. If a barrel `index.ts` exists in the parent folder, append the export.
