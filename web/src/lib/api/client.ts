export class ApiError extends Error {
  code?: string;
  cardKey?: string;
  cardId?: string;
  constructor(public status: number, message: string, extra?: { code?: string; card_key?: string; card_id?: string }) {
    super(message);
    this.code = extra?.code;
    this.cardKey = extra?.card_key;
    this.cardId = extra?.card_id;
  }
}

/** A change that is waiting for approval. It has not failed, and the page offers a way to approve it. */
export const isWaitingOnBoard = (e: unknown): e is ApiError => e instanceof ApiError && e.code === "waiting_on_board";

function problemMessage(data: unknown, fallback: string): { message: string; code?: string; card_key?: string; card_id?: string } {
  const p = data as { message?: string; code?: string; card_key?: string; card_id?: string };
  return { message: p?.message || fallback, code: p?.code, card_key: p?.card_key, card_id: p?.card_id };
}

/** GET a control-plane endpoint; problem+json errors become ApiError. */
export async function apiGet<T>(path: string): Promise<T> {
  const res = await fetch(path, { credentials: "same-origin", headers: { Accept: "application/json" } });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    const p = problemMessage(body, res.statusText);
    throw new ApiError(res.status, p.message, p);
  }
  return (await res.json()) as T;
}

/** POST/PUT with JSON; returns the parsed body (problem+json → ApiError). */
export async function apiSend<T>(method: "POST" | "PUT" | "PATCH" | "DELETE", path: string, body?: unknown, options?: { note?: string; ifMatch?: string }): Promise<T> {
  const res = await fetch(path, {
    method,
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json", Accept: "application/json", "Idempotency-Key": crypto.randomUUID(),
      ...(options?.ifMatch ? { "If-Match": options.ifMatch } : {}),
      // Why this change is being made. Percent-encoded so that any language survives a header.
      ...(options?.note ? { "X-Change-Note": encodeURIComponent(options.note) } : {}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = res.status === 204 ? {} : await res.json().catch(() => ({}));
  if (!res.ok) {
    const p = problemMessage(data, res.statusText);
    throw new ApiError(res.status, p.message, p);
  }
  return data as T;
}
