import { Button } from "../../lib/design/components/button/button";
import { getPath, resolve } from "../../lib/a2ui/a2ui-model";
import type { A2UIComponent, ChildList } from "../../lib/a2ui/a2ui-types";
import { A2UIInput } from "./a2ui-input";
import { useSurface } from "./a2ui-surface";

const text = { h1: "text-xl font-semibold", h2: "text-lg font-semibold", h3: "text-[15px] font-semibold", h4: "text-[13.5px] font-semibold",
  h5: "text-[13px] font-semibold", caption: "text-[11px] font-medium uppercase tracking-wide text-fg-subtle", body: "text-[13px] text-fg-muted" };

/** Expands a ChildList: static ids, or a template repeated over a data array (relative scope). */
function children(list: ChildList, data: unknown, scope: string): { id: string; scope: string; key: string }[] {
  if (Array.isArray(list)) return list.map((id) => ({ id, scope, key: id }));
  const arr = getPath(data, list.path, scope);
  const base = list.path.startsWith("/") ? list.path : `${scope}/${list.path}`;
  return Array.isArray(arr) ? arr.map((_, i) => ({ id: list.componentId, scope: `${base}/${i}`, key: `${list.componentId}-${i}` })) : [];
}

/** One component by id. Missing ids render nothing (progressive rendering). */
export function A2UINode({ id, scope }: { id: string; scope: string }) {
  const { surface, onAction } = useSurface();
  const c: A2UIComponent | undefined = surface.components[id];
  if (!c) return null;
  const d = surface.data;
  switch (c.component) {
    case "Text": {
      const v = String(resolve(c.text, d, scope) ?? "");
      const cls = text[c.variant ?? "body"];
      return c.variant?.startsWith("h") ? <p role="heading" aria-level={Number(c.variant[1])} className={`${cls} text-fg`}>{v}</p> : <p className={`whitespace-pre-line ${cls}`}>{v}</p>;
    }
    case "Code":
      return <pre className="overflow-x-auto rounded-[var(--radius-control)] bg-canvas px-3 py-2 font-mono text-[12px] text-fg">{String(resolve(c.text, d, scope) ?? "")}</pre>;
    case "Card":
      return <div className="space-y-3 rounded-[var(--radius-card)] border border-line bg-surface-2 p-4"><A2UINode id={c.child} scope={scope} /></div>;
    case "Divider":
      return <hr className="border-line" />;
    case "Column":
    case "Row":
    case "List": {
      const kids = children(c.children, d, scope);
      const cls = c.component === "Row" ? "flex flex-wrap items-center gap-2" : c.component === "List" ? "space-y-2" : "space-y-3";
      const Tag = c.component === "List" ? "ul" : "div";
      return (
        <Tag className={cls}>
          {kids.map((k) => (c.component === "List" ? <li key={k.key}><A2UINode id={k.id} scope={k.scope} /></li> : <A2UINode key={k.key} id={k.id} scope={k.scope} />))}
        </Tag>
      );
    }
    case "Button": {
      const press = () => {
        const a = c.action;
        if ("functionCall" in a) {
          const url = resolve(a.functionCall.args.url, d, scope);
          if (url && /^(https:\/\/|\/)/.test(url)) window.open(url, "_blank", "noopener");
          return;
        }
        const context: Record<string, unknown> = {};
        for (const [k, v] of Object.entries(a.event.context ?? {})) context[k] = resolve(v as never, d, scope);
        onAction({ name: a.event.name, surfaceId: surface.id, sourceComponentId: c.id, timestamp: new Date().toISOString(), context });
      };
      const variant = c.variant === "primary" ? "primary" : c.variant === "borderless" ? "ghost" : "secondary";
      return <Button size="sm" variant={variant} tooltip={c.tooltip ?? "Run this action"} onPress={press}><A2UINode id={c.child} scope={scope} /></Button>;
    }
    default:
      return <A2UIInput c={c} scope={scope} />;
  }
}
