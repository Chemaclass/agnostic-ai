import { ProcessResult } from "./provenance";

export interface DriftStatusBar {
  text: string;
  tooltip: unknown;
  show(): void;
}

interface SyncCheckJSON {
  writes: { target: string; path: string; action: string; bytes: number }[];
  errors: { target: string; message: string }[];
}

const checkHint = "Click to run `agnostic-ai sync --check` in a terminal.";

export function updateDriftStatus(status: DriftStatusBar, result: ProcessResult): void {
  const presentation = driftStatus(result);
  status.text = presentation.text;
  status.tooltip = `${presentation.reason}\n\n${checkHint}`;
  status.show();
}

function driftStatus(result: ProcessResult): { text: string; reason: string } {
  const stderr = result.stderr.trim();
  if (result.code === -1) {
    return {
      text: "$(alert) agnostic-ai not found",
      reason: "Install agnostic-ai on PATH or set agnostic-ai.binaryPath to the installed binary.",
    };
  }
  if (result.stdout.trim() === "") {
    return failed(stderr || "sync --check returned no JSON output. Check the installed agnostic-ai version.");
  }
  let value: unknown;
  try {
    value = JSON.parse(result.stdout);
  } catch {
    return failed(stderr || "sync --check returned invalid JSON. Check the installed agnostic-ai version.");
  }
  if (!isSyncCheck(value)) {
    return failed([
      "sync --check returned an invalid or unsupported result. Expected version 1 sync --check output with writes and errors arrays.",
      stderr,
    ].filter(Boolean).join("\n"));
  }
  if (value.errors.length > 0) {
    return failed(value.errors.map(error => `${error.target}: ${error.message}`).join("\n"));
  }
  const drift = value.writes.length;
  if (result.code !== 0 && !(result.code === 1 && drift > 0)) {
    return failed(stderr || `sync --check failed with exit code ${result.code}.`);
  }
  if (drift > 0) {
    return {
      text: `$(warning) agnostic-ai: ${drift} drifted`,
      reason: `${drift} generated file${drift === 1 ? " differs" : "s differ"} from the specs. Run the check in a terminal for repair instructions.`,
    };
  }
  return { text: "$(check) agnostic-ai: in sync", reason: "Generated files are in sync." };
}

function failed(reason: string): { text: string; reason: string } {
  return { text: "$(error) agnostic-ai: check failed", reason };
}

function isSyncCheck(value: unknown): value is SyncCheckJSON {
  return isObject(value)
    && value.version === "1"
    && value.command === "sync --check"
    && Array.isArray(value.writes)
    && value.writes.every(write => isObject(write)
      && isNonEmptyString(write.target)
      && isNonEmptyString(write.path)
      && isNonEmptyString(write.action)
      && typeof write.bytes === "number"
      && Number.isFinite(write.bytes)
      && write.bytes >= 0)
    && Array.isArray(value.errors)
    && value.errors.every(error => isObject(error)
      && isNonEmptyString(error.target)
      && isNonEmptyString(error.message));
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === "string" && value.trim() !== "";
}
