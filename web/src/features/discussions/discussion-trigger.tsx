import { useId, useState } from "react";
import { Disclosure, DisclosurePanel, Heading, Button as RacButton } from "react-aria-components";
import { Button } from "../../lib/design/components/button/button";
import { Badge } from "../../lib/design/components/badge/badge";
import { discussionStatuses, parseDiscussion } from "./discussion";

type Props = {
  snapshot: unknown;
  /** Trusted origins supplied by the host application, never by the transcript. */
  allowedOrigins: readonly string[];
  onRefresh?: () => void;
  isRefreshing?: boolean;
};

/** Tenant authorization belongs to the data adapter; this component only displays a snapshot. */
export function DiscussionTrigger({ snapshot, allowedOrigins, onRefresh, isRefreshing }: Props) {
  const [open, setOpen] = useState(false), id = useId();
  let discussion;
  try { discussion = parseDiscussion(snapshot, allowedOrigins); }
  catch { return <p role="alert" className="text-base text-danger">Discussion unavailable: the snapshot is invalid.</p>; }
  return (
    <section className="min-w-0">
      <div className="flex flex-wrap items-center gap-3">
        <Button tooltip="Read the messages exchanged between agents" aria-expanded={open}
          aria-controls={id} onPress={() => setOpen(!open)}>View AI discussion ({discussion.messages.length})</Button>
        <span aria-live="polite"><Badge>{discussionStatuses[discussion.status]}</Badge></span>
      </div>
      <div id={id} hidden={!open} className="mt-4 min-w-0 space-y-4">
        <div>
          <h3 className="text-lg font-medium text-fg">{discussion.title}</h3>
          <p className="text-base text-fg-muted">Snapshot · captured <time dateTime={discussion.captured_at}>
            {new Date(discussion.captured_at).toLocaleString()}</time></p>
        </div>
        <ol aria-label="Agent messages" className="space-y-4">
          {discussion.messages.map((m) => <li key={m.id} className="min-w-0 border-l-2 border-line pl-4">
            <Disclosure>
              <Heading>
                <RacButton slot="trigger" className="focus-ring flex flex-wrap gap-x-2 text-left text-base font-medium text-fg [overflow-wrap:anywhere]">
                  <span>{m.sender} → {m.recipient}</span><span className="text-fg-muted">{m.kind === "result" ? "Reported result" : "Message"}</span>
                </RacButton>
              </Heading>
              <DisclosurePanel>
                <p className="text-xs text-fg-subtle">Observed <time dateTime={m.observed_at}>{new Date(m.observed_at).toLocaleString()}</time></p>
                <p className="mt-2 whitespace-pre-wrap break-words text-base text-fg [overflow-wrap:anywhere]">{m.content}</p>
              </DisclosurePanel>
            </Disclosure>
          </li>)}
        </ol>
        {!discussion.messages.length && <p className="text-base text-fg-muted">No exchanged messages have been recorded.</p>}
        <div className="flex flex-wrap items-center gap-4">
          {onRefresh && <Button variant="secondary" tooltip="Ask the connected source for newer messages"
            isDisabled={isRefreshing} onPress={onRefresh}>{isRefreshing ? "Checking…" : "Check for replies"}</Button>}
          <a className="focus-ring text-base text-primary underline" href={discussion.source_url}
            target="_blank" rel="noopener noreferrer">Open source conversation</a>
        </div>
      </div>
    </section>
  );
}
