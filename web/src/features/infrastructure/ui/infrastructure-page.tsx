import { useState } from "react";
import { Plus, Server } from "lucide-react";
import { Button } from "../../../lib/design/components/button/button";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { Tabs } from "../../../lib/design/components/tabs/tabs";
import { useInfraOverview } from "../../../lib/hooks/use-infra";
import { useNavigate, useParams } from "../../../lib/router/router";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { PageHeader } from "../../../template/page-header";
import { OVERVIEW_TAB, providersWithHosts, selectedTab, tabHref } from "../model";
import { AddHostDialog } from "./add-host-dialog";
import { OverviewTab } from "./overview-tab";
import { ProviderTab } from "./provider-tab";

/** Overview plus one tab per provider that has a host. The server decides what exists; the URL holds the tab. */
export function InfrastructurePage() {
  const t = useText();
  const nav = useNavigate();
  const { tab } = useParams();
  const q = useInfraOverview();
  const [adding, setAdding] = useState<{ kind?: string; existing?: string } | null>(null);
  const providers = providersWithHosts(q.data);
  const open = (kind?: string, existing?: string) => setAdding({ kind, existing });
  return (
    <>
      <PageHeader
        eyebrow={t("infra.eyebrow")}
        title={t("infra.title")}
        description={t("infra.description")}
        actions={q.data && (
          <Button variant="primary" size="sm" tooltip={t("infra.add.tooltip")} onPress={() => open()}>
            <Plus className="size-4" /> {t("infra.add")}
          </Button>
        )}
      />
      {q.isPending && <div className="space-y-4">{Array.from({ length: 3 }, (_, i) => <Skeleton key={i} className="h-24" />)}</div>}
      {q.isError && <EmptyState illustration={<Server className="size-10 text-fg-subtle" />} title={t("infra.loadError.title")} description={q.error.message} />}
      {q.data && (
        <>
          <Tabs
            label={t("infra.tab.aria")}
            selected={selectedTab(q.data, tab)}
            onSelect={(id) => nav(tabHref(id))}
            tabs={[
              { id: OVERVIEW_TAB, label: t("infra.tab.overview"), content: <OverviewTab data={q.data} onAdd={open} /> },
              ...providers.map((p) => ({ id: p.kind, label: `${p.label} (${p.nodes.length})`, content: <ProviderTab provider={p} /> })),
            ]}
          />
          <AddHostDialog data={q.data} isOpen={adding !== null} initialKind={adding?.kind} initialExisting={adding?.existing}
            onOpenChange={(o) => { if (!o) setAdding(null); }} onAdded={(kind) => nav(tabHref(kind))} />
        </>
      )}
    </>
  );
}
