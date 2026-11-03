import { SHIPPIN_CATALOG, type A2UIComponent, type A2UIMessage, type Dynamic, type Surface } from "./a2ui-types";

/** Surfaces by id. Pure functions so the renderer and tests share one state machine. */
export type Surfaces = Record<string, Surface>;

const KNOWN = new Set(["Text", "Button", "Card", "Column", "Row", "List", "TextField", "CheckBox", "ChoicePicker", "Divider", "Code"]);
const CATALOGS = new Set([SHIPPIN_CATALOG, "https://a2ui.org/specification/v0_9/catalogs/basic/catalog.json"]);

const segs = (p: string) => p.split("/").filter(Boolean).map((s) => s.replace(/~1/g, "/").replace(/~0/g, "~"));

/** JSON Pointer read (RFC 6901); A2UI relative paths resolve against `scope`. */
export function getPath(data: unknown, path: string, scope = ""): unknown {
  const full = path.startsWith("/") ? path : `${scope}/${path}`;
  let cur: unknown = data;
  for (const s of segs(full)) {
    if (cur == null || typeof cur !== "object") return undefined;
    cur = (cur as Record<string, unknown>)[s];
  }
  return cur;
}

/** Immutable JSON Pointer write; omitting value removes the key (A2UI upsert semantics). */
export function setPath(data: unknown, path: string, value: unknown): unknown {
  const parts = segs(path);
  if (!parts.length) return value;
  const [head, ...rest] = parts;
  const obj = (data && typeof data === "object" ? data : {}) as Record<string, unknown>;
  const copy: Record<string, unknown> = Array.isArray(obj) ? ([...obj] as unknown as Record<string, unknown>) : { ...obj };
  const next = rest.length ? setPath(obj[head], `/${rest.join("/")}`, value) : value;
  if (next === undefined && !rest.length) delete copy[head];
  else copy[head] = next;
  return copy;
}

/** Literal or {path} → value. */
export function resolve<T>(v: Dynamic<T> | undefined, data: unknown, scope = ""): T | undefined {
  if (v && typeof v === "object" && "path" in (v as object)) return getPath(data, (v as { path: string }).path, scope) as T;
  return v as T | undefined;
}

/** Unknown component types and catalogs are dropped, never rendered. */
export function isRenderable(c: A2UIComponent): boolean {
  return typeof c?.id === "string" && KNOWN.has(c.component);
}

export function applyMessage(s: Surfaces, m: A2UIMessage): Surfaces {
  if (m.version !== "v0.9") return s;
  if ("createSurface" in m) {
    const { surfaceId, catalogId } = m.createSurface;
    if (s[surfaceId] || !CATALOGS.has(catalogId)) return s;
    return { ...s, [surfaceId]: { id: surfaceId, catalogId, components: {}, data: {} } };
  }
  if ("updateComponents" in m) {
    const cur = s[m.updateComponents.surfaceId];
    if (!cur) return s;
    const components = { ...cur.components };
    for (const c of m.updateComponents.components) if (isRenderable(c)) components[c.id] = c;
    return { ...s, [cur.id]: { ...cur, components } };
  }
  if ("updateDataModel" in m) {
    const cur = s[m.updateDataModel.surfaceId];
    if (!cur) return s;
    return { ...s, [cur.id]: { ...cur, data: setPath(cur.data, m.updateDataModel.path ?? "/", m.updateDataModel.value) } };
  }
  if ("deleteSurface" in m) {
    const rest = { ...s };
    delete rest[m.deleteSurface.surfaceId];
    return rest;
  }
  return s;
}

export const applyAll = (ms: A2UIMessage[], s: Surfaces = {}) => ms.reduce(applyMessage, s);

/** Parses a JSONL stream; malformed lines are skipped. */
export function parseJSONL(text: string): A2UIMessage[] {
  const out: A2UIMessage[] = [];
  for (const line of text.split("\n")) {
    if (!line.trim()) continue;
    try { out.push(JSON.parse(line) as A2UIMessage); } catch { /* skip */ }
  }
  return out;
}
