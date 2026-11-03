import { Info } from "lucide-react";
import type { ReactNode } from "react";
import { Button } from "react-aria-components";
import { Tooltip } from "../../../../lib/design/components/tooltip/tooltip";
import glossary from "./glossary.json";

type Term = keyof typeof glossary;

/** A2UI-style term hint: an info button whose tooltip explains a catalog term. */
export function TermTip({ term, children }: { term: Term; children?: ReactNode }) {
  return (
    <Tooltip content={glossary[term]}>
      <Button aria-label={`What is ${term.replace("_", " ")}?`} className="focus-ring inline-flex cursor-help items-center gap-1 rounded text-fg-subtle data-hovered:text-fg">
        {children}
        <Info className="size-3.5" />
      </Button>
    </Tooltip>
  );
}

export const explain = (term: string): string => (glossary as Record<string, string>)[term] ?? term;
