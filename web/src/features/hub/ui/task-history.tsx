import { History } from "lucide-react";
import { useState } from "react";
import { Button, Disclosure, DisclosurePanel, Heading } from "react-aria-components";
import { useHubEvents } from "../use-hub";

const label: Record<string, string> = {
  created: "created", claimed: "claimed", progress: "progress", approval_requested: "asked for approval",
  approved: "approved", rejected: "rejected", done: "finished",
};

const time = (iso: string) => new Date(iso).toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });

/** A card's event history, fetched on first expand and kept fresh by the live stream. */
export function TaskHistory({ taskId }: { taskId: string }) {
  const [open, setOpen] = useState(false);
  const ev = useHubEvents(taskId, open);
  return (
    <Disclosure isExpanded={open} onExpandedChange={setOpen} className="mt-2">
      <Heading>
        <Button slot="trigger" className="focus-ring inline-flex items-center gap-1 rounded-sm text-xs text-fg-subtle data-hovered:text-fg">
          <History className="size-3" aria-hidden /> History
        </Button>
      </Heading>
      <DisclosurePanel>
        {ev.isPending && <p className="mt-1 text-xs text-fg-subtle">Loading…</p>}
        {ev.isError && <p className="mt-1 text-xs text-danger">History is unavailable.</p>}
        <ol className="mt-1.5 space-y-1 border-l border-line pl-2.5" aria-label="Task history">
          {ev.data?.items.map((e) => (
            <li key={e.id} className="text-xs leading-snug text-fg-muted">
              <span className="font-medium text-fg">{e.actor}</span> {label[e.kind] ?? e.kind}
              {e.note && <span className="text-fg-subtle">: {e.note}</span>}
              <span className="block text-2xs text-fg-subtle">{time(e.at)}</span>
            </li>
          ))}
        </ol>
      </DisclosurePanel>
    </Disclosure>
  );
}
