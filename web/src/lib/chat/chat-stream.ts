import { ApiError } from "../api/client";
import type { ChatMessage } from "./chat-types";

export type StreamEvent =
  | { event: "user"; data: ChatMessage }
  | { event: "delta"; data: { text: string } }
  | { event: "done"; data: ChatMessage }
  | { event: "error"; data: { detail: string } };

/** POSTs a message and reads the server-sent events of the reply as they arrive. */
export async function streamMessage(sessionId: string, text: string, on: (e: StreamEvent) => void, signal?: AbortSignal) {
  const res = await fetch(`/v1/chat/sessions/${sessionId}/messages`, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", Accept: "text/event-stream" },
    body: JSON.stringify({ text }),
    signal,
  });
  if (!res.ok || !res.body) {
    const p = await res.json().catch(() => ({}));
    throw new ApiError(res.status, (p as { message?: string }).message ?? res.statusText);
  }
  const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
  let buf = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buf += value;
    let i;
    while ((i = buf.indexOf("\n\n")) >= 0) {
      const frame = buf.slice(0, i);
      buf = buf.slice(i + 2);
      const ev = /^event: (.+)$/m.exec(frame)?.[1];
      const data = /^data: (.+)$/m.exec(frame)?.[1];
      if (ev && data) on({ event: ev, data: JSON.parse(data) } as StreamEvent);
    }
  }
}
