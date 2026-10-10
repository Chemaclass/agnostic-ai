import { spawnSync } from "node:child_process";
import { readdirSync } from "node:fs";
import { join } from "node:path";

const files = readdirSync(__dirname)
  .filter(file => file.endsWith(".test.js"))
  .sort()
  .map(file => join(__dirname, file));

if (files.length === 0) {
  throw new Error("No compiled extension tests found.");
}

const result = spawnSync(process.execPath, ["--test", ...files], { stdio: "inherit" });
if (result.error) throw result.error;
process.exit(result.status ?? 1);
