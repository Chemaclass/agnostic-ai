export interface DiagnosticContext { binary: string; cwd: string }
export interface DiagnosticSession { start(signal: AbortSignal): Promise<void>; stop(): Promise<void> }
export interface DiagnosticDependencies {
  create(context: DiagnosticContext): DiagnosticSession;
  failure(context: DiagnosticContext, error: unknown): void;
}

export class SourceDiagnostics {
  private desired: DiagnosticContext | undefined;
  private version = 0;
  private processed = 0;
  private active: DiagnosticSession | undefined;
  private starting: AbortController | undefined;
  private task: Promise<void> | undefined;
  private disposed = false;

  constructor(private readonly dependencies: DiagnosticDependencies) {}

  update(context: DiagnosticContext | undefined): Promise<void> {
    if (this.disposed) return this.settled();
    if (this.desired?.binary === context?.binary && this.desired?.cwd === context?.cwd) return this.settled();
    this.desired = context;
    this.version++;
    this.starting?.abort();
    this.schedule();
    return this.settled();
  }

  dispose(): Promise<void> {
    this.disposed = true;
    this.desired = undefined;
    this.version++;
    this.starting?.abort();
    this.schedule();
    return this.settled();
  }

  private schedule(): void {
    if (this.task) return;
    this.task = this.run().finally(() => {
      this.task = undefined;
      if (this.processed !== this.version) this.schedule();
    });
  }

  private async settled(): Promise<void> {
    while (this.task) await this.task;
  }

  private async stop(): Promise<void> {
    if (!this.active) return;
    const session = this.active;
    await session.stop();
    this.active = undefined;
  }

  private async run(): Promise<void> {
    while (this.processed !== this.version) {
      const version = this.version;
      const context = this.desired;
      try {
        await this.stop();
      } catch (error) {
        this.processed = this.version;
        if (context) this.dependencies.failure(context, error);
        return;
      }
      if (version !== this.version) continue;
      this.processed = version;
      if (!context) continue;
      const controller = new AbortController();
      this.starting = controller;
      try {
        this.active = this.dependencies.create(context);
        await this.active.start(controller.signal);
      } catch (error) {
        if (version === this.version && !this.disposed) this.dependencies.failure(context, error);
        try { await this.stop(); }
        catch (stopError) {
          this.processed = this.version;
          this.dependencies.failure(context, stopError);
          return;
        }
      } finally {
        this.starting = undefined;
      }
    }
  }
}

export function isDiagnosticFile(uri: { scheme: string; fsPath: string }): boolean {
  return uri.scheme === "file" && /\.(md|mdc|yaml|yml)$/i.test(uri.fsPath);
}
