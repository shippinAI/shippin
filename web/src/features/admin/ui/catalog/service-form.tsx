import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { callAction } from "../../../../lib/api/actions";
import type { CatalogService } from "../../../../lib/api/catalog-types";
import { Button } from "../../../../lib/design/components/button/button";
import { Drawer } from "../../../../lib/design/components/drawer/drawer";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Switch } from "../../../../lib/design/components/switch/switch";
import { Select } from "../../../../lib/design/components/select/select";
import { toast } from "../../../../lib/design/components/toast/toast-queue";
import { deploymentSourceFromDraft, sourceDraft, validDeploymentSource } from "./service-source";
import type { CatalogRuntimeContractV3 } from "../../../../lib/api/catalog-types";
import {
  emptyRuntimeContractDraft,
  runtimeContractFromDraft,
  runtimeKindDraft,
  validRuntimeContract,
  validRuntimeDraft,
} from "./service-runtime";
import { RuntimeRecipeForm } from "./service-runtime-form";

const obj = (s: string): Record<string, unknown> | null => {
  try {
    const v: unknown = JSON.parse(s);
    return v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : null;
  } catch {
    return null;
  }
};

/** Create or edit a catalog service, including its JSON config schema and defaults. Every change carries a reason for the audit log. */
export function ServiceForm({ initial, onClose }: { initial?: CatalogService; onClose: () => void }) {
  const editing = !!initial;
  const [reason, setReason] = useState("");
  const qc = useQueryClient();
  const [s, setS] = useState({
    id: initial?.id ?? "",
    label: initial?.label ?? "",
    description: initial?.description ?? "",
    kind: initial?.kind ?? "app",
    docs_url: initial?.docs_url ?? "",
    required: initial?.required ?? false,
    active: initial?.active ?? true,
  });
  const [schema, setSchema] = useState(JSON.stringify(initial?.config_schema ?? { type: "object", properties: {} }, null, 2));
  const [defaults, setDefaults] = useState(JSON.stringify(initial?.default_config ?? {}, null, 2));
  const [source, setSource] = useState(() => sourceDraft(initial?.deployment_source));
  const [runtimeKind, setRuntimeKind] = useState(() => runtimeKindDraft(initial?.runtime_contract));
  const [runtimeRecipe, setRuntimeRecipe] = useState<CatalogRuntimeContractV3>(() => {
    // The server stores only contracts it validated; the guard narrows the generated shape to the recipe types this form edits.
    const saved = validRuntimeContract(initial?.runtime_contract) ? initial.runtime_contract : null;
    if (saved?.version === 3) return saved;
    if (saved?.version === 2) return { ...saved, version: 3, environment: [] };
    return emptyRuntimeContractDraft();
  });
  const [runtimeVersion, setRuntimeVersion] = useState<1 | 2 | 3>(() =>
    initial?.runtime_contract?.version === 1
      ? 1
      : initial?.runtime_contract?.version === 3
        ? 3
        : initial?.runtime_contract?.version === 2
          ? 2
          : 3,
  );
  const runtimeJSON = JSON.stringify(
    runtimeVersion === 1
      ? { version: 1, kind: "incus_oci_container", providers: ["incus"] }
      : runtimeVersion === 2
        ? (({ environment: _environment, ...runtimeV2 }) => ({ ...runtimeV2, version: 2 }))(runtimeRecipe)
        : runtimeRecipe,
  );
  const deploymentSource = deploymentSourceFromDraft(source);
  const sourceValid = validDeploymentSource(deploymentSource);
  const runtimeContract = runtimeContractFromDraft(runtimeKind, deploymentSource, runtimeJSON);
  const runtimeValid = validRuntimeDraft(runtimeKind, deploymentSource, runtimeJSON);
  const reasonOK = reason.trim().length >= 5;
  const ok = obj(schema) && obj(defaults) && s.id && s.label && sourceValid && runtimeValid && reasonOK;
  const done = (msg: string) => ({
    onSuccess: () => {
      toast.success(msg);
      void qc.invalidateQueries({ queryKey: ["catalog"] });
      onClose();
    },
    onError: (e: Error) => toast.error("Not saved", e.message),
  });
  // The action takes exactly its input fields: on an update the id is in the path, not the body.
  const { id, ...fields } = s;
  const input = {
    ...fields,
    config_schema: obj(schema) ?? {},
    default_config: obj(defaults) ?? {},
    deployment_source: deploymentSource,
    runtime_contract: runtimeContract,
    reason,
  };
  const save = useMutation({
    mutationFn: () =>
      initial
        ? callAction("shippin.services.catalog_service.update", { catalogServiceId: id }, input)
        : callAction("shippin.services.catalog_service.create", {}, { id, ...input }),
    ...done(editing ? "Service updated" : "Service added"),
  });
  const del = useMutation({
    mutationFn: () => callAction("shippin.services.catalog_service.delete", { catalogServiceId: id }, { reason }),
    ...done("Service removed"),
  });
  const set = (k: keyof typeof s) => (v: string | boolean) => setS({ ...s, [k]: v });
  const updateSourceField = (k: keyof typeof source) => (v: string) => setSource((current) => ({ ...current, [k]: v }));
  return (
    <Drawer
      isOpen
      onOpenChange={(o) => !o && onClose()}
      title={editing ? `Edit ${initial.label}` : "New service"}
      subtitle="Defaults are the system layer; tiers and customers change them within each lock mode."
      footer={
        <>
          {editing && (
            <Button
              tooltip={reasonOK ? "Delete this service" : "Give a reason first"}
              variant="danger"
              isDisabled={!reasonOK || del.isPending}
              onPress={() => del.mutate()}
            >
              Delete
            </Button>
          )}
          <div className="flex-1" />
          <Button tooltip="Cancel" variant="ghost" onPress={onClose}>
            Cancel
          </Button>
          <Button
            tooltip={reasonOK ? "Save service" : "Give a reason first"}
            variant="primary"
            isDisabled={!ok || save.isPending}
            onPress={() => save.mutate()}
          >
            Save service
          </Button>
        </>
      }
    >
      <div className="grid gap-4 px-6 py-5">
        <TextField
          label="Reason (audited)"
          value={reason}
          onChange={setReason}
          placeholder="Why this changes"
          description="Kept in the audit log with the change. At least 5 characters."
        />
        <div className="grid grid-cols-2 gap-4">
          <TextField label="Id" value={s.id} onChange={set("id")} isDisabled={!!initial} mono isRequired />
          <TextField label="Label" value={s.label} onChange={set("label")} isRequired />
          <TextField label="Kind" value={s.kind} onChange={set("kind")} />
          <TextField label="Docs" value={s.docs_url} onChange={set("docs_url")} mono />
        </div>
        <TextField label="Description" multiline value={s.description} onChange={set("description")} />
        <TextField
          label="Config schema (JSON Schema)"
          multiline
          mono
          value={schema}
          onChange={setSchema}
          description={obj(schema) ? undefined : "Must be a JSON object"}
        />
        <TextField
          label="System defaults"
          multiline
          mono
          value={defaults}
          onChange={setDefaults}
          description={obj(defaults) ? undefined : "Must be a JSON object"}
        />
        <section className="grid gap-3 rounded-lg border border-line p-4" aria-label="Deployment source">
          <div>
            <h3 className="text-base font-semibold text-fg">Deployment source</h3>
            <p className="mt-1 text-xs text-fg-muted">
              Pin a repository commit or OCI image digest. This saves source metadata; it does not mean a service has been deployed.
            </p>
          </div>
          <Select
            label="Source type"
            options={[
              { id: "none", label: "No deployment source" },
              { id: "github", label: "GitHub repository" },
              { id: "git", label: "Custom Git repository" },
              { id: "oci", label: "OCI image" },
            ]}
            value={source.kind}
            onChange={(kind) => {
              const next = kind as typeof source.kind;
              setSource((current) => ({ ...current, kind: next }));
              if (next !== "oci") setRuntimeKind("none");
            }}
          />
          {source.kind !== "none" && (
            <>
              <TextField
                label={source.kind === "oci" ? "Image by digest" : "HTTPS repository URL"}
                mono
                value={source.reference}
                onChange={updateSourceField("reference")}
                description={
                  source.kind === "oci"
                    ? "Use a fully qualified image@sha256 digest; mutable tags are not accepted."
                    : "The fetch runtime will separately enforce its configured egress policy."
                }
              />
              {source.kind !== "oci" && (
                <>
                  <TextField
                    label="Commit"
                    mono
                    value={source.commit}
                    onChange={updateSourceField("commit")}
                    description="Use the full immutable commit hash, not a branch or tag."
                  />
                  <TextField
                    label="Build context path (optional)"
                    mono
                    value={source.context_path}
                    onChange={updateSourceField("context_path")}
                    description="A relative directory inside the pinned repository."
                  />
                </>
              )}
              <TextField
                label="Credential reference (optional)"
                mono
                value={source.credential_ref}
                onChange={updateSourceField("credential_ref")}
                autoComplete="off"
                description="Use vaultix://<environment>/<folder>/<key> for the deployment tenant's Vaultix project, or a SOPS file reference. Secret values are never accepted here."
              />
              {!sourceValid && (
                <p role="alert" className="text-base text-danger">
                  Use an HTTPS repository pinned to a full commit, or an OCI image pinned by digest. Credentials must be references.
                </p>
              )}
            </>
          )}
        </section>
        {source.kind !== "none" && (
          <section className="grid gap-3 rounded-lg border border-line p-4" aria-label="Runtime compatibility">
            <div>
              <h3 className="text-base font-semibold text-fg">Runtime compatibility</h3>
              <p className="mt-1 text-xs text-fg-muted">
                A pinned source alone is metadata. The current Incus declaration identifies provider compatibility; it does not enable
                deployment.
              </p>
            </div>
            <Select
              label="Runtime declaration"
              options={[
                { id: "none", label: "Metadata only; no runtime declared" },
                ...(source.kind === "oci" ? [{ id: "incus_oci_container", label: "Incus OCI application container" }] : []),
              ]}
              value={runtimeKind}
              onChange={(kind) => {
                const next = kind as typeof runtimeKind;
                setRuntimeKind(next);
              }}
            />
            {runtimeKind !== "none" && (
              <>
                <Select
                  label="Recipe version"
                  options={[
                    { id: "1", label: "Compatibility metadata only" },
                    { id: "2", label: "Resources, storage, network and health" },
                    { id: "3", label: "Recipe with config-to-environment bindings" },
                  ]}
                  value={String(runtimeVersion)}
                  onChange={(v) => setRuntimeVersion(v === "1" ? 1 : v === "2" ? 2 : 3)}
                />
                {runtimeVersion === 1 && (
                  <p className="text-xs text-fg-muted">
                    Version 1 preserves provider compatibility metadata without resource, storage, network, or health requirements. It does
                    not enable deployment.
                  </p>
                )}
                {(runtimeVersion === 2 || runtimeVersion === 3) && (
                  <>
                    <RuntimeRecipeForm value={runtimeRecipe} onChange={setRuntimeRecipe} showEnvironment={runtimeVersion === 3} />
                  </>
                )}
              </>
            )}
            {!runtimeValid && (
              <p role="alert" className="text-base text-danger">
                Enter a valid runtime recipe for the selected OCI source. Version 2 requires bounded resources, storage, network ports, and
                a matching health check; version 3 also validates its config bindings.
              </p>
            )}
          </section>
        )}
        <div className="flex gap-6">
          <Switch isSelected={s.required} onChange={set("required")} tooltip="Required services are locked on for every plan">
            Required
          </Switch>
          <Switch isSelected={s.active} onChange={set("active")} tooltip="Inactive services are left out of effective config">
            Active
          </Switch>
        </div>
      </div>
    </Drawer>
  );
}
