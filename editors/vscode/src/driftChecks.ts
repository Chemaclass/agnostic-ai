import { ProcessResult } from "./provenance";

export interface DriftContext {
  binary: string;
  cwd: string;
}

export interface DriftClock {
  every(callback: () => void, milliseconds: number): () => void;
}

interface DriftOptions {
  context(): DriftContext | undefined;
  run(context: DriftContext, signal: AbortSignal): Promise<ProcessResult>;
  publish(result: ProcessResult): void;
  pollMilliseconds: number;
  clock?: DriftClock;
}

const clock: DriftClock = {
  every(callback, milliseconds) {
    const timer = setInterval(callback, milliseconds);
    return () => clearInterval(timer);
  },
};

export class DriftChecks {
  private readonly stopPolling: () => void;
  private context: DriftContext | undefined;
  private active: { controller: AbortController; revision: number } | undefined;
  private revision = 0;
  private pending = false;
  private disposed = false;

  constructor(private readonly options: DriftOptions) {
    this.stopPolling = (options.clock ?? clock).every(() => this.request("poll"), options.pollMilliseconds);
  }

  request(reason: "save" | "poll" = "save"): void {
    if (this.disposed) return;
    const context = this.options.context();
    if (context?.cwd !== this.context?.cwd || context?.binary !== this.context?.binary) {
      this.context = context;
      this.revision++;
      this.pending = false;
      this.active?.controller.abort();
    }
    if (reason === "save") this.revision++;
    if (!context) {
      this.pending = false;
      return;
    }
    if (this.active) {
      this.pending = true;
      return;
    }
    void this.check(context);
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    this.pending = false;
    this.stopPolling();
    this.active?.controller.abort();
  }

  private async check(context: DriftContext): Promise<void> {
    const active = { controller: new AbortController(), revision: this.revision };
    this.active = active;
    this.pending = false;
    try {
      let result: ProcessResult;
      try {
        result = await this.options.run(context, active.controller.signal);
      } catch (error) {
        result = { stdout: "", stderr: error instanceof Error ? error.message : String(error), code: 1 };
      }
      if (!this.disposed && !active.controller.signal.aborted && active.revision === this.revision) {
        this.options.publish(result);
      }
    } finally {
      this.active = undefined;
      if (!this.disposed && this.pending) this.request("poll");
    }
  }
}
