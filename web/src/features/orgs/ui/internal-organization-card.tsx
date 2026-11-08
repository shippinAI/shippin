import { Badge } from "../../../lib/design/components/badge/badge";
import { Card } from "../../../lib/design/components/card/card";
import { useAdminSystem } from "../../../lib/hooks/use-admin";
import { useText } from "../../../lib/ui-config/use-ui-config";

/** The control plane, marked apart from the customers listed below it. */
export function InternalOrganizationCard() {
  const t = useText();
  const { data, error } = useAdminSystem();
  if (error) return <p role="alert" className="mb-5 text-sm text-danger">{error.message}</p>;
  if (!data) return null;
  const { audit } = data;
  const auditWord = !audit.enabled ? t("system.audit.off") : audit.ok ? t("system.audit.protected", { rows: audit.rows }) : t("system.audit.broken", { at: audit.first_bad ?? 0 });
  const auditTone = !audit.enabled ? "text-warning" : audit.ok ? "text-fg" : "text-danger";
  return (
    <Card className="mb-5 p-5" data-testid="system-tenant">
      <div className="mb-1 flex flex-wrap items-center gap-2">
        <h2 className="text-sm font-semibold text-fg">{data.organization.name}</h2>
        <Badge tone="warning">{t("system.badge")}</Badge>
      </div>
      <p className="max-w-3xl text-[13px] text-fg-muted">{t("system.body")}</p>
      <dl className="mt-4 grid gap-4 text-[13px] sm:grid-cols-3">
        <div><dt className="text-fg-subtle">{t("system.hosts")}</dt><dd className="mt-0.5 text-fg">{data.hosts}</dd></div>
        <div><dt className="text-fg-subtle">{t("system.selfGuest")}</dt><dd className="mt-0.5 font-mono text-fg">{data.self_guest || t("system.selfGuest.unknown")}</dd></div>
        <div><dt className="text-fg-subtle">{t("system.audit")}</dt><dd className={`mt-0.5 ${auditTone}`} data-testid="system-audit">{auditWord}</dd></div>
      </dl>
    </Card>
  );
}
