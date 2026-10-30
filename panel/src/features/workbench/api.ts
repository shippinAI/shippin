import type { ApiExecutionResult, MeIdentity } from "../../shared/contracts";

function normalizeHeaders(headers: Headers): Record<string, string> {
  const output: Record<string, string> = {};
  headers.forEach((value, key) => {
    output[key] = value;
  });
  return output;
}

export async function executeApiRequest(options: {
  method: string;
  url: string;
  token: string;
  body: string;
}): Promise<ApiExecutionResult> {
  const requestHeaders: Record<string, string> = {
    Accept: "application/json"
  };
  let requestBody: string | null = null;

  if (options.token.trim().length > 0) {
    requestHeaders.Authorization = `Bearer ${options.token.trim()}`;
  }

  const canHaveBody = !["GET", "HEAD"].includes(options.method.toUpperCase());
  if (canHaveBody && options.body.trim().length > 0) {
    requestHeaders["Content-Type"] = "application/json";
    requestBody = options.body;
  }

  const response = await fetch(options.url, {
    method: options.method,
    headers: requestHeaders,
    body: requestBody
  });

  const rawBody = await response.text();
  const safeRequestHeaders = { ...requestHeaders };
  if (safeRequestHeaders.Authorization) {
    safeRequestHeaders.Authorization = "Bearer [REDACTED]";
  }

  return {
    request: {
      method: options.method,
      url: options.url,
      headers: safeRequestHeaders,
      body: requestBody
    },
    response: {
      status: response.status,
      statusText: response.statusText,
      headers: normalizeHeaders(response.headers),
      body: rawBody
    }
  };
}

export async function fetchMe(apiBaseUrl: string, token: string): Promise<MeIdentity> {
  const response = await fetch(`${apiBaseUrl.replace(/\/$/, "")}/me`, {
    method: "GET",
    headers: {
      Accept: "application/json",
      Authorization: `Bearer ${token.trim()}`
    }
  });
  if (!response.ok) {
    const body = await response.text();
    throw new Error(`GET /me failed (${response.status}): ${body || response.statusText}`);
  }
  return (await response.json()) as MeIdentity;
}
