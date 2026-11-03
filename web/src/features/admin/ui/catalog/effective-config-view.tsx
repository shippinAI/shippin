import { Disclosure, DisclosurePanel, Button as RacButton, Heading } from "react-aria-components";
import { ChevronRight } from "lucide-react";
import type { Effective } from "../../../../lib/api/catalog-types";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { useEffective } from "../../../../lib/hooks/use-catalog";
import { QueryView } from "../query-view";
import { LayerBadge, LockBadge } from "./layer-badge";

const show = (v: unknown) => (typeof v === "string" ? v : JSON.stringify(v));

function Row({ e }: { e: Effective }) {
  return (
    <Disclosure className="border-b border-line last:border-0">
      <Heading className="flex flex-wrap items-center gap-x-3 gap-y-1.5 px-5 py-2.5">
        <RacButton slot="trigger" className="focus-ring group flex min-w-0 basis-full items-center gap-2 rounded text-left sm:basis-0 sm:flex-1">
          <ChevronRight className="size-3.5 shrink-0 text-fg-subtle transition-transform group-aria-expanded:rotate-90" />
          <span className="truncate font-mono text-[12.5px] text-fg">{e.key}</span>
        </RacButton>
        <span className="max-w-full truncate font-mono text-[12.5px] text-primary">{show(e.value)}</span>
        <LayerBadge layer={e.source} refId={e.ref} />
        <LockBadge lock={e.lock} />
        {e.flags.length > 0 && <Badge tone="warning">clamped</Badge>}
        {e.trail.some((s) => s.outcome === "rejected") && <Badge tone="danger">override rejected</Badge>}
      </Heading>
      <DisclosurePanel>
        <ol className="space-y-1 px-12 pb-3 text-xs text-fg-muted">
          {e.note && <li className="text-fg">{e.note}</li>}
          {e.bounds && <li>bounds: {JSON.stringify(e.bounds)}</li>}
          {e.trail.map((s, i) => (
            <li key={i}><span className="font-medium text-fg">{s.layer}{s.ref ? ` ${s.ref.slice(0, 8)}` : ""}</span> → {show(s.value)}
              {" "}<span className={s.outcome === "rejected" || s.outcome === "clamped" ? "text-warning" : ""}>({s.outcome}{s.reason ? `: ${s.reason}` : ""})</span></li>
          ))}
        </ol>
      </DisclosurePanel>
    </Disclosure>
  );
}

/** Tenant effective-config viewer: services first, then settings, each with its source layer. */
export function EffectiveConfigView({ tenantId }: { tenantId: string }) {
  const q = useEffective(tenantId);
  return (
    <QueryView isPending={q.isPending} error={q.error} onRetry={() => void q.refetch()}>
      {() => q.data && (
        <>
          <div className="flex flex-wrap items-center gap-2 border-b border-line px-5 py-3 text-xs text-fg-muted">
            Enabled services: {q.data.enabled_services.map((s) => <Badge key={s} tone="success">{s}</Badge>)}
          </div>
          <div className="px-5 pt-3 text-[11.5px] font-medium tracking-wide text-fg-subtle uppercase">Services</div>
          {q.data.services.map((e) => <Row key={e.key} e={e} />)}
          <div className="px-5 pt-3 text-[11.5px] font-medium tracking-wide text-fg-subtle uppercase">Settings</div>
          {q.data.settings.map((e) => <Row key={e.key} e={e} />)}
        </>
      )}
    </QueryView>
  );
}
