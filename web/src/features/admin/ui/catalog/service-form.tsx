import { useState } from "react";
import type { CatalogService } from "../../../../lib/api/catalog-types";
import { Button } from "../../../../lib/design/components/button/button";
import { Drawer } from "../../../../lib/design/components/drawer/drawer";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Switch } from "../../../../lib/design/components/switch/switch";
import { Select } from "../../../../lib/design/components/select/select";
import { catalogPaths, useCatalogWrite } from "../../../../lib/hooks/use-catalog";
import { deploymentSourceFromDraft, sourceDraft, validDeploymentSource } from "./service-source";
import { runtimeContractFromDraft, runtimeKindDraft, validRuntimeDraft } from "./service-runtime";

const obj = (s: string) => { try { const v = JSON.parse(s); return v && typeof v === "object" && !Array.isArray(v) ? v : null; } catch { return null; } };

/** Create or edit a catalog service, including its JSON config schema and defaults. */
export function ServiceForm({ initial, onClose }: { initial?: Partial<CatalogService>; onClose: () => void }) {
  const editing = !!initial?.id;
  const [s, setS] = useState({ id: initial?.id ?? "", label: initial?.label ?? "", description: initial?.description ?? "", kind: initial?.kind ?? "app",
    docs_url: initial?.docs_url ?? "", required: initial?.required ?? false, active: initial?.active ?? true });
  const [schema, setSchema] = useState(JSON.stringify(initial?.config_schema ?? { type: "object", properties: {} }, null, 2));
  const [defaults, setDefaults] = useState(JSON.stringify(initial?.default_config ?? {}, null, 2));
  const [source, setSource] = useState(() => sourceDraft(initial?.deployment_source));
  const [runtimeKind, setRuntimeKind] = useState(() => runtimeKindDraft(initial?.runtime_contract));
  const save = useCatalogWrite(editing ? "Service updated" : "Service added");
  const del = useCatalogWrite("Service removed");
  const deploymentSource = deploymentSourceFromDraft(source);
  const sourceValid = validDeploymentSource(deploymentSource);
  const runtimeContract = runtimeContractFromDraft(runtimeKind, deploymentSource);
  const runtimeValid = validRuntimeDraft(runtimeKind, deploymentSource);
  const ok = obj(schema) && obj(defaults) && s.id && s.label && sourceValid && runtimeValid;
  const set = (k: keyof typeof s) => (v: string | boolean) => setS({ ...s, [k]: v });
  const updateSourceField = (k: keyof typeof source) => (v: string) => setSource((current) => ({ ...current, [k]: v }));
  return (
    <Drawer isOpen onOpenChange={(o) => !o && onClose()} title={editing ? `Edit ${initial.label}` : "New service"}
      subtitle="Defaults are the system layer; tiers and customers change them within each lock mode."
      footer={<>
        {editing && <Button tooltip="Delete" variant="danger" onPress={() => del.mutate({ method: "DELETE", path: catalogPaths.service(s.id) }, { onSuccess: onClose })}>Delete</Button>}
        <div className="flex-1" />
        <Button tooltip="Cancel" variant="ghost" onPress={onClose}>Cancel</Button>
        <Button tooltip="Save service" variant="primary" isDisabled={!ok || save.isPending}
          onPress={() => save.mutate({ method: editing ? "PUT" : "POST", path: catalogPaths.service(initial?.id),
            body: { ...s, config_schema: obj(schema), default_config: obj(defaults), deployment_source: deploymentSource, runtime_contract: runtimeContract } }, { onSuccess: onClose })}>Save service</Button>
      </>}>
      <div className="grid gap-4 px-6 py-5">
        <div className="grid grid-cols-2 gap-4">
          <TextField label="Id" value={s.id} onChange={set("id")} isDisabled={!!initial} mono isRequired />
          <TextField label="Label" value={s.label} onChange={set("label")} isRequired />
          <TextField label="Kind" value={s.kind} onChange={set("kind")} />
          <TextField label="Docs" value={s.docs_url} onChange={set("docs_url")} mono />
        </div>
        <TextField label="Description" multiline value={s.description} onChange={set("description")} />
        <TextField label="Config schema (JSON Schema)" multiline mono value={schema} onChange={setSchema} description={obj(schema) ? undefined : "Must be a JSON object"} />
        <TextField label="System defaults" multiline mono value={defaults} onChange={setDefaults} description={obj(defaults) ? undefined : "Must be a JSON object"} />
        <section className="grid gap-3 rounded-lg border border-line p-4" aria-label="Deployment source">
          <div>
            <h3 className="text-sm font-semibold text-fg">Deployment source</h3>
            <p className="mt-1 text-xs text-fg-muted">Pin a repository commit or OCI image digest. This saves source metadata; it does not mean a service has been deployed.</p>
          </div>
          <Select label="Source type" options={[
            { id: "none", label: "No deployment source" },
            { id: "github", label: "GitHub repository" },
            { id: "git", label: "Custom Git repository" },
            { id: "oci", label: "OCI image" },
          ]} value={source.kind} onChange={(kind) => {
            const next = kind as typeof source.kind;
            setSource((current) => ({ ...current, kind: next }));
            if (next !== "oci") setRuntimeKind("none");
          }} />
          {source.kind !== "none" && <>
            <TextField label={source.kind === "oci" ? "Image by digest" : "HTTPS repository URL"} mono value={source.reference} onChange={updateSourceField("reference")}
              description={source.kind === "oci" ? "Use a fully qualified image@sha256 digest; mutable tags are not accepted." : "The fetch runtime will separately enforce its configured egress policy."} />
            {source.kind !== "oci" && <>
              <TextField label="Commit" mono value={source.commit} onChange={updateSourceField("commit")} description="Use the full immutable commit hash, not a branch or tag." />
              <TextField label="Build context path (optional)" mono value={source.context_path} onChange={updateSourceField("context_path")} description="A relative directory inside the pinned repository." />
            </>}
            <TextField label="Credential reference (optional)" mono value={source.credential_ref} onChange={updateSourceField("credential_ref")} autoComplete="off"
              description="Use a vaultix:// or sops:// reference. Secret values are never accepted here." />
            {!sourceValid && <p role="alert" className="text-sm text-danger">Use an HTTPS repository pinned to a full commit, or an OCI image pinned by digest. Credentials must be references.</p>}
          </>}
        </section>
        {source.kind !== "none" && <section className="grid gap-3 rounded-lg border border-line p-4" aria-label="Runtime compatibility">
          <div>
            <h3 className="text-sm font-semibold text-fg">Runtime compatibility</h3>
            <p className="mt-1 text-xs text-fg-muted">A pinned source alone is metadata. The current Incus declaration identifies provider compatibility; it does not enable deployment.</p>
          </div>
          <Select label="Runtime declaration" options={[
            { id: "none", label: "Metadata only; no runtime declared" },
            ...(source.kind === "oci" ? [{ id: "incus_oci_container", label: "Incus OCI application container" }] : []),
          ]} value={runtimeKind} onChange={(kind) => setRuntimeKind(kind as typeof runtimeKind)} />
          {runtimeKind !== "none" && <p className="text-xs text-fg-muted">Provider execution, persistent storage, secrets, health, upgrades, backup, restore, and removal are not implemented for this declaration yet.</p>}
          {!runtimeValid && <p role="alert" className="text-sm text-danger">The selected runtime does not support this source type.</p>}
        </section>}
        <div className="flex gap-6">
          <Switch isSelected={s.required} onChange={set("required")} tooltip="Required services are locked on for every plan">Required</Switch>
          <Switch isSelected={s.active} onChange={set("active")} tooltip="Inactive services are left out of effective config">Active</Switch>
        </div>
      </div>
    </Drawer>
  );
}
