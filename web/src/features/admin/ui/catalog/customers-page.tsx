import { Plus } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "../../../../lib/router/router";
import type { Customer } from "../../../../lib/api/catalog-types";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { Button } from "../../../../lib/design/components/button/button";
import { Card } from "../../../../lib/design/components/card/card";
import { DataTable, type ColumnDef } from "../../../../lib/design/components/data-table/data-table";
import { useCustomers, usePlans } from "../../../../lib/hooks/use-catalog";
import { CatalogPerm, useCan } from "../../../../lib/hooks/use-can";
import { PageHeader } from "../../../../template/page-header";
import { QueryView } from "../query-view";
import { CustomerForm } from "./customer-form";
import { GuidedEmpty } from "./guided-empty";
import { TermTip } from "./term-tip";

const statusTone = { active: "success", draft: "neutral", suspended: "warning", error: "danger", provisioning: "info" } as const;

export function CustomersPage() {
  const q = useCustomers();
  const plans = usePlans();
  const nav = useNavigate();
  const [creating, setCreating] = useState(false);
  const can = useCan();
  const write = can(CatalogPerm.customersWrite);
  const planLabel = (id: string) => plans.data?.items.find((p) => p.id === id)?.label ?? id;
  const cols: ColumnDef<Customer>[] = [
    { id: "name", label: "Customer", isRowHeader: true, render: (c) => (
      <div><div className="font-medium text-fg">{c.name}</div><div className="font-mono text-[11.5px] text-fg-subtle">{c.id.slice(0, 13)}</div></div>
    ) },
    { id: "plan", label: "Plan", width: "120px", render: (c) => <Badge tone="info">{planLabel(c.plan_id)}</Badge> },
    { id: "status", label: "Status", width: "130px", render: (c) => (
      <Badge tone={statusTone[c.status as keyof typeof statusTone] ?? "neutral"}>{c.status}</Badge>
    ) },
    ...(can(CatalogPerm.contactsRead) ? [{ id: "contact", label: "Primary contact", render: (c: Customer) => <span className="text-xs">{c.contacts?.[0]?.email ?? "—"}</span> }] : []),
    { id: "ws", label: "Workspaces", width: "110px", render: (c) => c.workspaces },
    { id: "intent", label: "Last intent", width: "120px", render: (c) => (c.last_intent ? <Badge tone="warning">{c.last_intent}</Badge> : "—") },
  ];
  return (
    <>
      <PageHeader eyebrow="Provider" title="Customers"
        description={<>Every tenant, its plan and contacts. Open one to see its effective config and record a provision intent <TermTip term="provision_intent" />.</>}
        actions={write && <Button variant="primary" size="sm" tooltip="Add a draft customer" onPress={() => setCreating(true)}><Plus className="size-4" /> New customer</Button>} />
      <Card className="overflow-hidden">
        <QueryView isPending={q.isPending} error={q.error} onRetry={() => void q.refetch()}>
          {() => (
            <DataTable label="Customers" columns={cols} rows={q.data?.items ?? []} onRowAction={(id) => nav(`/admin/customers/${id}`)}
              empty={<GuidedEmpty title="No customers yet" action={write && <Button tooltip="Add the first customer" variant="primary" size="sm" onPress={() => setCreating(true)}>Add the first customer</Button>}
                steps={["Check Plans & Tiers: each tier lists the services it includes.", "Add a customer and pick a plan; it starts as a draft.",
                  "Review its effective config, then record a provision intent for change control."]} />} />
          )}
        </QueryView>
      </Card>
      {creating && <CustomerForm onClose={() => setCreating(false)} />}
    </>
  );
}
