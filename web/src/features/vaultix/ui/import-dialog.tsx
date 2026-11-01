import { CloudDownload } from "lucide-react";
import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../lib/design/components/field/text-field";
import { toast } from "../../../lib/design/components/toast/toast-queue";
import type { CloudImport } from "../vaultix-types";
import { useCloudImport } from "../use-vaultix";

const blank: CloudImport = { url: "https://app.infisical.com", client_id: "", client_secret: "", project: "", from_env: "prod", to_env: "", path: "/" };

/** Copy one Infisical Cloud environment into this workspace with a read-only machine identity. */
export function ImportDialog({ envs }: { envs: string[] }) {
  const [open, setOpen] = useState(false);
  const [f, setF] = useState(blank);
  const run = useCloudImport();
  const set = (k: keyof CloudImport) => (v: string) => setF((o) => ({ ...o, [k]: v }));
  const to = f.to_env || envs[0] || "";
  return (
    <>
      <Button variant="ghost" tooltip="Bring secrets over from Infisical Cloud" onPress={() => setOpen(true)}>
        <CloudDownload className="size-4" aria-hidden /> Import
      </Button>
      <ModalDialog isOpen={open} onOpenChange={setOpen} title="Import from Infisical Cloud"
        description="Create a machine identity with read access in Infisical, paste it here. Nothing about it is stored.">
        <form className="grid gap-3 px-6 py-5 sm:grid-cols-2" onSubmit={(e) => {
          e.preventDefault();
          run.mutate({ ...f, to_env: to }, {
            onSuccess: (r) => { toast.success(`Imported ${r.imported} secrets`, `into ${to}`); setF(blank); setOpen(false); },
            onError: (err) => toast.error("Import failed", err.message),
          });
        }}>
          <div className="sm:col-span-2"><TextField label="Infisical URL" mono value={f.url} onChange={set("url")} isRequired /></div>
          <TextField label="Client ID" mono value={f.client_id} onChange={set("client_id")} isRequired />
          <TextField label="Client secret" mono type="password" value={f.client_secret} onChange={set("client_secret")} isRequired />
          <TextField label="Project ID" mono value={f.project} onChange={set("project")} isRequired />
          <TextField label="Folder" mono value={f.path} onChange={set("path")} />
          <TextField label="From environment" mono value={f.from_env} onChange={set("from_env")} isRequired />
          <TextField label={`Into (${envs.join(", ")})`} mono value={to} onChange={set("to_env")} isRequired />
          <div className="mt-1 flex justify-end gap-2 sm:col-span-2">
            <Button variant="ghost" tooltip="Close without importing" onPress={() => setOpen(false)}>Cancel</Button>
            <Button variant="primary" type="submit" tooltip="Copy every secret of that environment" isDisabled={run.isPending}>Import</Button>
          </div>
        </form>
      </ModalDialog>
    </>
  );
}
