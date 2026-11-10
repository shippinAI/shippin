import { useEffect, useRef, useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import { Disclosure } from "../../../lib/design/components/disclosure/disclosure";
import { TextField } from "../../../lib/design/components/field/text-field";
import { Select } from "../../../lib/design/components/select/select";
import { Checkbox } from "../../../lib/design/components/checkbox/checkbox";
import { useAddHost, useTestHost, type ConnectResult, type InfraField, type InfraNode, type InfraOverview } from "../../../lib/hooks/use-infra";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { fieldsComplete, filledValues, initialValues, noteOK, siteOK, slugFrom, slugOK } from "../model";

const NEW_HOST = "new";

type Props = {
  data: InfraOverview;
  isOpen: boolean;
  initialKind?: string;
  initialExisting?: string;
  editNode?: InfraNode;
  onOpenChange: (open: boolean) => void;
  onAdded: (kind: string) => void;
};

/** What a test found, and if the server's certificate is unknown, the decision to trust it. */
function TestOutcome({ result, onTrust }: { result: ConnectResult; onTrust: (pem: string) => void }) {
  const t = useText();
  const c = result.certificate;
  if (c) {
    return (
      <div className="space-y-2 rounded-lg border border-warning/30 bg-warning/10 p-3 text-sm" data-testid="trust-prompt">
        <p className="font-medium text-warning">{t("infra.trust.title")}</p>
        <p className="text-fg-muted">{t("infra.trust.body")}</p>
        <dl className="space-y-1 text-fg-muted">
          <div><dt className="inline font-medium text-fg">{t("infra.trust.fingerprint")} </dt><dd className="inline break-all font-mono text-xs">{c.fingerprint}</dd></div>
          <div><dt className="inline font-medium text-fg">{t("infra.trust.names")} </dt><dd className="inline">{c.names.join(", ")}</dd></div>
          <div><dt className="inline font-medium text-fg">{t("infra.trust.expires")} </dt><dd className="inline">{new Date(c.not_after).toLocaleDateString()}</dd></div>
        </dl>
        <Button size="sm" variant="secondary" tooltip={t("infra.trust.acceptTooltip")} onPress={() => onTrust(c.pem)}>{t("infra.trust.accept")}</Button>
      </div>
    );
  }
  if (result.ok) {
    return (
      <p role="status" data-testid="test-ok" className="rounded-lg border border-success/30 bg-success/10 px-3 py-2 text-sm text-success">
        {result.detail ?? t("infra.dialog.testOk", { version: result.version ?? "" })}
        {result.nodes && result.nodes.length > 1 ? ` ${t("infra.dialog.testNodes", { nodes: result.nodes.join(", ") })}` : ""}
      </p>
    );
  }
  return <p role="alert" className="break-words rounded-lg border border-danger/30 bg-danger/10 px-3 py-2 text-sm text-danger">{result.error}</p>;
}

/** The form comes entirely from the overview: which providers exist and what each one asks for. */
export function AddHostDialog({ data, isOpen, initialKind, initialExisting, editNode, onOpenChange, onAdded }: Props) {
  const t = useText();
  const add = useAddHost();
  const test = useTestHost();
  const providers = data.providers.filter((p) => p.attachable);
  const [kind, setKind] = useState("");
  const [target, setTarget] = useState(NEW_HOST);
  const [label, setLabel] = useState("");
  const [slug, setSlug] = useState("");
  const [slugEdited, setSlugEdited] = useState(false);
  const [site, setSite] = useState("");
  const [values, setValues] = useState<Record<string, string>>({});
  const [trusted, setTrusted] = useState("");
  const [allowTenantSecrets, setAllowTenantSecrets] = useState(false);
  const [note, setNote] = useState("");
  const provider = providers.find((p) => p.kind === kind) ?? providers[0];
  const fields = provider?.fields ?? [];

  const resetTest = () => test.reset();
  // The form can be longer than the dialog, and what a test found must not land out of sight.
  const outcome = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (test.data || test.isError) outcome.current?.scrollIntoView({ block: "nearest" });
  }, [test.data, test.isError]);

  useEffect(() => {
    if (!isOpen) return;
    const first = providers.find((p) => p.kind === initialKind) ?? providers[0];
    setKind(first?.kind ?? "");
    setValues({ ...initialValues(first?.fields ?? []), ...(editNode?.config ?? {}) });
    setTarget(initialExisting ?? NEW_HOST);
    setLabel("");
    setSlug("");
    setSlugEdited(false);
    setSite(data.default_site ?? "");
    setTrusted("");
    setAllowTenantSecrets(editNode?.allow_tenant_deployment_secrets ?? false);
    setNote("");
    resetTest();
    // Reset only when the dialog opens: the overview refreshes on a timer behind it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen, initialKind, initialExisting, editNode]);

  const pick = (next: string) => {
    setKind(next);
    setValues(initialValues(providers.find((p) => p.kind === next)?.fields ?? []));
    setTrusted("");
    setAllowTenantSecrets(false);
    resetTest();
  };
  const setField = (name: string, value: string) => {
    setValues((s) => ({ ...s, [name]: value }));
    resetTest();
  };
  const existing = data.unattached.find((s) => s.id === target);
  const isNew = !existing && !editNode;
  const hostOK = !isNew || (label.trim().length > 0 && slugOK(slug) && (!site || siteOK(site)));
  const editableFields = editNode ? fields.map((f) => ({ ...f, required: f.required && !f.secret })) : fields;
  const filled = fieldsComplete(editableFields, values) && Object.keys(filledValues(fields, values)).length > 0;
  const valid = !!provider && hostOK && filled && noteOK(note) && test.data?.ok === true;
  const hostValues = () => ({ kind: provider!.kind, values: filledValues(fields, values), trustedCert: trusted || undefined,
    allowTenantDeploymentSecrets: allowTenantSecrets, serverId: editNode?.id });
  const runTest = (trust?: string) => {
    if (!provider) return;
    test.mutate({ ...hostValues(), trustedCert: trust ?? (trusted || undefined) });
  };
  const trust = (pem: string) => {
    setTrusted(pem);
    runTest(pem);
  };
  const submit = () => {
    if (!provider) return;
    // A detached host keeps its slug, so adding it back under that slug reattaches it.
    const detached = existing ?? data.unattached.find((s) => s.slug === slug);
    if (editNode) {
      add.mutate({ ...hostValues(), existing: editNode.id, label: editNode.label, note: note.trim() }, {
        onSuccess: () => { onOpenChange(false); onAdded(provider.kind); },
      });
      return;
    }
    const request = detached
      ? { ...hostValues(), existing: detached.id, label: label.trim() || detached.label || detached.slug, note: note.trim() }
      : { ...hostValues(), label: label.trim(), slug, site, note: note.trim() };
    add.mutate(request, {
      onSuccess: () => {
        onOpenChange(false);
        onAdded(provider.kind);
      },
    });
  };
  const targets = [{ id: NEW_HOST, label: t("infra.dialog.targetNew") }, ...data.unattached.map((s) => ({ id: s.id, label: s.label || s.slug }))];
  const field = (f: InfraField) => (
    <TextField key={`${provider?.kind}:${f.name}`} label={f.label} value={values[f.name] ?? ""} mono={!f.secret} type={f.secret ? "password" : "text"} isRequired={f.required}
      description={f.help} placeholder={f.secret && editNode && (editNode.has_secret || editNode.config?.token_env || editNode.config?.cert_file) ? t("infra.dialog.secretKeep") : f.placeholder} autoComplete="off" onChange={(v) => setField(f.name, v)} />
  );
  const advanced = fields.filter((f) => f.advanced);
  return (
    <ModalDialog isOpen={isOpen} onOpenChange={onOpenChange} title={editNode ? t("infra.dialog.editTitle", { label: editNode.label }) : t("infra.dialog.title")} description={t("infra.dialog.description")}>
      <div className="max-h-[75vh] space-y-4 overflow-y-auto px-6 py-5">
        {editNode ? <p className="text-base text-fg-muted">{provider?.label} · {editNode.slug}</p> : <Select label={t("infra.dialog.provider")} options={providers.map((p) => ({ id: p.kind, label: p.label }))} value={provider?.kind ?? ""} onChange={pick} />}
        {provider?.description && <p className="text-sm text-fg-muted">{provider.description}</p>}
        {targets.length > 1 && <Select label={t("infra.dialog.target")} options={targets} value={target} onChange={setTarget} />}
        {isNew && (
          <>
            <TextField label={t("infra.dialog.name")} value={label} autoFocus
              onChange={(v) => { setLabel(v); if (!slugEdited) setSlug(slugFrom(v)); }} />
            <div className="grid gap-4 sm:grid-cols-2">
              <TextField label={t("infra.dialog.slug")} value={slug} mono description={t("infra.dialog.slug.help")}
                onChange={(v) => { setSlug(v.toLowerCase()); setSlugEdited(true); }} />
              <TextField label={t("infra.dialog.site")} value={site} mono description={t("infra.dialog.site.help")} onChange={(v) => setSite(v.toLowerCase())} />
            </div>
          </>
        )}
        {fields.filter((f) => !f.advanced).map(field)}
        {provider?.tenant_secret_exposure_policy && <Checkbox isSelected={allowTenantSecrets} onChange={setAllowTenantSecrets}
          description={t("infra.dialog.tenantSecrets.help")}>{t("infra.dialog.tenantSecrets.label")}</Checkbox>}
        {advanced.length > 0 && <Disclosure title={t("infra.dialog.advanced")}><div className="space-y-4 p-4">{advanced.map(field)}</div></Disclosure>}
        {trusted && <p className="text-sm text-fg-muted">{t("infra.trust.accepted")}</p>}
        <div ref={outcome} className="space-y-2 empty:hidden">
          {test.data && <TestOutcome result={test.data} onTrust={trust} />}
          {test.isError && <p role="alert" className="text-sm text-danger">{test.error.message}</p>}
        </div>
        <TextField label={t("infra.note.label")} description={t("infra.note.help")} value={note} onChange={setNote} />
        {add.isError && <p role="alert" data-testid="add-error" className="break-words text-sm text-danger">{add.error.message}</p>}
        <div className="flex flex-wrap justify-end gap-2 border-t border-line pt-4">
          <Button variant="ghost" tooltip={t("infra.dialog.cancelTooltip")} onPress={() => onOpenChange(false)}>{t("infra.dialog.cancel")}</Button>
          <Button variant="secondary" isDisabled={!filled || test.isPending} tooltip={t("infra.dialog.testTooltip")} onPress={() => runTest()}>
            {test.isPending ? t("infra.dialog.testing") : t("infra.dialog.test")}
          </Button>
          <Button variant="primary" isDisabled={!valid || add.isPending} tooltip={valid ? t("infra.dialog.submitTooltip") : test.data?.ok ? t("infra.dialog.incomplete") : t("infra.dialog.testRequired")} onPress={submit}>
            {add.isPending ? t("infra.dialog.adding") : editNode ? t("infra.dialog.saveEdit") : t("infra.dialog.submit")}
          </Button>
        </div>
      </div>
    </ModalDialog>
  );
}
