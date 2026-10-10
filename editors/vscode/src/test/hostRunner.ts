import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import { runTests } from "@vscode/test-electron";

async function main(): Promise<void> {
  const binary = process.env.AGNOSTIC_AI_TEST_BINARY;
  if (!binary || !fs.existsSync(binary)) throw new Error("Set AGNOSTIC_AI_TEST_BINARY to the built CLI.");
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "agnostic diagnostics "));
  const project = path.join(root, "configured project");
  const external = path.join(root, "external rules");
  const home = path.join(root, "home");
  fs.mkdirSync(home);
  fs.mkdirSync(project);
  fs.mkdirSync(external);
  fs.mkdirSync(path.join(project, "custom hooks"));
  fs.writeFileSync(path.join(project, "agnostic-ai.yaml"), `version: 1\ntargets: [claude]\nsources:\n  rules: ${JSON.stringify(external)}\n  hooks: custom hooks\n`);
  fs.writeFileSync(path.join(external, "empty.md"), "---\nname: empty\n---\n");
  fs.mkdirSync(path.join(project, ".vscode"));
  fs.writeFileSync(path.join(project, ".vscode", "settings.json"), JSON.stringify({ "agnostic-ai.binaryPath": binary, "agnostic-ai.driftPollSeconds": 3600 }));
  await runTests({
    version: "1.85.2",
    extensionDevelopmentPath: path.resolve(__dirname, "../.."),
    extensionTestsPath: path.resolve(__dirname, "integration", "host.js"),
    launchArgs: [project, "--disable-extensions", "--skip-welcome", "--skip-release-notes", "--disable-workspace-trust", "--user-data-dir", path.join(root, "user data"), "--extensions-dir", path.join(root, "extensions")],
    extensionTestsEnv: { HOME: home, USERPROFILE: home, AGNOSTIC_AI_TEST_ROOT: project, AGNOSTIC_AI_TEST_EXTERNAL: external, AGNOSTIC_AI_TEST_BINARY: binary, AGNOSTIC_AI_TEST_NODE: process.execPath, AGNOSTIC_AI_TEST_MODE: process.env.AGNOSTIC_AI_TEST_MODE ?? "" },
  });
  console.log(`Native Problems acceptance fixture: ${root}`);
}
void main().catch(error => { console.error(error); process.exitCode = 1; });
