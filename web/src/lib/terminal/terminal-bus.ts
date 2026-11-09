/** A tiny pub/sub between whatever drives the terminal (guides, an assistant) and the terminal view. */
export type TerminalSink = (data: string) => void;

export class TerminalBus {
  private sink: TerminalSink | null = null;
  private pending: string[] = [];

  attach(sink: TerminalSink | null) {
    this.sink = sink;
    if (sink) for (const p of this.pending.splice(0)) sink(p);
  }

  /** Types text into the shell; run=true presses Enter. Queued until the terminal connects. */
  type(text: string, run = true) {
    const data = run ? `${text}\r` : text;
    if (this.sink) this.sink(data);
    else this.pending.push(data);
  }

  get connected() {
    return this.sink !== null;
  }
}
