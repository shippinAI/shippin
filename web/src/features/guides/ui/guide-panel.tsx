import { ChevronLeft, Sparkles } from "lucide-react";
import { useCallback, useMemo } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { guideMessages, pickerMessages } from "../../../lib/guides/guide-a2ui";
import type { GuideRunner } from "../../../lib/guides/use-guide-runner";
import type { A2UIUserAction } from "../../../lib/a2ui/a2ui-types";
import { A2UISurface } from "../../a2ui/a2ui-surface";
import { useSurfaces } from "../../a2ui/use-surfaces";

type Props = { runner: GuideRunner; onLink: (to: string) => void };

/** The guide side panel. Every guide message is an A2UI v0.9 surface rendered by A2UISurface;
 * user actions come back as A2UI `action` events and drive the runner. */
export function GuidePanel({ runner, onLink }: Props) {
  const { guide, index, status } = runner;
  const messages = useMemo(() => (guide ? guideMessages(guide, index, status) : pickerMessages(runner.guides)), [guide, index, status, runner.guides]);
  const { surfaces, edit } = useSurfaces(messages);
  const surface = surfaces[guide ? `node-${guide.id}` : "node-picker"];
  const onAction = useCallback((a: A2UIUserAction) => {
    const step = Number(a.context.step);
    if (a.name === "open") runner.open(String(a.context.guide));
    else if (a.name === "run") runner.run(step);
    else if (a.name === "next") runner.next();
    else if (a.name === "goto") runner.setIndex(step);
    else if (a.name === "link") onLink(String(a.context.to));
  }, [runner, onLink]);
  return (
    <aside aria-label="Guide" className="flex h-full min-h-0 flex-col border-l border-line bg-surface">
      <header className="flex items-center gap-2 border-b border-line px-4 py-3">
        {guide && (
          <Button variant="ghost" size="sm" aria-label="All guides" tooltip="Back to all guides" onPress={runner.close}><ChevronLeft className="size-4" /></Button>
        )}
        <Sparkles className="size-4 text-hue-ai" aria-hidden />
        <h2 className="truncate text-base font-semibold text-fg">{guide ? guide.title : "Guide"}</h2>
      </header>
      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        {surface && <A2UISurface surface={surface} onAction={onAction} onData={(p, v) => edit(surface.id, p, v)} label={guide ? guide.title : "Guides"} />}
      </div>
    </aside>
  );
}
