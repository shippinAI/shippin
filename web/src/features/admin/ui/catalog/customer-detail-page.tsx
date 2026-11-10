import { ArrowLeft, Pencil, Send } from "lucide-react";
import { useState } from "react";
import { Link } from "react-aria-components";
import { useParams } from "../../../../lib/router/router";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { Button } from "../../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../../lib/design/components/card/card";
import { catalogPaths, useCatalogWrite, useCustomer, useIntents } from "../../../../lib/hooks/use-catalog";
import { PageHeader } from "../../../../template/page-header";
import { CatalogPerm, useCan } from "../../../../lib/hooks/use-can";
import { QueryView } from "../query-view";
import { CustomerForm } from "./customer-form";
import { CustomerFacts } from "./customer-facts";
import { EffectiveConfigView } from "./effective-config-view";
import { TermTip } from "./term-tip";

export function CustomerDetailPage() {
  const { customerId = "" } = useParams();
  const q = useCustomer(customerId);
  const intents = useIntents(customerId);
  const record = useCatalogWrite("Provision intent recorded (pending change approval)");
  const [editing, setEditing] = useState(false);
  const can = useCan();
  const c = q.data;
  return (
    <>
      <Link href="/admin/customers" className="focus-ring mb-4 inline-flex items-center gap-1.5 rounded-sm text-sm text-fg-muted hover:text-fg">
        <ArrowLeft className="size-3.5" /> Customers
      </Link>
      <QueryView isPending={q.isPending} error={q.error} onRetry={() => void q.refetch()}>
        {() => c && (
          <>
            <PageHeader eyebrow="Customer" title={c.name}
              description={<span className="inline-flex flex-wrap items-center gap-2">
                <Badge tone={c.status === "active" ? "success" : "neutral"}>{c.status}</Badge>
                {c.status === "draft" && <TermTip term="draft" />}
                <Badge tone="info">plan {c.plan_id}</Badge>
                <Badge tone={c.idp_linked ? "success" : "warning"}>{c.idp_linked ? "sign-in org linked" : "no sign-in org yet"}</Badge>
              </span>}
              actions={<>
                {can(CatalogPerm.customersWrite) && <Button variant="secondary" size="sm" tooltip="Edit name, plan, contacts and endpoints" onPress={() => setEditing(true)}><Pencil className="size-4" /> Edit</Button>}
                {can(CatalogPerm.intentCreate) && <Button variant="primary" size="sm" isDisabled={record.isPending} tooltip="Snapshot the effective config as desired state; change control applies it later"
                  onPress={() => record.mutate({ method: "POST", path: catalogPaths.intent(c.id), body: { note: "from admin panel" } })}>
                  <Send className="size-4" /> Record provision intent
                </Button>}
              </>} />
            <CustomerFacts customer={c} intents={intents.data?.items ?? []} />
            {can(CatalogPerm.effectiveRead) && <Card className="mt-6 overflow-hidden">
              <CardHeader title={<span className="inline-flex items-center gap-1.5">Effective config <TermTip term="effective" /></span>}
                description="Each value with the layer it came from. Hover a badge to learn what it means; open a row for the full chain." />
              <EffectiveConfigView tenantId={c.id} />
            </Card>}
            {editing && <CustomerForm initial={c} onClose={() => setEditing(false)} />}
          </>
        )}
      </QueryView>
    </>
  );
}
