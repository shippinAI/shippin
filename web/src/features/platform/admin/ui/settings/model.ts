// What the Settings and Secret store pages show and send (CFG-01, docs/settings.md), kept apart from
// React so it is tested on its own. Every type here is the generated one from the actions' Go types.
import type {
  SecretStoreConnection,
  SecretStoreFields,
  SettingUpdateInput,
  SettingsFieldView,
  SettingsView,
  Version,
} from "../../../../../lib/api/actions.gen";
import type { BadgeTone } from "../../../../../lib/design/components/badge/badge";

/** The shortest reason the server accepts for a change. */
export const minReason = 5;
export const reasonOK = (r: string) => r.trim().length >= minReason;

/** The reason a check of the connection is recorded with; a check changes nothing but its own result. */
export const checkReason = "Checked the secret store connection from the Secret store page.";

/** "just now", "5 min ago", "3 h ago", "2 d ago"; "never" for no time. */
export function ago(iso: string | null | undefined, now: number): string {
  if (!iso) return "never";
  const t = Date.parse(iso);
  if (!Number.isFinite(t)) return "never";
  const s = Math.max(0, Math.round((now - t) / 1000));
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)} min ago`;
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`;
  return `${Math.floor(s / 86400)} d ago`;
}

/** A server message as a sentence: a capital first letter and a full stop. */
export function sentence(m: string): string {
  const t = m.trim().replace(/[.\s]+$/, "");
  return t ? `${t[0].toUpperCase()}${t.slice(1)}.` : "";
}

/** Why a request failed, in words a person can act on. kind tells the page what to offer: a reload for
 * "stale", a link to the Secret store page for "no_store". */
export type Problem = { kind: "stale" | "no_store" | "invalid" | "vaultix" | "denied" | "other"; message: string };

export function problemOf(e: unknown): Problem {
  const err = (e ?? {}) as { status?: number; message?: string };
  const m = err.message?.trim() ?? "";
  switch (err.status) {
    case 409:
      return /secret store|vaultix/i.test(m) && !/changed since|saved by someone/i.test(m)
        ? { kind: "no_store", message: sentence(m) || "No secret store is connected. Connect Vaultix on the Secret store page first." }
        : { kind: "stale", message: m || "This changed since you opened it. Reload it, then save again." };
    case 400:
      return { kind: "invalid", message: m || "One of the values isn't accepted." };
    case 502:
      return {
        kind: "vaultix",
        message: m ? `${sentence(m)} Nothing was changed.` : "Vaultix didn't answer. Nothing was changed; try again in a moment.",
      };
    case 403:
      return { kind: "denied", message: "Your staff role can't change settings. Ask for the platform.settings.write permission." };
    case 503:
      return { kind: "other", message: m || "This installation can't do that right now. Try again in a moment." };
  }
  return { kind: "other", message: m || "That didn't work. Try again in a moment." };
}

// ---------------------------------------------------------------- Secret store

export type Status = { label: string; tone: BadgeTone; detail: string };

/** How the connection reads: words first, a tone beside them, never colour alone. */
export function storeStatus(c: SecretStoreConnection | undefined, now: number): Status {
  if (!c) return { label: "Checking…", tone: "neutral", detail: "" };
  if (!c.connected) {
    return {
      label: "Not connected",
      tone: "warning",
      detail: "Keys entered on the Settings page can't be saved until the panel is connected to Vaultix.",
    };
  }
  if (c.check_error) return { label: "Last check failed", tone: "danger", detail: sentence(c.check_error) };
  return {
    label: "Connected",
    tone: "success",
    detail: c.checked_at ? `It worked when last checked, ${ago(c.checked_at, now)}.` : "Not checked yet.",
  };
}

/** What staff type to connect. The client secret is never filled in from the server. */
export type StoreForm = {
  api_url: string;
  project_id: string;
  environment: string;
  path: string;
  client_id: string;
  client_secret: string;
};

export const defaultFolder = "/shippin";

export const storeForm = (c?: SecretStoreConnection): StoreForm => ({
  api_url: c?.connected ? c.api_url : "",
  project_id: c?.connected ? c.project_id : "",
  environment: c?.connected ? c.environment : "",
  path: c?.connected && c.path ? c.path : defaultFolder,
  client_id: c?.connected ? c.client_id : "",
  client_secret: "",
});

/** While settings keep keys in the store, where it is (address, project, environment, folder) can't change. */
export const whereLocked = (c?: SecretStoreConnection) => !!c?.connected && c.keys_held > 0;

/** Whether this form needs a client secret typed in: a new connection, a new identity, or none saved. */
export const secretNeeded = (f: StoreForm, c?: SecretStoreConnection) =>
  !c?.connected || !c.has_client_secret || f.client_id.trim() !== c.client_id;

const envSlug = /^[a-z0-9][a-z0-9_-]{0,63}$/;
const folder = /^(\/[A-Za-z0-9_-]{1,64}){1,8}$/;
const projectId = /^[A-Za-z0-9][A-Za-z0-9_-]{0,99}$/;

/** What is wrong with the form before it is sent, the same rules the server applies; null when none. */
export function storeFormProblem(f: StoreForm, c?: SecretStoreConnection): string | null {
  const url = parseURL(f.api_url.trim());
  if (!url) return "Enter the Vaultix address, such as https://vaultix.example.com.";
  if (!projectId.test(f.project_id.trim())) return "Enter the Vaultix project ID, from the project's settings.";
  if (!envSlug.test(f.environment.trim())) return "Enter the environment's slug, such as prod.";
  if (!folder.test(f.path.trim() || defaultFolder))
    return "The folder must look like /shippin: letters, digits, dashes and underscores, up to eight levels.";
  if (!f.client_id.trim()) return "Enter the machine identity's client ID.";
  if (secretNeeded(f, c) && !f.client_secret.trim()) return "Enter the machine identity's client secret.";
  if (
    whereLocked(c) &&
    c &&
    (body(f).api_url !== c.api_url ||
      body(f).project_id !== c.project_id ||
      body(f).environment !== c.environment ||
      body(f).path !== c.path)
  ) {
    return "Settings keep keys in this store, so its address, project, environment and folder can't change. Only the identity can.";
  }
  return null;
}

function body(f: StoreForm): SecretStoreFields {
  return {
    api_url: f.api_url.trim().replace(/\/+$/, ""),
    project_id: f.project_id.trim(),
    environment: f.environment.trim(),
    path: f.path.trim() || defaultFolder,
    client_id: f.client_id.trim(),
  };
}

/** The update's body: trimmed, with the client secret only when one was typed (left out keeps the saved one). */
export function storeBody(f: StoreForm): SecretStoreFields {
  const secret = f.client_secret.trim();
  return secret ? { ...body(f), client_secret: secret } : body(f);
}

/** Whether the form differs from what is saved. */
export function storeChanged(f: StoreForm, c?: SecretStoreConnection): boolean {
  if (!c?.connected) return true;
  const b = body(f);
  return (
    !!f.client_secret.trim() ||
    b.api_url !== c.api_url ||
    b.project_id !== c.project_id ||
    b.environment !== c.environment ||
    b.path !== c.path ||
    b.client_id !== c.client_id
  );
}

/** Why disconnecting is refused now; null when it may go ahead. */
export function disconnectBlock(c?: SecretStoreConnection): string | null {
  if (!c?.connected) return "No secret store is connected.";
  if (c.keys_held > 0) {
    const n = c.keys_held === 1 ? "One settings section keeps" : `${c.keys_held} settings sections keep`;
    return `${n} keys in this store. Remove those keys on the Settings page first, or the panel couldn't read them any more.`;
  }
  return null;
}

/** Where the store is, in one line. */
export const storeWhere = (c: SecretStoreConnection) =>
  `${c.api_url}, project ${c.project_id}, environment ${c.environment}, folder ${c.path || defaultFolder}`;

// ---------------------------------------------------------------- Settings sections

export const isKey = (f: Pick<SettingsFieldView, "kind">) => f.kind === "key" || f.kind === "long_key";

/** Where a field's value comes from now. */
export type Origin = { label: string; tone: BadgeTone; hint: string };

export function fieldOrigin(view: Pick<SettingsView, "version">, f: SettingsFieldView): Origin {
  if (isKey(f)) {
    switch (f.key?.in) {
      case "secret_store":
        return { label: "In Vaultix", tone: "success", hint: f.key.name ? `Saved in Vaultix as ${f.key.name}.` : "Saved in Vaultix." };
      case "environment":
        return {
          label: "In the environment",
          tone: "warning",
          hint: `Read from ${f.env ? `the ${f.env} environment variable` : "the panel's environment"}. Move it to Vaultix to change it here.`,
        };
    }
    return { label: "Not set", tone: "neutral", hint: "No key is saved." };
  }
  if (view.version > 0) {
    if (f.file_value === undefined) return { label: "Set in the panel", tone: "primary", hint: "The bootstrap file doesn't set it." };
    const differs = !same(f.value, f.file_value);
    return {
      label: "Set in the panel",
      tone: "primary",
      hint: differs ? `The bootstrap file has ${displayValue(f, f.file_value)}.` : "The same as the bootstrap file.",
    };
  }
  if (f.file_value !== undefined)
    return {
      label: "From the file",
      tone: "neutral",
      hint: "From the bootstrap file. Saving this section here makes the panel's value the one in force.",
    };
  return { label: "Default", tone: "neutral", hint: "The built-in default." };
}

/** A value as a person reads it. */
export function displayValue(f: Pick<SettingsFieldView, "kind" | "choices">, v: unknown): string {
  if (v === undefined || v === null || v === "") return "nothing";
  if (f.kind === "bool") return v === true ? "on" : "off";
  if (Array.isArray(v)) return v.length ? v.join(", ") : "an empty list";
  if (typeof v === "object") {
    const e = Object.entries(v as Record<string, unknown>);
    return e.length ? e.map(([k, x]) => `${k}=${String(x)}`).join(", ") : "nothing";
  }
  if (f.kind === "choice") return f.choices?.find((c) => c.value === v)?.label ?? String(v);
  return String(v);
}

/** What an input holds: a switch's on or off, otherwise text. */
export type Input = string | boolean;

/** A field's value as its input shows it: a list one entry per line, a map one name=value per line. */
export function toInput(f: Pick<SettingsFieldView, "kind">, v: unknown): Input {
  switch (f.kind) {
    case "bool":
      return v === true;
    case "list":
      return Array.isArray(v) ? v.map(String).join("\n") : "";
    case "map":
      return v && typeof v === "object" && !Array.isArray(v)
        ? Object.entries(v as Record<string, unknown>)
            .map(([k, x]) => `${k}=${String(x)}`)
            .join("\n")
        : "";
  }
  return v === undefined || v === null ? "" : String(v);
}

const mapName = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$/;
const control = /[\u0000-\u001f\u007f]/;

/** An input read back as the value the server takes, with the same rules it checks; or what is wrong. */
export function fromInput(f: SettingsFieldView, input: Input): { value: unknown } | { error: string } {
  const label = f.label;
  if (f.kind === "bool") return { value: input === true };
  const s = typeof input === "string" ? input.trim() : "";
  switch (f.kind) {
    case "integer": {
      if (!/^-?\d+$/.test(s)) return { error: `${label} must be a whole number.` };
      const n = Number(s);
      const min = f.min ?? 0;
      const max = f.max ?? 0;
      if (n < min || (max !== 0 && n > max))
        return { error: max !== 0 ? `${label} must be between ${min} and ${max}.` : `${label} must be at least ${min}.` };
      return { value: n };
    }
    case "list": {
      const items = s
        .split(/\r?\n/)
        .map((x) => x.trim())
        .filter(Boolean);
      if (items.length > 200) return { error: `${label} can have at most 200 entries.` };
      if (items.some((x) => x.length > 200)) return { error: `${label}: each entry must be at most 200 characters.` };
      return { value: items };
    }
    case "map": {
      const out: Record<string, string> = {};
      for (const line of s
        .split(/\r?\n/)
        .map((x) => x.trim())
        .filter(Boolean)) {
        const at = line.indexOf("=");
        const k = at < 0 ? "" : line.slice(0, at).trim();
        const v = at < 0 ? "" : line.slice(at + 1).trim();
        if (!mapName.test(k))
          return { error: `${label}: write each line as name=value, with a short name of letters, digits, dots, dashes or underscores.` };
        if (v.length > 200) return { error: `${label}: the value of ${k} is longer than 200 characters.` };
        out[k] = v;
      }
      if (Object.keys(out).length > 100) return { error: `${label} can have at most 100 names.` };
      return { value: out };
    }
  }
  // text, long_text, url, choice
  const limit = f.max && f.max > 0 ? f.max : f.kind === "long_text" ? 64 << 10 : 1000;
  if (!s && !f.optional) return { error: `${label} is required.` };
  if (s.length > limit) return { error: `${label} is longer than ${limit} characters.` };
  if (f.kind !== "long_text" && control.test(s)) return { error: `${label} must be one line.` };
  if (s && f.kind === "url" && !parseURL(s))
    return { error: `${label} must be an http or https address without a password, query or fragment.` };
  if (f.kind === "choice" && !(f.choices ?? []).some((c) => c.value === s)) return { error: `${label}: pick one of the choices.` };
  return { value: s };
}

function parseURL(s: string): URL | null {
  try {
    const u = new URL(s);
    if ((u.protocol !== "https:" && u.protocol !== "http:") || !u.hostname || u.username || u.password || u.search || u.hash) return null;
    return u;
  } catch {
    return null;
  }
}

/** Two JSON values with the same content; object key order doesn't matter. */
export function same(a: unknown, b: unknown): boolean {
  return stable(a) === stable(b);
}

function stable(v: unknown): string {
  if (Array.isArray(v)) return `[${v.map(stable).join(",")}]`;
  if (v && typeof v === "object") {
    return `{${Object.keys(v as object)
      .sort()
      .map((k) => `${JSON.stringify(k)}:${stable((v as Record<string, unknown>)[k])}`)
      .join(",")}}`;
  }
  return JSON.stringify(v ?? null);
}

/** A pending change to one key: a new value, removing it, or moving it from the environment into Vaultix. */
export type KeyEdit = { op: "replace"; value: string } | { op: "remove" } | { op: "import" };

/** The form's state: each field's input, and the keys being changed. */
export type Draft = { inputs: Record<string, Input>; keys: Record<string, KeyEdit> };

export function initialDraft(view: SettingsView): Draft {
  const inputs: Record<string, Input> = {};
  for (const f of view.fields) if (!isKey(f)) inputs[f.name] = toInput(f, f.value);
  return { inputs, keys: {} };
}

/** The update body as the server decodes it: a key set to null is removed. The generated type can't say
 * so (it reads the map's *string as string), so the page casts this one field when it sends it. */
export type SettingPatch = Omit<SettingUpdateInput, "keys"> & { keys?: Record<string, string | null> };

export type Plan = {
  body: SettingPatch;
  /** The labels of what the person changed, for the save dialog. */
  changed: string[];
  /** Keys that move from the environment into Vaultix with this save without being asked one by one:
   * a section's first save stops reading the environment, so they would stop working otherwise. */
  moved: string[];
  /** Each field's problem, by name. */
  errors: Record<string, string>;
  /** Why this can't be saved at all; null when it can. */
  blocked: string | null;
};

/** Only what changed, against the section's version. Untouched fields are never sent. */
export function planSave(view: SettingsView, draft: Draft, storeConnected: boolean): Plan {
  const values: Record<string, unknown> = {};
  const keys: Record<string, string | null> = {};
  const imports: string[] = [];
  const changed: string[] = [];
  const moved: string[] = [];
  const errors: Record<string, string> = {};
  let writes = false;
  for (const f of view.fields) {
    if (isKey(f)) {
      const edit = draft.keys[f.name];
      if (!edit) {
        if (view.version === 0 && f.key?.in === "environment") {
          imports.push(f.name);
          moved.push(f.label);
          writes = true;
        }
        continue;
      }
      if (edit.op === "remove") {
        keys[f.name] = null;
        changed.push(`${f.label} (removed)`);
      } else if (edit.op === "import") {
        imports.push(f.name);
        changed.push(`${f.label} (moved to Vaultix)`);
        writes = true;
      } else {
        const v = f.kind === "key" ? edit.value.trim() : edit.value;
        if (!v.trim()) errors[f.name] = `Enter the new ${f.label.toLowerCase()}, or cancel replacing it.`;
        else if (v.length > 64 << 10) errors[f.name] = `${f.label} is longer than 64 KiB.`;
        else if (f.kind === "key" && control.test(v)) errors[f.name] = `${f.label} must be one line.`;
        else {
          keys[f.name] = v;
          changed.push(`${f.label} (new key)`);
          writes = true;
        }
      }
      continue;
    }
    const input = draft.inputs[f.name];
    if (input === undefined || input === toInput(f, f.value)) continue;
    const r = fromInput(f, input);
    if ("error" in r) {
      errors[f.name] = r.error;
      continue;
    }
    const before = fromInput(f, toInput(f, f.value));
    if ("value" in before && same(before.value, r.value)) continue;
    values[f.name] = r.value;
    changed.push(f.label);
  }
  let blocked: string | null = null;
  if (writes && !storeConnected) {
    blocked =
      moved.length && !changed.some((c) => c.endsWith("(new key)") || c.endsWith("(moved to Vaultix)"))
        ? `Saving this section for the first time moves ${list(moved)} from the environment into Vaultix, so connect the secret store first.`
        : "Keys are saved in Vaultix, so connect the secret store first.";
  }
  const body: SettingPatch = { version: view.version };
  if (Object.keys(values).length) body.values = values;
  if (Object.keys(keys).length) body.keys = keys;
  if (imports.length) body.import = imports;
  return { body, changed, moved, errors, blocked };
}

/** "a", "a and b", "a, b and c". */
export function list(items: string[]): string {
  if (items.length <= 1) return items[0] ?? "";
  return `${items.slice(0, -1).join(", ")} and ${items[items.length - 1]}`;
}

/** Whether a plan has anything to send. */
export const hasChanges = (p: Plan) => p.changed.length > 0;

/** One choice for a field that picks from a panel list ("fleets"). The empty value has its own id,
 * because a list box can't key an option by "". */
export const noneId = "__none__";
export type PickOption = { id: string; label: string };

export function pickOptions(items: { id: string; label: string }[], current: string, optional: boolean): PickOption[] {
  const out: PickOption[] = optional ? [{ id: noneId, label: "None" }] : [];
  for (const i of items) out.push({ id: i.id, label: i.label || i.id });
  if (current && !items.some((i) => i.id === current)) out.push({ id: current, label: `${current} (not found)` });
  return out;
}

// ---------------------------------------------------------------- History

export type HistoryEntry = { version: number; at: string; by: string; reason: string; changes: string[] };

/** Each saved version, newest first, with what it changed against the one before it (the first against
 * the bootstrap file). Keys are named, never shown. */
export function historyEntries(view: Pick<SettingsView, "fields">, versions: Version[]): HistoryEntry[] {
  const byName = new Map(view.fields.map((f) => [f.name, f]));
  const sorted = [...versions].sort((a, b) => a.version - b.version);
  const out: HistoryEntry[] = [];
  let prev: Record<string, unknown> | null = null;
  for (const v of sorted) {
    const vals = (v.values && typeof v.values === "object" ? v.values : {}) as Record<string, unknown>;
    const changes: string[] = [];
    for (const f of view.fields) {
      if (isKey(f) || !(f.name in vals)) continue;
      const before = prev ? prev[f.name] : f.file_value;
      if (prev === null && f.file_value === undefined) continue;
      if (!same(before, vals[f.name])) changes.push(`${f.label}: ${displayValue(f, before)} → ${displayValue(f, vals[f.name])}`);
    }
    for (const k of v.keys_changed ?? []) changes.push(`${byName.get(k)?.label ?? k}: key changed`);
    out.push({ version: v.version, at: v.changed_at, by: v.changed_by, reason: v.reason, changes });
    prev = vals;
  }
  return out.reverse();
}
