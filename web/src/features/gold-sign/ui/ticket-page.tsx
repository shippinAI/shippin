import { useParams } from "../../../lib/router/router";
import { PageHeader } from "../../../template/page-header";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { useGoldTicket, useGoldTickets } from "../use-gold-sign";
import { DecisionBar } from "./decision-bar";
import { TicketFacts } from "./ticket-facts";
import { useSeen } from "../use-seen";

/** One ticket: gold-policy facts, the full sha256, then Approve / Deny. */
export function TicketPage() {
  const { ticketId = "" } = useParams();
  const t = useGoldTicket(ticketId);
  const list = useGoldTickets();
  const [commandsEnd, commandsSeen] = useSeen<HTMLDivElement>(`${t.data?.id}:${t.data?.sha256}`);
  if (t.isError) return <EmptyState title="Ticket not found" description="It may have been decided, or its facts are not synced yet." />;
  if (!t.data) return null;
  const f = t.data;
  return (
    <div className="space-y-4 pb-2">
      <PageHeader eyebrow="Gold approval" title={f.id}
        description={`Requested ${new Date(f.requested_at * 1000).toLocaleString()}. Signing binds your decision to this exact sha256; any edit to the ticket voids it.`} />
      <TicketFacts t={f} commandsEnd={commandsEnd} />
      <DecisionBar t={f} rpId={list.data?.rp_id ?? ""} keys={list.data?.keys ?? []} commandsSeen={commandsSeen} />
    </div>
  );
}
