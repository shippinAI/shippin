import { useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { Button } from "../../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../../lib/design/components/card/card";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Switch } from "../../../../lib/design/components/switch/switch";
import { Tabs } from "../../../../lib/design/components/tabs/tabs";
import { useAdminAccess } from "../../../../lib/hooks/use-admin";
import { usePlatformSettings, useSetPlatformSetting } from "../../../../lib/hooks/use-platform-settings";
import { compactUi, type NavEntryConfig, type ProviderConfig, type ProviderFieldConfig, type UiConfig } from "../../../../lib/ui-config/model";
import { textDefaults, type TextKey } from "../../../../lib/ui-config/text-keys";
import { uiConfigKey, useUiConfig } from "../../../../lib/ui-config/use-ui-config";
import { adminNav } from "../../../../template/admin/admin-nav";
import { accountNav, infrastructureNav, navSections, operatorNav, tenantNav } from "../../../../template/nav-items";
import { PageHeader } from "../../../../template/page-header";

const UI_KEY = "ui.config";
const REASON_MIN = 5;

const allSections = Object.values(navSections);
const allItems = [...tenantNav, ...infrastructureNav, ...accountNav, ...operatorNav, ...adminNav];

function upsert<T>(list: T[] | undefined, match: (e: T) => boolean, patch: Partial<T>, fresh: T): T[] {
  const out = [...(list ?? [])];
  const i = out.findIndex(match);
  if (i >= 0) out[i] = { ...out[i]!, ...patch };
  else out.push({ ...fresh, ...patch });
  return out;
}

function NavRow({ id, fallback, entry, disabled, onChange }: { id: string; fallback: string; entry?: NavEntryConfig; disabled: boolean; onChange: (p: Partial<NavEntryConfig>) => void }) {
  return (
    <li className="grid items-end gap-3 px-5 py-3 sm:grid-cols-[1fr_6rem_auto]">
      <TextField label={fallback} description={id} value={entry?.label ?? ""} placeholder={fallback} isDisabled={disabled} onChange={(label) => onChange({ label })} />
      <TextField label="Position" type="number" value={entry?.order ? String(entry.order) : ""} isDisabled={disabled} onChange={(v) => onChange({ order: Number(v) || 0 })} />
      <Switch isSelected={!entry?.hidden} isDisabled={disabled} tooltip={`Show or hide ${fallback}`} onChange={(on) => onChange({ hidden: !on })}>Shown</Switch>
    </li>
  );
}

function ProviderCardForm({ kind, fieldNames, entry, disabled, onChange }: { kind: string; fieldNames: string[]; entry?: ProviderConfig; disabled: boolean; onChange: (p: Partial<ProviderConfig>) => void }) {
  const fieldOf = (name: string) => entry?.fields?.find((f) => f.name === name);
  const setField = (name: string, patch: Partial<ProviderFieldConfig>) =>
    onChange({ fields: upsert(entry?.fields, (f) => f.name === name, patch, { name }) });
  return (
    <Card>
      <CardHeader title={entry?.label || kind} description={kind}
        actions={<Switch isSelected={!entry?.hidden} isDisabled={disabled} tooltip={`Offer or hide ${kind}`} onChange={(on) => onChange({ hidden: !on })}>Offered</Switch>} />
      <div className="space-y-4 px-5 pb-5">
        <div className="grid gap-4 sm:grid-cols-2">
          <TextField label="Name" value={entry?.label ?? ""} placeholder={kind} isDisabled={disabled} onChange={(label) => onChange({ label })} />
          <TextField label="Description" value={entry?.description ?? ""} isDisabled={disabled} onChange={(description) => onChange({ description })} />
        </div>
        <ul className="space-y-3">
          {fieldNames.map((name) => {
            const f = fieldOf(name);
            return (
              <li key={name} className="grid gap-3 rounded-lg border border-line p-3 sm:grid-cols-3">
                <TextField label="Field name" description={name} value={f?.label ?? ""} placeholder={name} isDisabled={disabled} onChange={(label) => setField(name, { label })} />
                <TextField label="Help" value={f?.help ?? ""} isDisabled={disabled} onChange={(help) => setField(name, { help })} />
                <TextField label="Default" value={f?.default ?? ""} isDisabled={disabled} onChange={(v) => setField(name, { default: v })} />
              </li>
            );
          })}
        </ul>
      </div>
    </Card>
  );
}

/** Provider panel: what the panel calls things and shows. Saved through the audited platform setting. */
export function InterfacePage() {
  const qc = useQueryClient();
  const config = useUiConfig();
  const setting = usePlatformSettings().data?.items.find((s) => s.key === UI_KEY);
  const canWrite = (useAdminAccess().data?.staff?.permissions ?? []).includes("platform.settings.write");
  const save = useSetPlatformSetting();
  const [draft, setDraft] = useState<UiConfig>({});
  const [reason, setReason] = useState("");
  useEffect(() => {
    if (config.data) setDraft(config.data);
  }, [config.data]);
  const changed = useMemo(() => JSON.stringify(compactUi(draft)) !== JSON.stringify(compactUi(config.data ?? {})), [draft, config.data]);
  const reasonOK = reason.trim().length >= REASON_MIN;
  const disabled = !canWrite;
  const setNav = (group: "sections" | "items", id: string, patch: Partial<NavEntryConfig>) =>
    setDraft((d) => ({ ...d, nav: { ...d.nav, [group]: upsert(d.nav?.[group], (e) => e.id === id, patch, { id }) } }));
  const setProvider = (kind: string, patch: Partial<ProviderConfig>) =>
    setDraft((d) => ({ ...d, providers: upsert(d.providers, (p) => p.kind === kind, patch, { kind }) }));
  const after = {
    onSuccess: () => {
      setReason("");
      void qc.invalidateQueries({ queryKey: uiConfigKey });
      void qc.invalidateQueries({ queryKey: ["infra"] });
    },
  };
  const textKeys = Object.keys(textDefaults) as TextKey[];
  return (
    <>
      <PageHeader title="Interface" description="What this installation calls things and what it shows. The config file stays the default; every change here needs a reason and is audited." />
      {config.isError && <p role="alert" className="text-sm text-danger">{config.error.message}</p>}
      <Card className="mb-6 p-5">
        <div className="flex flex-wrap items-end gap-3">
          <div className="min-w-64 flex-1"><TextField label="Reason (audited)" value={reason} isDisabled={disabled} onChange={setReason} placeholder="Why this change" /></div>
          <Button variant="primary" size="sm" isDisabled={disabled || !changed || !reasonOK || save.isPending}
            tooltip={!changed ? "Nothing has changed" : reasonOK ? "Save and apply to everyone" : "Give a reason of at least five characters"}
            onPress={() => save.mutate({ key: UI_KEY, value: compactUi(draft), reason }, after)}>Save</Button>
          {setting?.overridden && (
            <Button variant="ghost" size="sm" isDisabled={disabled || !reasonOK || save.isPending} tooltip={reasonOK ? "Return everything to the config file" : "Give a reason of at least five characters"}
              onPress={() => save.mutate({ key: UI_KEY, value: null, reason }, after)}>Reset to config file</Button>
          )}
          <Badge tone={setting?.overridden ? "warning" : "neutral"}>{setting?.overridden ? "Overridden" : "From config"}</Badge>
        </div>
        {!canWrite && <p className="mt-3 text-sm text-fg-muted">You can view this page. Changing it needs the platform settings permission.</p>}
      </Card>
      <Tabs
        label="Interface sections"
        tabs={[
          {
            id: "navigation", label: "Navigation",
            content: (
              <>
                <Card className="mb-6 overflow-hidden">
                  <CardHeader title="Sections" description="The headings in the sidebar." />
                  <ul className="divide-y divide-line">
                    {allSections.map((s) => (
                      <NavRow key={s.id} id={s.id} fallback={s.label} disabled={disabled} entry={draft.nav?.sections?.find((e) => e.id === s.id)} onChange={(p) => setNav("sections", s.id, p)} />
                    ))}
                  </ul>
                </Card>
                <Card className="overflow-hidden">
                  <CardHeader title="Items" description="Leave a name empty to keep the default. A lower position moves an item up; hiding an item does not grant or remove access." />
                  <ul className="divide-y divide-line">
                    {allItems.map((i) => (
                      <NavRow key={i.id} id={i.id} fallback={i.label} disabled={disabled} entry={draft.nav?.items?.find((e) => e.id === i.id)} onChange={(p) => setNav("items", i.id, p)} />
                    ))}
                  </ul>
                </Card>
              </>
            ),
          },
          {
            id: "providers", label: "Providers",
            content: (
              <div className="space-y-6">
                <Card className="p-5">
                  <TextField label="Default site" description="Prefills the site of a host added from the panel." value={draft.default_site ?? ""} isDisabled={disabled}
                    onChange={(default_site) => setDraft((d) => ({ ...d, default_site }))} />
                </Card>
                {(draft.drivers ?? []).map((d) => (
                  <ProviderCardForm key={d.kind} kind={d.kind} fieldNames={d.fields.map((f) => f.name)} disabled={disabled}
                    entry={draft.providers?.find((p) => p.kind === d.kind)} onChange={(p) => setProvider(d.kind, p)} />
                ))}
              </div>
            ),
          },
          {
            id: "text", label: "Text",
            content: (
              <Card className="overflow-hidden">
                <CardHeader title="Panel text" description="Words in {braces} are filled in by the panel. Leave a line empty to keep the default." />
                <ul className="divide-y divide-line">
                  {textKeys.map((k) => (
                    <li key={k} className="px-5 py-3">
                      <TextField label={textDefaults[k]} description={k} value={draft.text?.[k] ?? ""} isDisabled={disabled}
                        onChange={(v) => setDraft((d) => ({ ...d, text: { ...d.text, [k]: v } }))} />
                    </li>
                  ))}
                </ul>
              </Card>
            ),
          },
        ]}
      />
    </>
  );
}
