import { Tab, TabList, TabPanel, Tabs } from "react-aria-components";
import type { RRDecision, RRMemory, RRSession } from "../../../lib/api/rrflow-types";
import { Badge } from "../../../lib/design/components/badge/badge";
import { useRRFlowState } from "../../../lib/hooks/use-rrflow";

const tab = "focus-ring relative -mb-px cursor-default border-b-2 border-transparent pb-2 text-sm text-fg-muted outline-none data-hovered:text-fg data-selected:border-primary data-selected:text-fg";
const row = "flex flex-wrap items-baseline gap-x-3 gap-y-1 border-b border-line py-2 text-[13px]";
const when = (s: string) => new Date(s).toLocaleString();
const none = (what: string) => <p className="py-3 text-[13px] text-fg-subtle">No {what} yet.</p>;

type Props = { tenantId: string; projectId: string; serviceId: string };

/** Read-only rrflow/state views (02c data model: ai_session, memory_item, decision). */
export function RRFlowViews({ tenantId, projectId, serviceId }: Props) {
  const sessions = useRRFlowState<{ items: RRSession[] }>(tenantId, projectId, serviceId, "sessions");
  const memory = useRRFlowState<{ items: RRMemory[] }>(tenantId, projectId, serviceId, "memory");
  const log = useRRFlowState<{ items: RRDecision[]; chain_ok: boolean }>(tenantId, projectId, serviceId, "decisions");
  return (
    <Tabs>
      <TabList aria-label="rrflow state" className="mb-2 flex gap-5 border-b border-line">
        <Tab id="sessions" className={tab}>Sessions</Tab>
        <Tab id="memory" className={tab}>Memory</Tab>
        <Tab id="decisions" className={tab}>Decision log</Tab>
      </TabList>
      <TabPanel id="sessions" className="outline-none">
        {sessions.data?.items.length ? sessions.data.items.map((s) => (
          <div key={s.id} className={row}>
            <span className="font-medium text-fg">{s.agent}</span><span className="text-fg-muted">{s.mode}</span>
            <Badge tone={s.status === "running" ? "info" : "neutral"}>{s.status}</Badge><span className="text-fg-subtle">{when(s.started_at)}</span>
          </div>
        )) : none("sessions")}
      </TabPanel>
      <TabPanel id="memory" className="outline-none">
        {memory.data?.items.length ? memory.data.items.map((m) => (
          <div key={m.key} className={row}>
            <span className="font-mono text-fg">{m.key}</span><span className="min-w-0 flex-1 break-words text-fg-muted">{m.value}</span>
            <span className="text-fg-subtle">v{m.version}</span>
          </div>
        )) : none("memories")}
      </TabPanel>
      <TabPanel id="decisions" className="outline-none">
        {log.data && <Badge tone={log.data.chain_ok ? "success" : "danger"}>{log.data.chain_ok ? "Chain verified" : "Chain broken"}</Badge>}
        {log.data?.items.length ? log.data.items.map((d) => (
          <div key={d.id} className={row}>
            <span className="text-fg-subtle">#{d.seq}</span><span className="text-fg">{d.actor} · {d.action}</span>
            <span className="text-fg-muted">{d.resource}</span><Badge tone={d.effect === "allow" ? "success" : d.effect === "deny" ? "danger" : "warning"}>{d.effect}</Badge>
            <span className="font-mono text-[11px] text-fg-subtle">{d.hash.slice(0, 12)}</span>
          </div>
        )) : none("decisions")}
      </TabPanel>
    </Tabs>
  );
}
