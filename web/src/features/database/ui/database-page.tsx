import { useEffect, useState } from "react";
import { Database } from "lucide-react";
import { Switch } from "../../../lib/design/components/switch/switch";
import { useDbInfo } from "../../../lib/hooks/use-database";
import { useTenant } from "../../../lib/hooks/use-tenant";
import { Tabs } from "../../../lib/design/components/tabs/tabs";
import { PageHeader } from "../../../template/page-header";
import { BackupsPanel } from "./backups-panel";
import { ConnectPanel } from "./connect-panel";
import { SqlPanel } from "./sql-panel";
import { TablesPanel } from "./tables-panel";

const roKey = (t: string) => `shippin.db.readonly.${t}`;

/** Tenant database: tables, SQL, connection details, and backups. */
export function DatabasePage() {
  const { tenant } = useTenant();
  const id = tenant?.tenant_id ?? "";
  const info = useDbInfo(id);
  const [readOnly, setReadOnly] = useState(false);
  useEffect(() => {
    if (id) setReadOnly(localStorage.getItem(roKey(id)) === "1");
  }, [id]);
  const toggle = (on: boolean) => {
    setReadOnly(on);
    if (id) localStorage.setItem(roKey(id), on ? "1" : "0");
  };
  return (
    <>
      <PageHeader
        eyebrow={tenant?.name ?? "Database"}
        title="Database"
        description="A Postgres database just for this organization. Browse it like a spreadsheet, or write one statement at a time."
        actions={
          <Switch isSelected={readOnly} onChange={toggle} tooltip="While this is on, inserts, edits, deletes, and SQL changes are refused.">
            Read-only
          </Switch>
        }
      />
      <div className="mb-6 flex flex-wrap items-center gap-3 rounded-[var(--radius-card)] border border-line bg-surface px-4 py-3 text-[13px] text-fg-muted shadow-[var(--shadow-e1)]">
        <Database className="size-4 text-hue-data" aria-hidden />
        {info.data ? (
          <span>
            Queries run as <span className="font-mono text-fg">{info.data.role}</span> on{" "}
            <span className="font-mono text-fg">{info.data.database}</span> only, and stop after {Math.round(info.data.statement_timeout_ms / 1000)}s.
          </span>
        ) : (
          <span>Loading the database name…</span>
        )}
      </div>
      {info.data && !info.data.login_ready && (
        <div role="status" className="mb-6 rounded-xl border border-warning/30 bg-warning/10 px-4 py-3 text-[13px] text-fg">
          This console can name the database, but it isn't signed in as your database role yet. Connection details still copy. Running a query needs a pooler login for that role — the password stays in the platform, not in the browser.
        </div>
      )}
      <Tabs
        label="Database sections"
        tabs={[
          { id: "tables", label: "Tables", content: <TablesPanel tenantId={id} readOnly={readOnly} /> },
          { id: "sql", label: "SQL", content: <SqlPanel tenantId={id} readOnly={readOnly} /> },
          { id: "connect", label: "Connect", content: <ConnectPanel info={info.data} loading={info.isPending} /> },
          { id: "backups", label: "Backups", content: <BackupsPanel tenantId={id} /> },
        ]}
      />
    </>
  );
}
