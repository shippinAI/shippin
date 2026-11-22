import { useState } from "react";
import type { SecretStoreConnection } from "../../../../lib/api/actions.gen";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { Button } from "../../../../lib/design/components/button/button";
import { LinkButton } from "../../../../lib/design/components/button/link-button";
import { Card, CardHeader } from "../../../../lib/design/components/card/card";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Notice } from "../../../../lib/design/components/notice/notice";
import { Skeleton } from "../../../../lib/design/components/skeleton/skeleton";
import { PageHeader } from "../../../../template/page-header";
import {
  ago,
  defaultFolder,
  disconnectBlock,
  problemOf,
  secretNeeded,
  storeChanged,
  storeForm,
  storeFormProblem,
  storeStatus,
  type StoreForm,
  whereLocked,
} from "./model";
import { ReasonDialog } from "./reason-dialog";
import { useCanWriteSettings, useDisconnectSecretStore, useSaveSecretStore, useSecretStore, useTestSecretStore } from "./use-settings";

/** Settings > Secret store (CFG-01, docs/settings.md): the installation's Vaultix account, connected with a
 * machine identity. Keys entered on the Settings page are written there. */
export function SecretStorePage() {
  const q = useSecretStore();
  const canWrite = useCanWriteSettings();
  return (
    <>
      <PageHeader
        title="Secret store"
        description="Where the panel keeps the keys you enter in Settings, such as a private key or a webhook secret: a folder in a Vaultix project that this installation signs in to."
        actions={
          <LinkButton href="/admin/settings" variant="secondary" size="sm" tooltip="Edit this installation's settings">
            Settings
          </LinkButton>
        }
      />
      {q.isError && (
        <Notice tone="danger" title="Couldn't read the secret store">
          {problemOf(q.error).message}
        </Notice>
      )}
      {q.isPending && <Skeleton className="h-40 w-full" />}
      {q.data && (
        <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,22rem)]">
          <div className="space-y-6">
            <StatusCard c={q.data} canWrite={canWrite} />
            {/* Remounted when what is saved changes, so the form starts from it. */}
            <ConnectForm key={`${q.data.updated_at ?? "new"}-${q.data.connected}`} c={q.data} canWrite={canWrite} />
          </div>
          <Explainer />
        </div>
      )}
    </>
  );
}

function StatusCard({ c, canWrite }: { c: SecretStoreConnection; canWrite: boolean }) {
  const test = useTestSecretStore();
  const [now] = useState(() => Date.now());
  const status = storeStatus(c, now);
  const [disconnecting, setDisconnecting] = useState(false);
  const blocked = disconnectBlock(c);
  return (
    <Card>
      <CardHeader
        title="Connection"
        description={c.connected ? "The Vaultix project this installation's keys are kept in." : "No Vaultix project is connected yet."}
        actions={<Badge tone={status.tone}>{status.label}</Badge>}
      />
      <div className="space-y-4 px-5 py-4">
        {c.connected ? (
          <>
            <dl className="grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2">
              <Item label="Vaultix address" value={c.api_url} mono />
              <Item label="Project" value={c.project_id} mono />
              <Item label="Environment" value={c.environment} mono />
              <Item label="Folder" value={c.path || defaultFolder} mono />
              <Item label="Machine identity (client ID)" value={c.client_id} mono />
              <Item label="Client secret" value={c.has_client_secret ? "Saved, sealed in the panel's database" : "Not saved"} />
              <Item
                label="Last checked"
                value={c.checked_at ? `${ago(c.checked_at, now)}${c.check_error ? ", failed" : ", worked"}` : "Not checked yet"}
              />
              <Item label="Last changed" value={`${ago(c.updated_at, now)}${c.updated_by ? ` by ${c.updated_by}` : ""}`} />
              <Item
                label="Keys kept here"
                value={
                  c.keys_held === 0 ? "None yet" : c.keys_held === 1 ? "By one settings section" : `By ${c.keys_held} settings sections`
                }
              />
            </dl>
            {c.check_error ? (
              <Notice tone="danger" title="The last check failed">
                {status.detail} Change the connection below if the identity or its access changed in Vaultix, then check again.
              </Notice>
            ) : (
              <p className="text-sm text-fg-muted">{status.detail}</p>
            )}
            {canWrite && (
              <div className="flex flex-wrap items-center gap-2 border-t border-line pt-4">
                <Button
                  variant="secondary"
                  size="sm"
                  isDisabled={test.isPending}
                  onPress={() => test.mutate()}
                  tooltip="Sign in to Vaultix again and write, read back and delete a check value"
                >
                  {test.isPending ? "Checking…" : "Check now"}
                </Button>
                <Button
                  variant="danger"
                  size="sm"
                  isDisabled={!!blocked}
                  onPress={() => setDisconnecting(true)}
                  tooltip={blocked ?? "Stop using this Vaultix project. The keys already there stay in Vaultix."}
                >
                  Disconnect
                </Button>
              </div>
            )}
            {canWrite && blocked && <p className="text-sm text-fg-muted">You can't disconnect now. {blocked}</p>}
          </>
        ) : (
          <p className="text-base text-fg-muted">{status.detail} Fill in the form below to connect.</p>
        )}
      </div>
      {disconnecting && <DisconnectDialog onClose={() => setDisconnecting(false)} />}
    </Card>
  );
}

function Item({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-fg-subtle">{label}</dt>
      <dd className={`mt-0.5 break-all text-fg ${mono ? "font-mono text-xs" : "text-sm"}`}>{value || "—"}</dd>
    </div>
  );
}

function DisconnectDialog({ onClose }: { onClose: () => void }) {
  const del = useDisconnectSecretStore();
  return (
    <ReasonDialog
      title="Disconnect the secret store?"
      description="The panel stops signing in to this Vaultix project. Nothing in Vaultix is deleted."
      confirmLabel="Disconnect"
      danger
      pending={del.isPending}
      onCancel={onClose}
      onConfirm={(reason) => del.mutate(reason, { onSuccess: onClose })}
    >
      <p>Until you connect one again, keys entered on the Settings page can't be saved.</p>
      {del.isError && (
        <Notice tone="danger" title="Not disconnected">
          {problemOf(del.error).message}
        </Notice>
      )}
    </ReasonDialog>
  );
}

function ConnectForm({ c, canWrite }: { c: SecretStoreConnection; canWrite: boolean }) {
  const save = useSaveSecretStore();
  const [f, setF] = useState<StoreForm>(() => storeForm(c));
  const [asking, setAsking] = useState(false);
  const [lastReason, setLastReason] = useState("");
  const set = (k: keyof StoreForm) => (v: string) => setF((x) => ({ ...x, [k]: v }));
  const locked = whereLocked(c);
  const problem = storeFormProblem(f, c);
  const changed = storeChanged(f, c);
  const disabled = !canWrite || save.isPending;
  const needSecret = secretNeeded(f, c);
  return (
    <Card>
      <CardHeader
        title={c.connected ? "Change the connection" : "Connect Vaultix"}
        description="Saving signs in as the machine identity and writes, reads back and deletes a check value first. Nothing is saved unless that works."
      />
      <form
        className="space-y-4 px-5 py-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (!problem && changed && canWrite) setAsking(true);
        }}
      >
        {!canWrite && (
          <Notice tone="info">
            Your staff role can read this page but not change it. Changing it needs the platform.settings.write permission.
          </Notice>
        )}
        {locked && (
          <Notice tone="info" title="Only the identity can change">
            Settings keep keys in this store, so its address, project, environment and folder stay as they are. You can still switch to
            another machine identity or enter a new client secret.
          </Notice>
        )}
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="sm:col-span-2">
            <TextField
              label="Vaultix address"
              value={f.api_url}
              onChange={set("api_url")}
              isDisabled={disabled || locked}
              type="url"
              placeholder="https://vaultix.example.com"
              description="The address the panel reaches Vaultix at. Use https."
            />
          </div>
          <TextField
            label="Project ID"
            value={f.project_id}
            onChange={set("project_id")}
            isDisabled={disabled || locked}
            mono
            description="On the project's settings page in Vaultix."
          />
          <TextField
            label="Environment"
            value={f.environment}
            onChange={set("environment")}
            isDisabled={disabled || locked}
            mono
            placeholder="prod"
            description="The environment's short name (slug), such as prod."
          />
          <div className="sm:col-span-2">
            <TextField
              label="Folder"
              value={f.path}
              onChange={set("path")}
              isDisabled={disabled || locked}
              mono
              placeholder={defaultFolder}
              description={`Keys go in this folder, each settings section in its own folder beneath it. Empty means ${defaultFolder}.`}
            />
          </div>
          <TextField
            label="Client ID"
            value={f.client_id}
            onChange={set("client_id")}
            isDisabled={disabled}
            mono
            autoComplete="off"
            description="The machine identity's Universal Auth client ID."
          />
          <TextField
            label="Client secret"
            value={f.client_secret}
            onChange={set("client_secret")}
            isDisabled={disabled}
            type="password"
            autoComplete="new-password"
            description={
              c.has_client_secret && !needSecret
                ? "A client secret is saved and never shown. Leave this empty to keep it, or enter a new one to replace it."
                : "Required. It is sealed in the panel's database and never shown again."
            }
          />
        </div>
        {save.isError && (
          <Notice tone="danger" title="Not saved">
            {problemOf(save.error).message}
          </Notice>
        )}
        {canWrite && (
          <div className="flex flex-wrap items-center justify-end gap-3 border-t border-line pt-4">
            {problem && changed && <p className="mr-auto text-sm text-fg-muted">{problem}</p>}
            <Button
              type="submit"
              variant="primary"
              isDisabled={disabled || !!problem || !changed}
              tooltip={!changed ? "Change something first" : (problem ?? "Check the connection, then save it")}
            >
              {save.isPending ? "Checking and saving…" : c.connected ? "Check and save" : "Check and connect"}
            </Button>
          </div>
        )}
      </form>
      {asking && (
        <ReasonDialog
          title={c.connected ? "Save the new connection?" : "Connect Vaultix?"}
          description="The panel checks the connection first and saves it only if every step works."
          confirmLabel={c.connected ? "Check and save" : "Check and connect"}
          pending={save.isPending}
          initialReason={lastReason}
          onCancel={() => setAsking(false)}
          onConfirm={(reason) => {
            setLastReason(reason);
            save.mutate({ form: f, reason }, { onSettled: () => setAsking(false) });
          }}
        />
      )}
    </Card>
  );
}

function Explainer() {
  return (
    <Card className="h-fit">
      <CardHeader title="What is a machine identity?" />
      <div className="space-y-3 px-5 py-4 text-sm text-fg-muted">
        <p>
          A machine identity is an account in Vaultix for a program rather than a person. The panel signs in with it to save and read the
          keys you enter in Settings, and it can only reach what you give it access to.
        </p>
        <p>To create one in Vaultix:</p>
        <ol className="list-decimal space-y-1 pl-5">
          <li>Create a machine identity with Universal Auth, and copy its client ID and client secret.</li>
          <li>Give it access to the project, with permission to read, write and delete secrets in the environment you choose.</li>
          <li>Enter the details here. The panel tries them before saving.</li>
        </ol>
        <p>
          Every other key is kept in Vaultix, but the client secret is the key to Vaultix itself, so it can't be kept there. The panel seals
          it in its own database with the panel's at-rest key, never shows it again, and only lets you replace it.
        </p>
      </div>
    </Card>
  );
}
