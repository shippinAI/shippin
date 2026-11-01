import { useEffect } from "react";
import { useWorkspaces } from "../../../lib/hooks/use-workspaces";
import { useNavigate, useParams } from "../../../lib/router/router";
import { PageHeader } from "../../../template/page-header";
import { TenantOnboardingCard } from "./tenant-onboarding-card";

const features: Record<string, { label: string; path: (base: string) => string }> = {
  terminal: { label: "Terminal", path: (b) => `${b}/terminal` },
  connect: { label: "Connect accounts", path: (b) => `${b}/connect` },
  publish: { label: "Publish", path: (b) => `${b}?tab=publish` },
  ai: { label: "AI & Agents", path: (b) => `${b}/terminal?node=1` },
};

/** Sidebar shortcut: opens a workspace feature in the tenant's running (else newest) workspace. */
export function GoPage() {
  const { tenantId = "", feature = "" } = useParams();
  const navigate = useNavigate();
  const q = useWorkspaces(tenantId);
  const items = q.data?.items ?? [];
  const ws = items.find((w) => w.state === "running") ?? items[0];
  const f = features[feature];
  useEffect(() => {
    if (feature === "usage") navigate(`/t/${tenantId}/projects#usage`);
    else if (f && ws) navigate(f.path(`/t/${tenantId}/workspaces/${ws.id}`));
  }, [feature, f, ws, tenantId, navigate]);
  if (q.isLoading || (ws && f) || feature === "usage") return null;
  return (
    <>
      <PageHeader title={f?.label ?? "Not found"} description={f ? "Opens in your workspace." : "There's no such shortcut."} />
      {f && <TenantOnboardingCard tenantId={tenantId} feature={f.label} />}
    </>
  );
}
