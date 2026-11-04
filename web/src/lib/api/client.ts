export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

function problemMessage(data: unknown, fallback: string): string {
  const p = data as { message?: string };
  return p?.message || fallback;
}

/** GET a control-plane endpoint; problem+json errors become ApiError. */
export async function apiGet<T>(path: string): Promise<T> {
  const res = await fetch(path, { credentials: "same-origin", headers: { Accept: "application/json" } });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new ApiError(res.status, problemMessage(body, res.statusText));
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
    throw new ApiError(res.status, problemMessage(data, res.statusText));
  }
  return data as T;
}
