import { useCallback, useMemo, useState } from "react";
import type { TerminalBus } from "../terminal/terminal-bus";
import type { Guide, StepStatus } from "./guide-types";

/**
 * The controller a guide panel renders from. Everything is a plain method so a future assistant
 * API can drive the same panel: open(guideId), run(stepIndex), next(), back(), close().
 */
export function useGuideRunner(guides: Guide[], bus: TerminalBus) {
  const [guideId, setGuideId] = useState<string | null>(null);
  const [index, setIndex] = useState(0);
  const [status, setStatus] = useState<Record<string, StepStatus>>({});
  const guide = useMemo(() => guides.find((g) => g.id === guideId) ?? null, [guides, guideId]);
  const open = useCallback((id: string) => { setGuideId(id); setIndex(0); setStatus({}); }, []);
  const mark = (id: string, s: StepStatus) => setStatus((m) => ({ ...m, [id]: s }));
  const run = useCallback((i: number) => {
    const step = guide?.steps[i];
    if (!step) return;
    if (step.command) bus.type(step.command, !step.edit);
    mark(step.id, "ran");
  }, [guide, bus]);
  const next = useCallback(() => {
    const step = guide?.steps[index];
    if (step) mark(step.id, "done");
    setIndex((i) => Math.min(i + 1, (guide?.steps.length ?? 1) - 1));
  }, [guide, index]);
  const back = useCallback(() => setIndex((i) => Math.max(i - 1, 0)), []);
  const close = useCallback(() => setGuideId(null), []);
  return { guides, guide, index, status, open, run, next, back, close, setIndex };
}

export type GuideRunner = ReturnType<typeof useGuideRunner>;
