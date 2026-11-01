import { A2UI_VERSION, SHIPPIN_CATALOG, type A2UIComponent, type A2UIMessage } from "../a2ui/a2ui-types";
import type { Guide, StepStatus } from "./guide-types";

/** Clyffy's guides as A2UI v0.9 messages. The panel renders only these, so an agent can send the
 * same JSON later. Actions: open{guide}, run{step}, next, goto{step}, link{to}. */
const v = A2UI_VERSION;
const create = (surfaceId: string): A2UIMessage => ({ version: v, createSurface: { surfaceId, catalogId: SHIPPIN_CATALOG } });
const comps = (surfaceId: string, components: A2UIComponent[]): A2UIMessage => ({ version: v, updateComponents: { surfaceId, components } });

const label = (id: string, text: string): A2UIComponent => ({ id, component: "Text", text });
const button = (id: string, text: string, name: string, context: Record<string, unknown>, tooltip: string, variant: "primary" | "default" | "borderless" = "default"): A2UIComponent[] => [
  label(`${id}-label`, text),
  { id, component: "Button", child: `${id}-label`, variant, tooltip, action: { event: { name, context } } },
];

/** Guide list: a List templated over /guides in the data model. */
export function pickerMessages(guides: Guide[]): A2UIMessage[] {
  const s = "node-picker";
  return [
    create(s),
    comps(s, [
      { id: "root", component: "Column", children: ["intro", "list"] },
      { id: "intro", component: "Text", text: "Pick a guide. I'll type each command into the terminal for you and explain what it did." },
      { id: "list", component: "List", children: { path: "/guides", componentId: "item" } },
      { id: "item", component: "Card", child: "item-col" },
      { id: "item-col", component: "Column", children: ["item-title", "item-summary", "item-open"] },
      { id: "item-title", component: "Text", text: { path: "title" }, variant: "h4" },
      { id: "item-summary", component: "Text", text: { path: "summary" } },
      { id: "item-open-label", component: "Text", text: "Start guide" },
      { id: "item-open", component: "Button", child: "item-open-label", tooltip: "Start this guide", action: { event: { name: "open", context: { guide: { path: "id" } } } } },
    ]),
    { version: v, updateDataModel: { surfaceId: s, path: "/guides", value: guides.map((g) => ({ id: g.id, title: g.title, summary: g.summary })) } },
  ];
}

/** One guide: the active step as a Card; the others as compact "go to" buttons. */
export function guideMessages(g: Guide, index: number, status: Record<string, StepStatus>): A2UIMessage[] {
  const s = `node-${g.id}`;
  const out: A2UIComponent[] = [];
  const rows: string[] = [];
  g.steps.forEach((st, i) => {
    const p = `s${i}`;
    rows.push(p);
    const mark = status[st.id] === "done" ? "✓" : String(i + 1);
    if (i !== index) {
      out.push(...button(p, `${mark}  ${st.title}`, "goto", { step: i }, "Go to this step", "borderless"));
      return;
    }
    const ran = (status[st.id] ?? "todo") !== "todo";
    const kids = [`${p}-n`, `${p}-t`, `${p}-b`];
    out.push({ id: p, component: "Card", child: `${p}-col` }, { id: `${p}-n`, component: "Text", text: `Step ${i + 1} of ${g.steps.length}`, variant: "caption" },
      { id: `${p}-t`, component: "Text", text: st.title, variant: "h4" }, label(`${p}-b`, st.body));
    if (st.command) { kids.push(`${p}-cmd`); out.push({ id: `${p}-cmd`, component: "Code", text: st.command }); }
    const acts: string[] = [];
    if (st.command) { acts.push(`${p}-run`); out.push(...button(`${p}-run`, ran ? "Run again" : "Run this", "run", { step: i }, st.edit ? "Types it in; edit, then press Enter" : "Types it into the terminal and runs it", ran ? "default" : "primary")); }
    if (st.link) { acts.push(`${p}-link`); out.push(...button(`${p}-link`, st.link.label, "link", { to: st.link.to }, "Opens it in the console", st.command ? "borderless" : "primary")); }
    if (i < g.steps.length - 1) { acts.push(`${p}-next`); out.push(...button(`${p}-next`, "Next step →", "next", {}, "Mark this step done and go on")); }
    kids.push(`${p}-acts`);
    out.push({ id: `${p}-acts`, component: "Row", children: acts });
    if (ran || !st.command) {
      kids.push(`${p}-what`, `${p}-explain`);
      out.push({ id: `${p}-what`, component: "Text", text: "What happened", variant: "caption" }, label(`${p}-explain`, st.explain));
    }
    out.push({ id: `${p}-col`, component: "Column", children: kids });
  });
  return [create(s), comps(s, [{ id: "root", component: "Column", children: rows }, ...out])];
}
