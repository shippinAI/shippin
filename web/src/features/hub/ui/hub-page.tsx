import { PageHeader } from "../../../template/page-header";
import { Badge } from "../../../lib/design/components/badge/badge";
import { columns } from "../hub-types";
import { useHubMe, useHubTasks } from "../use-hub";
import { useHubLive } from "../use-hub-live";
import { NewTask } from "./new-task";
import { TaskCard } from "./task-card";

/** Agent hub: the checkout board. Agents claim cards under a lease; sign-off happens here. */
export function HubPage() {
  const me = useHubMe();
  const live = useHubLive();
  const tasks = useHubTasks(live === "live");
  const items = tasks.data?.items ?? [];
  const waiting = items.filter((t) => t.status === "needs_approval").length;
  return (
    <>
      <PageHeader
        eyebrow="Agent hub"
        title="Board"
        description="Agents check out tasks under a lease, post progress and ask for sign-off. Approvals happen on the card."
        actions={<NewTask />}
      />
      <div className="mb-4 flex flex-wrap items-center gap-2 text-sm">
        <Badge tone={live === "live" ? "success" : "neutral"}>{live === "live" ? "Live" : live === "connecting" ? "Connecting…" : "Offline: polling"}</Badge>
        {waiting > 0 && <Badge tone="warning">{waiting} waiting for your approval</Badge>}
        {me.data?.is_owner && <Badge tone="primary">You approve</Badge>}
        {tasks.isError && <span className="text-danger">The board is unavailable: {tasks.error.message}</span>}
      </div>
      <div className="-mx-1 flex snap-x gap-3 overflow-x-auto px-1 pb-4" role="list" aria-label="Board columns">
        {columns.map((col) => {
          const cards = items.filter((t) => t.status === col.status);
          return (
            <section key={col.status} role="listitem" aria-label={col.label}
              className="flex w-[15.5rem] shrink-0 snap-start flex-col rounded-2xl border border-line bg-raised/60 p-2.5">
              <header className="mb-2.5 flex items-center gap-2 px-1">
                <span className={`size-2 rounded-full ${col.accent}`} aria-hidden />
                <h2 className="text-[12.5px] font-semibold tracking-wide text-fg uppercase">{col.label}</h2>
                <span className="ml-auto rounded-full bg-surface px-2 text-[11.5px] text-fg-muted">{cards.length}</span>
              </header>
              <div className="flex min-h-24 flex-col gap-2.5">
                {cards.map((t) => <TaskCard key={t.id} task={t} isOwner={!!me.data?.is_owner} />)}
                {cards.length === 0 && <p className="rounded-xl border border-dashed border-line px-3 py-6 text-center text-[12px] text-fg-subtle">Nothing here</p>}
              </div>
            </section>
          );
        })}
      </div>
    </>
  );
}
