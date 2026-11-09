/**
 * Guided experiences. A guide is plain data so an agent (or any API) can send one later; the
 * components render whatever guide and step index they are given.
 */
export type GuideLink = { label: string; to: string };

export type GuideStep = {
  id: string;
  title: string;
  body: string;
  /** Typed into the terminal by "Run this" (a trailing newline runs it). */
  command?: string;
  /** Type the command without pressing Enter, so the user fills in a placeholder first. */
  edit?: boolean;
  /** Shown after the step ran: what just happened, in plain words. */
  explain: string;
  link?: GuideLink;
};

export type Guide = { id: string; title: string; summary: string; steps: GuideStep[] };

export type StepStatus = "todo" | "ran" | "done";

/** Guide tokens like {connectUrl} are filled from page context (tenant, workspace…). */
export function fillGuide(g: Guide, vars: Record<string, string>): Guide {
  const fill = (s: string) => s.replace(/\{(\w+)\}/g, (m, k: string) => vars[k] ?? m);
  return {
    ...g,
    steps: g.steps.map((s) => ({ ...s, body: fill(s.body), command: s.command && fill(s.command), explain: fill(s.explain),
      link: s.link && { label: s.link.label, to: fill(s.link.to) } })),
  };
}
