import { useMe } from "../../../../lib/hooks/use-api";
import { useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { Button } from "../../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../../lib/design/components/card/card";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Select } from "../../../../lib/design/components/select/select";
import { Switch } from "../../../../lib/design/components/switch/switch";
import { Tabs } from "../../../../lib/design/components/tabs/tabs";
import { useAdminAccess } from "../../../../lib/hooks/use-admin";
import { usePlatformSettings, useSetPlatformSetting } from "../../../../lib/hooks/use-platform-settings";
import { compactUi, type ImageConfig, type ImageRemoteConfig, type NavEntryConfig, type ProviderConfig, type ProviderFieldConfig, type UiConfig } from "../../../../lib/ui-config/model";
import { textDefaults, type TextKey } from "../../../../lib/ui-config/text-keys";
import { uiConfigKey, useUiConfig } from "../../../../lib/ui-config/use-ui-config";
import { adminNav } from "../../../../template/admin/admin-nav";
import { accountNav, featureOn, infrastructureNav, navSections, operatorNav, tenantNav } from "../../../../template/nav-items";
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

const imageKinds = [{ id: "any", label: "Any guest" }, { id: "vm", label: "Virtual machine" }, { id: "ct", label: "Container" }];
const imageProtocols = [{ id: "simplestreams", label: "Simple Streams" }, { id: "incus", label: "Incus" }, { id: "oci", label: "OCI registry" }];
const row = "grid items-end gap-3 rounded-lg border border-line p-3";

/** The images a guest can be made from, and on drivers that fetch them, the servers they come from. */
function ImageSettings({ kind, usesServers, entry, disabled, onChange }: { kind: string; usesServers: boolean; entry?: ProviderConfig; disabled: boolean; onChange: (p: Partial<ProviderConfig>) => void }) {
  const images = entry?.images ?? [];
  const remotes = entry?.image_remotes ?? [];
  const setImage = (at: number, patch: Partial<ImageConfig>) => onChange({ images: images.map((m, i) => (i === at ? { ...m, ...patch } : m)) });
  const setRemote = (at: number, patch: Partial<ImageRemoteConfig>) => onChange({ image_remotes: remotes.map((r, i) => (i === at ? { ...r, ...patch } : r)) });
  return (
    <div className="space-y-5 border-t border-line pt-4">
      {usesServers && (
        <section aria-label={`Image servers for ${kind}`} className="space-y-3">
          <div>
            <h4 className="text-base font-semibold text-fg">Image servers</h4>
            <p className="text-sm text-fg-muted">Where this provider downloads images from. People can only choose a server listed here. HTTPS only.</p>
          </div>
          {remotes.map((r, i) => (
            <div key={i} className={`${row} sm:grid-cols-[1fr_2fr_10rem_auto]`}>
              <TextField label="Name" value={r.name} isDisabled={disabled} onChange={(name) => setRemote(i, { name })} />
              <TextField label="Server URL" type="url" placeholder="https://images.example.org" value={r.server} isDisabled={disabled} onChange={(server) => setRemote(i, { server })} />
              <Select label="Protocol" options={imageProtocols} value={r.protocol} isDisabled={disabled} onChange={(protocol) => setRemote(i, { protocol })} />
              <Button variant="ghost" size="sm" isDisabled={disabled} tooltip={`Remove image server ${r.name || i + 1}`} onPress={() => onChange({ image_remotes: remotes.filter((_, n) => n !== i) })}>Remove</Button>
            </div>
          ))}
          <Button size="sm" variant="secondary" isDisabled={disabled} tooltip="Add a server images can be downloaded from" onPress={() => onChange({ image_remotes: [...remotes, { name: "", server: "", protocol: "simplestreams" }] })}>Add image server</Button>
        </section>
      )}
      <section aria-label={`Images for ${kind}`} className="space-y-3">
        <div>
          <h4 className="text-base font-semibold text-fg">Images</h4>
          <p className="text-sm text-fg-muted">
            {usesServers ? "What a new guest can be made from, beyond what the host already has. The id is a server name, a colon, then the image: images:debian/13." : "What a new guest can be made from, beyond what the host already has. The id is what the host calls the image."}
          </p>
        </div>
        {images.map((m, i) => (
          <div key={i} className={`${row} sm:grid-cols-[2fr_1fr_10rem_auto]`}>
            <TextField label="Image id" mono value={m.id} isDisabled={disabled} onChange={(id) => setImage(i, { id })} />
            <TextField label="Shown as" value={m.label} isDisabled={disabled} onChange={(label) => setImage(i, { label })} />
            <Select label="Suits" options={imageKinds} value={m.kind || "any"} isDisabled={disabled} onChange={(v) => setImage(i, { kind: v === "any" ? "" : v })} />
            <Button variant="ghost" size="sm" isDisabled={disabled} tooltip={`Remove image ${m.label || m.id || i + 1}`} onPress={() => onChange({ images: images.filter((_, n) => n !== i) })}>Remove</Button>
          </div>
        ))}
        <Button size="sm" variant="secondary" isDisabled={disabled} tooltip="Add an image new guests can be made from" onPress={() => onChange({ images: [...images, { id: "", label: "" }] })}>Add image</Button>
      </section>
    </div>
  );
}

function ProviderCardForm({ kind, fieldNames, usesServers, entry, disabled, onChange }: { kind: string; fieldNames: string[]; usesServers: boolean; entry?: ProviderConfig; disabled: boolean; onChange: (p: Partial<ProviderConfig>) => void }) {
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
        <ImageSettings kind={kind} usesServers={usesServers} entry={entry} disabled={disabled} onChange={onChange} />
      </div>
    </Card>
  );
}

/** Provider panel: what the panel calls things and shows. Saved through the audited platform setting. */
export function InterfacePage() {
  const caps = useMe().data?.capabilities ?? {};
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
      {config.isError && <p role="alert" className="text-base text-danger">{config.error.message}</p>}
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
        {!canWrite && <p className="mt-3 text-base text-fg-muted">You can view this page. Changing it needs the platform settings permission.</p>}
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
                    {allItems.filter(featureOn(caps)).map((i) => (
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
                  <ProviderCardForm key={d.kind} kind={d.kind} fieldNames={d.fields.map((f) => f.name)} usesServers={!!d.image_servers} disabled={disabled}
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
