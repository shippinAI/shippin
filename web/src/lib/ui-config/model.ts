import { textDefaults, type TextKey } from "./text-keys";

export type NavEntryConfig = { id: string; label?: string; hidden?: boolean; order?: number };
export type ProviderFieldConfig = { name: string; label?: string; help?: string; placeholder?: string; default?: string };
export type ProviderConfig = { kind: string; label?: string; description?: string; hidden?: boolean; fields?: ProviderFieldConfig[] };
export type DriverSpec = { kind: string; fields: { name: string; required: boolean }[]; capabilities: string[]; verbs: string[] };

/** GET /v1/settings/ui: the config file with the ops override merged over it. */
export type UiConfig = {
  nav?: { sections?: NavEntryConfig[]; items?: NavEntryConfig[] };
  text?: Record<string, string>;
  providers?: ProviderConfig[];
  default_site?: string;
  drivers?: DriverSpec[];
};

/** Applies renames, hiding and order to built-in entries. Order 0 keeps an entry's own place. */
export function applyNav<T extends { id: string; label: string }>(items: T[], overrides: NavEntryConfig[] = []): T[] {
  const byId = new Map(overrides.map((o) => [o.id, o]));
  return items
    .map((item, index) => {
      const o = byId.get(item.id);
      return { item: o?.label ? { ...item, label: o.label } : item, hidden: !!o?.hidden, rank: o?.order || index + 1, index };
    })
    .filter((e) => !e.hidden)
    .sort((a, b) => a.rank - b.rank || a.index - b.index)
    .map((e) => e.item);
}

/** Fills {name} placeholders. An unknown placeholder stays visible rather than vanishing. */
export function format(template: string, vars: Record<string, string | number> = {}): string {
  return template.replace(/\{(\w+)\}/g, (whole, name: string) => (name in vars ? String(vars[name]) : whole));
}

export function textFor(ui: UiConfig | undefined, key: TextKey, vars?: Record<string, string | number>): string {
  return format(ui?.text?.[key] || textDefaults[key], vars);
}

/** What gets stored: only values someone actually set, so later defaults still reach this site. */
export function compactUi(draft: UiConfig): UiConfig {
  const nav = (entries: NavEntryConfig[] = []) =>
    entries
      .map((e) => ({ id: e.id, ...(e.label?.trim() ? { label: e.label.trim() } : {}), ...(e.hidden ? { hidden: true } : {}), ...(e.order ? { order: e.order } : {}) }))
      .filter((e) => Object.keys(e).length > 1);
  const providers = (draft.providers ?? [])
    .map((p) => ({
      kind: p.kind,
      ...(p.label?.trim() ? { label: p.label.trim() } : {}),
      ...(p.description?.trim() ? { description: p.description.trim() } : {}),
      ...(p.hidden ? { hidden: true } : {}),
      fields: (p.fields ?? [])
        .map((f) => ({
          name: f.name,
          ...(f.label?.trim() ? { label: f.label.trim() } : {}),
          ...(f.help?.trim() ? { help: f.help.trim() } : {}),
          ...(f.placeholder?.trim() ? { placeholder: f.placeholder.trim() } : {}),
          ...(f.default?.trim() ? { default: f.default.trim() } : {}),
        }))
        .filter((f) => Object.keys(f).length > 1),
    }))
    .filter((p) => Object.keys(p).length > 2 || p.fields.length > 0);
  const text = Object.fromEntries(Object.entries(draft.text ?? {}).filter(([, v]) => v.trim()).map(([k, v]) => [k, v.trim()]));
  return {
    nav: { sections: nav(draft.nav?.sections), items: nav(draft.nav?.items) },
    text,
    providers,
    ...(draft.default_site?.trim() ? { default_site: draft.default_site.trim() } : {}),
  };
}
