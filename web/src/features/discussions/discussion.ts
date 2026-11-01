/** Portable, read-only record of exchanged messages, never an authorization grant. */
export type DiscussionStatus = "awaiting_reply" | "working" | "complete" | "blocked";
export type DiscussionMessage = {
  id: string; sender: string; recipient: string; kind: "message" | "result";
  content: string; observed_at: string;
};
export type Discussion = {
  schema: "shippin.agent_discussion.v1"; id: string; title: string;
  status: DiscussionStatus; captured_at: string; source_url: string;
  messages: DiscussionMessage[];
};
export const discussionStatuses: Record<DiscussionStatus, string> = {
  awaiting_reply: "Awaiting reply", working: "Working", complete: "Complete", blocked: "Blocked",
};
const invalid = () => new Error("Invalid discussion snapshot");
function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw invalid();
  return value as Record<string, unknown>;
}
function text(value: unknown, limit: number): string {
  if (typeof value !== "string" || !value.trim() || value.length > limit) throw invalid();
  return value;
}
function identifier(value: unknown): string {
  const s = text(value, 128);
  if (!/^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(s)) throw invalid();
  return s;
}
function timestamp(value: unknown): string {
  const s = text(value, 24);
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{3})?Z$/.test(s)) throw invalid();
  const date = new Date(s);
  if (!Number.isFinite(date.getTime()) || date.toISOString().replace(".000Z", "Z") !== s.replace(".000Z", "Z")) throw invalid();
  return s;
}
function sourceURL(value: unknown, allowedOrigins: readonly string[]): string {
  const s = text(value, 2048);
  if (s !== s.trim() || /[\s\\]/.test(s)) throw invalid();
  const url = new URL(s);
  if (url.protocol !== "https:" || url.username || url.password || url.search || url.hash || !allowedOrigins.includes(url.origin)) throw invalid();
  return url.href;
}

/** Validate at the adapter boundary; unknown fields are discarded, not forwarded. */
export function parseDiscussion(value: unknown, allowedOrigins: readonly string[]): Discussion {
  const d = record(value);
  if (d.schema !== "shippin.agent_discussion.v1") throw invalid();
  if (typeof d.status !== "string" || !Object.hasOwn(discussionStatuses, d.status)) throw invalid();
  if (!Array.isArray(d.messages) || d.messages.length > 100) throw invalid();
  const ids = new Set<string>();
  const capturedAt = timestamp(d.captured_at);
  let previous = -Infinity;
  const messages = d.messages.map((value): DiscussionMessage => {
    const m = record(value), id = identifier(m.id), observedAt = timestamp(m.observed_at);
    const time = Date.parse(observedAt);
    if (ids.has(id) || time < previous || time > Date.parse(capturedAt)) throw invalid();
    if (m.kind !== "message" && m.kind !== "result") throw invalid();
    ids.add(id); previous = time;
    return { id, sender: text(m.sender, 120), recipient: text(m.recipient, 120), kind: m.kind,
      content: text(m.content, 10000), observed_at: observedAt };
  });
  return { schema: d.schema, id: identifier(d.id), title: text(d.title, 200),
    status: d.status as DiscussionStatus, captured_at: capturedAt,
    source_url: sourceURL(d.source_url, allowedOrigins), messages };
}
