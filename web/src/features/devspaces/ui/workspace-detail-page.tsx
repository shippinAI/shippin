import { AlertTriangle, ArrowLeft, Code2, KeyRound, SquareTerminal } from "lucide-react";
import { useParams, useSearchParams } from "../../../lib/router/router";
import { Button } from "../../../lib/design/components/button/button";
import { LinkButton } from "../../../lib/design/components/button/link-button";
import { Card } from "../../../lib/design/components/card/card";
import { InfoItem } from "../../../lib/design/components/info/info-item";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { Tabs } from "../../../lib/design/components/tabs/tabs";
import { timeAgo } from "../../../lib/format";
import { useMe } from "../../../lib/hooks/use-api";
import { useStateToasts } from "../../../lib/hooks/use-state-toasts";
import { useTenant } from "../../../lib/hooks/use-tenant";
import { useWorkspaces } from "../../../lib/hooks/use-workspaces";
import { PageHeader } from "../../../template/page-header";
import { AccessTab } from "./access-tab";
import { DomainsTab } from "./domains-tab";
import { NetworkingTab } from "./networking-tab";
import { OpenButton } from "./open-button";
import { OverviewTab } from "./overview-tab";
import { PreviewsTab } from "./previews-tab";
import { PublishTab } from "./publish-tab";
import { ProvisionProgress } from "./provision-progress";
import { StateBadge } from "./state-badge";
import { WorkspaceMenu } from "./workspace-menu";

export function WorkspaceDetailPage() {
  const { workspaceId } = useParams();
  const [search] = useSearchParams();
  const tab = search.get("tab") ?? undefined;
  const { tenant } = useTenant();
  const q = useWorkspaces(tenant?.tenant_id);
  useStateToasts(q.data?.items);
  const me = useMe().data?.user_id;
  const ws = q.data?.items.find((w) => w.id === workspaceId);
  if (!ws) return <Skeleton className="h-96" />;
  const can = !!q.data?.can_manage;
  const actions = (
    <>
      <OpenButton ws={ws} size="md" />
      <LinkButton href={`/t/${ws.tenant_id}/workspaces/${ws.id}/terminal`} variant="secondary" tooltip="A plain terminal with guided steps, no IDE">
        <SquareTerminal className="size-4" /> Terminal
      </LinkButton>
      <LinkButton href={`/t/${ws.tenant_id}/workspaces/${ws.id}/connect`} variant="secondary" tooltip="GitHub, Claude, Codex and Grok sign-ins">
        <KeyRound className="size-4" /> Connect accounts
      </LinkButton>
      <Button
        variant="secondary"
        isDisabled={ws.state !== "running" || !ws.vscode_url}
        tooltip="Open in desktop VS Code (Remote-SSH, uses your account SSH key)"
        onPress={() => { window.location.href = ws.vscode_url ?? ""; }}
      >
        <Code2 className="size-4" /> Open in VS Code
      </Button>
      <WorkspaceMenu ws={ws} canManage={can} />
    </>
  );
  return (
    <>
      <div className="mb-4">
        <LinkButton href={`/t/${ws.tenant_id}/workspaces`} variant="ghost" size="sm" tooltip="Back to all workspaces">
          <ArrowLeft className="size-4" /> Workspaces
        </LinkButton>
      </div>
      <PageHeader
        eyebrow={tenant?.name}
        title={ws.name}
        description={<span className="flex items-center gap-3"><StateBadge state={ws.state} /> Created {timeAgo(ws.created_at)}</span>}
        actions={actions}
      />
      {ws.state === "provisioning" && <ProvisionProgress ws={ws} />}
      {ws.state === "failed" && (
        <div className="mb-6 flex items-start gap-2 rounded-lg border border-danger/30 bg-danger/10 px-4 py-3 text-base text-danger">
          <AlertTriangle className="mt-0.5 size-4 shrink-0" /> {ws.error}
        </div>
      )}
      <Card className="mb-6 grid grid-cols-2 gap-6 px-6 py-5 lg:grid-cols-4">
        <InfoItem label="HTTPS address" value={ws.host} />
        <InfoItem label="Public IPv4" value={ws.public_ip} mono />
        <InfoItem label="Private IPv4" value={ws.ipv4 ?? ""} mono />
        <InfoItem label="Region" value={ws.region} copy={false} />
      </Card>
      <Tabs
        label="Workspace sections"
        defaultTab={tab}
        tabs={[
          { id: "overview", label: "Overview", content: <OverviewTab ws={ws} /> },
          { id: "previews", label: `Previews${ws.preview_urls.length ? ` (${ws.preview_urls.length})` : ""}`, content: <PreviewsTab ws={ws} canEdit={can || ws.created_by === me} /> },
          { id: "publish", label: "Publish", content: <PublishTab ws={ws} canEdit={can || ws.created_by === me} /> },
          { id: "networking", label: "Networking", content: <NetworkingTab ws={ws} canManage={can} /> },
          { id: "domains", label: "Domains", content: <DomainsTab ws={ws} canManage={can} /> },
          { id: "access", label: "Access", content: <AccessTab ws={ws} /> },
        ]}
      />
    </>
  );
}
