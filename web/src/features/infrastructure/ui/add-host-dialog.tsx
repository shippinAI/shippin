import { useEffect, useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../lib/design/components/field/text-field";
import { Select } from "../../../lib/design/components/select/select";
import { useAddHost, type InfraOverview } from "../../../lib/hooks/use-infra";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { fieldsComplete, filledValues, initialValues, siteOK, slugFrom, slugOK } from "../model";

const NEW_HOST = "new";

type Props = {
  data: InfraOverview;
  isOpen: boolean;
  initialKind?: string;
  initialExisting?: string;
  onOpenChange: (open: boolean) => void;
  onAdded: (kind: string) => void;
};

/** The form comes entirely from the overview: which providers exist and what each one asks for. */
export function AddHostDialog({ data, isOpen, initialKind, initialExisting, onOpenChange, onAdded }: Props) {
  const t = useText();
  const add = useAddHost();
  const providers = data.providers.filter((p) => p.attachable);
  const [kind, setKind] = useState("");
  const [target, setTarget] = useState(NEW_HOST);
  const [label, setLabel] = useState("");
  const [slug, setSlug] = useState("");
  const [slugEdited, setSlugEdited] = useState(false);
  const [site, setSite] = useState("");
  const [values, setValues] = useState<Record<string, string>>({});
  const provider = providers.find((p) => p.kind === kind) ?? providers[0];
  const fields = provider?.fields ?? [];

  useEffect(() => {
    if (!isOpen) return;
    const first = providers.find((p) => p.kind === initialKind) ?? providers[0];
    setKind(first?.kind ?? "");
    setValues(initialValues(first?.fields ?? []));
    setTarget(initialExisting ?? NEW_HOST);
    setLabel("");
    setSlug("");
    setSlugEdited(false);
    setSite(data.default_site ?? "");
    // Reset only when the dialog opens: the overview refreshes on a timer behind it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen, initialKind, initialExisting]);

  const pick = (next: string) => {
    setKind(next);
    setValues(initialValues(providers.find((p) => p.kind === next)?.fields ?? []));
  };
  const existing = data.unattached.find((s) => s.id === target);
  const isNew = !existing;
  const hostOK = !isNew || (label.trim().length > 0 && slugOK(slug) && (!site || siteOK(site)));
  const valid = !!provider && hostOK && fieldsComplete(fields, values);
  const submit = () => {
    if (!provider) return;
    const body = { kind: provider.kind, values: filledValues(fields, values) };
    const request = existing
      ? { ...body, existing: existing.id, label: existing.label || existing.slug }
      : { ...body, label: label.trim(), slug, site };
    add.mutate(request, {
      onSuccess: () => {
        onOpenChange(false);
        onAdded(provider.kind);
      },
    });
  };
  const targets = [{ id: NEW_HOST, label: t("infra.dialog.targetNew") }, ...data.unattached.map((s) => ({ id: s.id, label: s.label || s.slug }))];
  return (
    <ModalDialog isOpen={isOpen} onOpenChange={onOpenChange} title={t("infra.dialog.title")} description={t("infra.dialog.description")}>
      <div className="max-h-[70vh] space-y-4 overflow-y-auto px-6 py-5">
        <Select label={t("infra.dialog.provider")} options={providers.map((p) => ({ id: p.kind, label: p.label }))} value={provider?.kind ?? ""} onChange={pick} />
        {provider?.description && <p className="text-[13px] text-fg-muted">{provider.description}</p>}
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
        {fields.map((f) => (
          <TextField key={`${provider?.kind}:${f.name}`} label={f.label} value={values[f.name] ?? ""} mono isRequired={f.required}
            description={f.help} placeholder={f.placeholder} onChange={(v) => setValues((s) => ({ ...s, [f.name]: v }))} />
        ))}
        <div className="flex justify-end gap-2 border-t border-line pt-4">
          <Button variant="ghost" tooltip={t("infra.dialog.cancelTooltip")} onPress={() => onOpenChange(false)}>{t("infra.dialog.cancel")}</Button>
          <Button variant="primary" isDisabled={!valid || add.isPending} tooltip={valid ? t("infra.dialog.submitTooltip") : t("infra.dialog.incomplete")} onPress={submit}>
            {t("infra.dialog.submit")}
          </Button>
        </div>
      </div>
    </ModalDialog>
  );
}
