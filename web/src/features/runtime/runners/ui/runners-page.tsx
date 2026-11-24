import { Tabs } from "../../../../lib/design/components/tabs/tabs";
import { useTenant } from "../../../../lib/hooks/use-tenant";
import { useNavigate, useParams } from "../../../../lib/router/router";
import { useText } from "../../../../lib/ui-config/use-ui-config";
import { PageHeader } from "../../../../template/page-header";
import { JobsTab } from "./jobs-tab";
import { RepositoriesTab } from "./repositories-tab";
import { RunnersExplainer } from "./runners-explainer";

const tabs = ["jobs", "repositories", "how"] as const;
type Tab = (typeof tabs)[number];
const tabOf = (t?: string): Tab => ((tabs as readonly string[]).includes(t ?? "") ? (t as Tab) : "jobs");

/** The organization's build runners (RUN-01): its jobs, its GitHub repositories and how to ask for a runner.
 * The tab is in the URL, /t/:tenantId/runners/:tab. */
export function RunnersPage() {
  const { tenant } = useTenant();
  const { tab } = useParams();
  const nav = useNavigate();
  const text = useText();
  const tenantId = tenant?.tenant_id ?? "";
  const canManage = !!tenant?.can_manage;
  return (
    <>
      <PageHeader eyebrow={tenant?.name} title={text("runners.title")} description={text("runners.description")} />
      {tenantId && (
        <Tabs
          label={text("runners.title")}
          selected={tabOf(tab)}
          onSelect={(id) => nav(`/t/${tenantId}/runners/${id}`)}
          tabs={[
            { id: "jobs", label: text("runners.jobs"), content: <JobsTab tenantId={tenantId} /> },
            {
              id: "repositories",
              label: text("runners.repositories"),
              content: <RepositoriesTab tenantId={tenantId} canManage={canManage} />,
            },
            { id: "how", label: text("runners.how"), content: <RunnersExplainer /> },
          ]}
        />
      )}
    </>
  );
}
