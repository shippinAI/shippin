export type ApiOperation = {
  method: string;
  path: string;
  operationId: string;
  summary: string;
  tags: string[];
  implementationStatus: string;
  /** The capability this route needs to be offered; the API contract says which. */
  feature?: string;
};

export type OperationIndex = { basePath: string; operationCount: number; tags: string[]; operations: ApiOperation[] };

export const hasBody = (method: string) => !["GET", "HEAD"].includes(method.toUpperCase());

export function pathParameters(path: string): string[] {
  return (path.match(/\{([^}]+)\}/g) ?? []).map((m) => m.slice(1, -1));
}

/** Resolves each placeholder as exactly one URL segment, so a value cannot reach another route. */
export function resolvePath(path: string, values: Record<string, string>): string {
  return path.replace(/\{([^}]+)\}/g, (_, key: string) => {
    const value = values[key]?.trim();
    if (!value) throw new Error(`Missing required path parameter: ${key}`);
    if (value === "." || value === ".." || /[\\/%?#\x00-\x1f\x7f]/.test(value)) {
      throw new Error(`Invalid path parameter: ${key}. Enter a single unescaped path value.`);
    }
    return encodeURIComponent(value);
  });
}

export function availableOperations(operations: ApiOperation[], caps: Record<string, boolean>): ApiOperation[] {
  return operations.filter((o) => !o.feature || caps[o.feature] === true);
}

export function matchOperations(operations: ApiOperation[], query: string): ApiOperation[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  if (!words.length) return operations;
  return operations.filter((o) => {
    const text = `${o.method} ${o.path} ${o.operationId} ${o.summary} ${o.tags.join(" ")}`.toLowerCase();
    return words.every((w) => text.includes(w));
  });
}

/** A body is either empty or valid JSON, so a typo fails here and not as a confusing API error. */
export function bodyError(body: string): string | null {
  if (!body.trim()) return null;
  try {
    JSON.parse(body);
    return null;
  } catch (e) {
    return `Request body is not valid JSON: ${(e as Error).message}`;
  }
}
