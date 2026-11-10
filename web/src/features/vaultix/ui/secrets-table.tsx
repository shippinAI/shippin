import { DataTable, type ColumnDef } from "../../../lib/design/components/data-table/data-table";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import type { VaultixSecret } from "../vaultix-types";
import { useVaultixSecrets } from "../use-vaultix";
import { SecretActions } from "./secret-actions";

const ago = (iso: string) => new Date(iso).toLocaleString();

/** The environment's secrets: names and versions only; values are revealed on request. */
export function SecretsTable({ env, filter }: { env: string; filter: string }) {
  const list = useVaultixSecrets(env);
  if (list.isPending) return <Skeleton className="h-32" />;
  if (list.isError) return <p role="alert" className="text-danger">Could not load secrets: {list.error.message}</p>;
  const f = filter.trim().toLowerCase();
  const rows = list.data.secrets.filter((s) => !f || s.key.toLowerCase().includes(f)).sort((a, b) => a.key.localeCompare(b.key));
  const columns: ColumnDef<VaultixSecret>[] = [
    { id: "key", label: "Name", isRowHeader: true, render: (s) => <span className="font-mono text-sm">{s.key}</span> },
    { id: "value", label: "Value", render: (s) => <SecretActions env={env} secretKey={s.key} /> },
    { id: "version", label: "Version", width: "6rem", render: (s) => <span className="text-fg-muted">v{s.version}</span> },
    { id: "updated", label: "Updated", width: "12rem", render: (s) => <span className="text-fg-muted">{ago(s.updated_at)}</span> },
  ];
  return (
    <DataTable label={`Secrets in ${env}`} columns={columns} rows={rows} dense
      empty={<p className="py-8 text-center text-fg-subtle">No secrets in {env} yet. Add one, import from Infisical Cloud, or push from your keyring.</p>} />
  );
}
