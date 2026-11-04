import { Pencil, Plus } from "lucide-react";
import { useState } from "react";
import type { CatalogService } from "../../../../lib/api/catalog-types";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { Button } from "../../../../lib/design/components/button/button";
import { IconButton } from "../../../../lib/design/components/button/icon-button";
import { Card } from "../../../../lib/design/components/card/card";
import { useServices } from "../../../../lib/hooks/use-catalog";
import { PageHeader } from "../../../../template/page-header";
import { CatalogPerm, useCan } from "../../../../lib/hooks/use-can";
import { QueryView } from "../query-view";
import { GuidedEmpty } from "./guided-empty";
import { ServiceForm } from "./service-form";

export function ServicesPage() {
  const write = useCan()(CatalogPerm.plansWrite);
  const q = useServices();
  const [edit, setEdit] = useState<Partial<CatalogService> | null>(null);
  const items = q.data?.items ?? [];
  return (
    <>
      <PageHeader eyebrow="Provider" title="Service catalog" description="Manage catalog definitions, configuration defaults, and pinned deployment sources. A source definition is not a deployed service."
        actions={write && <Button variant="primary" size="sm" tooltip="Add a service row" onPress={() => setEdit({})}><Plus className="size-4" /> New service</Button>} />
      <QueryView isPending={q.isPending} error={q.error} onRetry={() => void q.refetch()}>
        {() => items.length === 0 ? (
          <Card><GuidedEmpty title="The catalog is empty" action={write && <Button tooltip="Add a service" variant="primary" size="sm" onPress={() => setEdit({})}>Add a service</Button>}
            steps={["Add a service with an id, label and config schema.", "Set its system defaults.", "Include it in tiers from Plans & tiers."]} /></Card>
        ) : (
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            {items.map((s) => (
              <Card key={s.id} className="flex flex-col p-5">
                <div className="flex items-start justify-between gap-2">
                  <div><div className="font-semibold text-fg">{s.label}</div><div className="font-mono text-[11.5px] text-fg-subtle">{s.id} · {s.kind}</div></div>
                  {write && <IconButton icon={Pencil} label={`Edit ${s.label}`} onPress={() => setEdit(s)} />}
                </div>
                <p className="mt-2 text-[13px] text-fg-muted">{s.description}</p>
                <div className="mt-3 flex flex-wrap gap-1.5">
                  {s.required && <Badge tone="danger">required on every plan</Badge>}
                  {!s.active && <Badge tone="warning">inactive</Badge>}
                  {s.deployment_source && <Badge tone="info">{s.deployment_source.kind} source pinned</Badge>}
                  {Object.entries(s.config_schema.properties ?? {}).map(([k, v]) => (
                    <Badge key={k}>{v.title ?? k}: {JSON.stringify(s.default_config[k] ?? "—")}</Badge>
                  ))}
                </div>
                {s.docs_url && <div className="mt-auto pt-3 font-mono text-[11px] text-fg-subtle">{s.docs_url}</div>}
              </Card>
            ))}
          </div>
        )}
      </QueryView>
      {edit && <ServiceForm initial={edit.id ? (edit as CatalogService) : undefined} onClose={() => setEdit(null)} />}
    </>
  );
}
