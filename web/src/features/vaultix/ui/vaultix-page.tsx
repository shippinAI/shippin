import { KeyRound, ShieldCheck } from "lucide-react";
import { useState } from "react";
import { SearchField, Input, Label, Tab, TabList, Tabs } from "react-aria-components";
import { Badge } from "../../../lib/design/components/badge/badge";
import { PageHeader } from "../../../template/page-header";
import { useVaultixAccount } from "../use-vaultix";
import { ImportDialog } from "./import-dialog";
import { KeyringPanel } from "./keyring-panel";
import { SecretEditor } from "./secret-editor";
import { SecretsTable } from "./secrets-table";

/** Vaultix: the signed-in person's own secrets workspace, per environment. */
export function VaultixPage() {
  const account = useVaultixAccount();
  const envs = account.data?.envs ?? [];
  const [env, setEnv] = useState("");
  const [filter, setFilter] = useState("");
  const current = env || envs[0] || "";
  return (
    <>
      <PageHeader eyebrow="Vaultix" title="Secrets"
        description="Your own encrypted workspace. Values stay hidden until you reveal them; the keyring keeps your machines in sync."
        actions={<div className="flex gap-2"><ImportDialog envs={envs} /><SecretEditor env={current} /></div>} />
      {account.isError && <p role="alert" className="mb-4 text-danger">Vaultix is unavailable: {account.error.message}</p>}
      {account.data && (
        <div className="mb-5 flex flex-wrap items-center gap-2 text-base">
          <Badge tone="success"><ShieldCheck className="size-3.5" aria-hidden /> Private workspace</Badge>
          <span className="text-fg-muted">{account.data.email}</span>
          <Badge tone={account.data.keyring_connected ? "primary" : "neutral"}>
            <KeyRound className="size-3.5" aria-hidden /> {account.data.keyring_connected ? "Keyring connected" : "Keyring not connected"}
          </Badge>
        </div>
      )}
      <div className="grid gap-6 lg:grid-cols-[1fr_20rem]">
        <section aria-label="Secrets" className="min-w-0 rounded-2xl border border-line bg-raised/60 p-4">
          <div className="mb-3 flex flex-wrap items-end justify-between gap-3">
            <Tabs selectedKey={current} onSelectionChange={(k) => setEnv(String(k))}>
              <TabList aria-label="Environment" className="flex gap-1 rounded-xl bg-surface p-1">
                {envs.map((e) => (
                  <Tab key={e} id={e} className="cursor-default rounded-lg px-3 py-1.5 text-sm text-fg-muted outline-none selected:bg-raised selected:font-semibold selected:text-fg focus-visible:ring-2 focus-visible:ring-primary">
                    {e}
                  </Tab>
                ))}
              </TabList>
            </Tabs>
            <SearchField value={filter} onChange={setFilter} className="flex flex-col gap-1">
              <Label className="sr-only">Filter secrets</Label>
              <Input placeholder="Filter by name" className="h-8 w-56 rounded-lg border border-line-control bg-surface px-2.5 text-sm outline-none focus:border-primary" />
            </SearchField>
          </div>
          {current && <SecretsTable env={current} filter={filter} />}
        </section>
        {account.data && <KeyringPanel account={account.data} env={current} />}
      </div>
    </>
  );
}
