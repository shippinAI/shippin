import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Input as AriaInput, Label, Text as AriaText, TextField as AriaTextField } from "react-aria-components";
import { callAction, type ActionOutput } from "../../../../lib/api/actions";
import { ApiError } from "../../../../lib/api/client";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { Button } from "../../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../../lib/design/components/card/card";
import { DataTable, type ColumnDef } from "../../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../../lib/design/components/empty-state/empty-state";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Select } from "../../../../lib/design/components/select/select";
import { Tabs } from "../../../../lib/design/components/tabs/tabs";
import { AcquisitionPerm, CatalogPerm, useCan } from "../../../../lib/hooks/use-can";
import { useText } from "../../../../lib/ui-config/use-ui-config";
import { acquisitionCapabilities, acquisitionSourceState, leadPreviewState, persistedSourceStateUpdate, sourceDraftIsStale, validSourceOriginPort } from "./acquisition-model";
import type { AcquisitionLeadPreview, AcquisitionSourceCreateInput, AcquisitionSourceDetail, AcquisitionSourceSummary, AcquisitionSourceUpdateInput } from "../../../../lib/api/actions.gen";

const sourceListKey = ["acquisition", "sources"] as const;
const sourceDetailKey = (id: string) => ["acquisition", "source", id] as const;
const leadListKey = (id: string) => ["acquisition", "leads", id] as const;

type CreateDraft = Omit<AcquisitionSourceCreateInput, "port" | "signing_key"> & { port: string };
type UpdateDraft = Omit<AcquisitionSourceUpdateInput, "port" | "expected_config_generation" | "signing_key" | "paused"> & { port: string };

const emptyCreate: CreateDraft = { label: "", origin: "", dial_ip: "", port: "", ca_pem: "", source_tenant: "", source_project: "", issuer: "", audience: "" };
const emptyUpdate: UpdateDraft = { label: "", origin: "", dial_ip: "", port: "", ca_pem: "" };
const fieldClass = "w-full rounded-lg border border-line-control bg-canvas/60 px-3 text-base text-fg placeholder:text-fg-subtle outline-none transition-colors focus:border-primary focus:ring-2 focus:ring-primary/25";

type SourceList = ActionOutput<"shippin.tenancy.acquisition_source.list">;
type SourceDetail = AcquisitionSourceDetail;
type LeadList = ActionOutput<"shippin.tenancy.acquisition_source.lead.list">;

type Notice = { text: string; kind: "status" | "error" };

function safeFailure(error: unknown) {
  return error instanceof ApiError && error.status === 403 ? "denied" : "generic";
}

function formatTimestamp(value?: string | null) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleString();
}

function validSeed(value: string) {
  if (!value) return false;
  try {
    const decoded = atob(value);
    return decoded.length === 32 && btoa(decoded) === value;
  } catch {
    return false;
  }
}

/** Secret input stays uncontrolled, outside React state and query/mutation caches. */
function SigningSeedField({ label, description, inputRef, required = false }: {
  label: string;
  description: string;
  inputRef: React.RefObject<HTMLInputElement | null>;
  required?: boolean;
}) {
  return (
    <AriaTextField isRequired={required} className="flex flex-col gap-1.5">
      <Label className="text-sm font-medium text-fg">{label}</Label>
      <AriaInput ref={inputRef} type="password" autoComplete="new-password" spellCheck={false} className={`${fieldClass} h-9 font-mono text-xs`} />
      <AriaText slot="description" className="text-xs text-fg-subtle">{description}</AriaText>
    </AriaTextField>
  );
}

export function AcquisitionPanel() {
  const can = useCan();
  const t = useText();
  const qc = useQueryClient();
  const caps = acquisitionCapabilities([
    ...(can(CatalogPerm.customersRead) ? [CatalogPerm.customersRead] : []),
    ...(can("platform.settings.read") ? ["platform.settings.read"] : []),
    ...(can("platform.settings.write") ? ["platform.settings.write"] : []),
    ...(can(AcquisitionPerm.read) ? [AcquisitionPerm.read] : []),
    ...(can(AcquisitionPerm.sync) ? [AcquisitionPerm.sync] : []),
  ]);
  const [activeTab, setActiveTab] = useState("source");
  const [selectedID, setSelectedID] = useState("");
  const [creating, setCreating] = useState(false);
  const [createDraft, setCreateDraft] = useState<CreateDraft>(emptyCreate);
  const [updateDraft, setUpdateDraft] = useState<UpdateDraft>(emptyUpdate);
  const [loadedID, setLoadedID] = useState("");
  const [loadedGeneration, setLoadedGeneration] = useState<number | null>(null);
  const [createReason, setCreateReason] = useState("");
  const [updateReason, setUpdateReason] = useState("");
  const [stateReason, setStateReason] = useState("");
  const [syncReason, setSyncReason] = useState("");
  const [createBusy, setCreateBusy] = useState(false);
  const [updateBusy, setUpdateBusy] = useState(false);
  const [syncBusy, setSyncBusy] = useState(false);
  const [createNotice, setCreateNotice] = useState<Notice | null>(null);
  const [updateNotice, setUpdateNotice] = useState<Notice | null>(null);
  const [stateNotice, setStateNotice] = useState<Notice | null>(null);
  const [sourceNotice, setSourceNotice] = useState<Notice | null>(null);
  const [syncNotice, setSyncNotice] = useState<Notice | null>(null);
  const [stale, setStale] = useState(false);
  const createSigner = useRef<HTMLInputElement>(null);
  const updateSigner = useRef<HTMLInputElement>(null);

  const sources = useQuery<SourceList>({
    queryKey: sourceListKey,
    queryFn: () => callAction("shippin.tenancy.acquisition_source.list", {}),
    enabled: caps.sourceRead,
    retry: false,
  });
  const selected = sources.data?.items.find((source) => source.id === selectedID);
  const detail = useQuery<SourceDetail>({
    queryKey: sourceDetailKey(selectedID),
    queryFn: () => callAction("shippin.tenancy.acquisition_source.get", { acquisitionSourceId: selectedID }),
    enabled: caps.sourceConfigure && !!selectedID,
    retry: false,
  });
  const leads = useQuery<LeadList>({
    queryKey: leadListKey(selectedID),
    queryFn: () => callAction("shippin.tenancy.acquisition_source.lead.list", { acquisitionSourceId: selectedID }),
    enabled: caps.sourceRead && activeTab === "leads" && !!selectedID,
    retry: false,
  });

  useEffect(() => {
    if (!selectedID && sources.data?.items[0]) setSelectedID(sources.data.items[0].id);
  }, [selectedID, sources.data]);
  useEffect(() => {
    setLoadedID("");
    setLoadedGeneration(null);
    setUpdateDraft(emptyUpdate);
    setUpdateReason("");
    setUpdateNotice(null);
    setStale(false);
    if (updateSigner.current) updateSigner.current.value = "";
  }, [selectedID]);
  useEffect(() => {
    if (caps.sourceConfigure && selectedID && detail.data?.summary.id === selectedID && loadedID !== selectedID) {
      setUpdateDraft({ label: detail.data.summary.label, origin: detail.data.origin, dial_ip: detail.data.dial_ip, port: String(detail.data.port), ca_pem: detail.data.ca_pem });
      setLoadedID(selectedID);
      setLoadedGeneration(detail.data.summary.config_generation);
    }
  }, [caps.sourceConfigure, detail.data, loadedID, selectedID]);
  useEffect(() => {
    // Mutation invalidation can briefly expose the accepted generation before this form records it.
    // Keep the CAS guard quiet while a save/pause is in flight, then compare after loadedGeneration updates.
    if (!updateBusy && loadedID === selectedID && detail.data?.summary.config_generation !== undefined && sourceDraftIsStale(loadedGeneration, detail.data.summary.config_generation)) setStale(true);
  }, [detail.data?.summary.config_generation, loadedGeneration, loadedID, selectedID, updateBusy]);
  useEffect(() => () => {
    if (createSigner.current) createSigner.current.value = "";
    if (updateSigner.current) updateSigner.current.value = "";
  }, []);


  function renderSourceState(source: AcquisitionSourceSummary) {
    const state = acquisitionSourceState(source);
    const badge = state === "paused" ? t("signups.acquisition.paused") : state === "never-synced" ? t("signups.acquisition.neverSynced") : t("signups.acquisition.synced", { time: formatTimestamp(source.latest_success_at) });
    return (
      <div className="grid gap-3 rounded-lg border border-line bg-canvas/40 p-4 sm:grid-cols-2">
        <div className="flex items-center gap-2"><span className="text-sm text-fg-muted">{t("signups.acquisition.stateLabel")}</span><Badge tone={state === "paused" ? "warning" : state === "synced" ? "success" : "neutral"}>{badge}</Badge></div>
        <p className="text-sm text-fg-muted">{t("signups.acquisition.cursor", { cursor: source.cursor })}</p>
        <p className="text-sm text-fg-muted">{t("signups.acquisition.leadCount", { count: source.active_lead_count })}</p>
        <p className="text-sm text-fg-muted">{source.signer_configured ? t("signups.acquisition.signerConfigured") : t("signups.acquisition.signerMissing")}</p>
        <p className="text-sm text-fg-muted">{source.ca_configured ? t("signups.acquisition.caConfigured") : t("signups.acquisition.caMissing")}</p>
        <p className="text-sm text-fg-muted">{t("signups.acquisition.generation", { generation: source.config_generation })}</p>
      </div>
    );
  }

  function sourceFormFields(draft: CreateDraft | UpdateDraft, update: (key: string, value: string) => void, create: boolean) {
    return (
      <div className="grid gap-4 md:grid-cols-2">
        <TextField label={t("signups.acquisition.name")} value={draft.label} onChange={(v) => update("label", v)} isRequired />
        <TextField label={t("signups.acquisition.origin")} description={t("signups.acquisition.originHelp")} value={draft.origin} onChange={(v) => update("origin", v)} isRequired />
        <TextField label={t("signups.acquisition.destinationIP")} value={draft.dial_ip} onChange={(v) => update("dial_ip", v)} isRequired />
        <TextField label={t("signups.acquisition.port")} value={draft.port} onChange={(v) => update("port", v)} type="number" min={1} max={65535} step={1} isRequired />
        <TextField label={t("signups.acquisition.ca")} description={t("signups.acquisition.caHelp")} value={draft.ca_pem} onChange={(v) => update("ca_pem", v)} multiline isRequired />
        {create && <>
          <TextField label={t("signups.acquisition.tenant")} value={createDraft.source_tenant} onChange={(v) => update("source_tenant", v)} isRequired />
          <TextField label={t("signups.acquisition.project")} value={createDraft.source_project} onChange={(v) => update("source_project", v)} isRequired />
          <TextField label={t("signups.acquisition.issuer")} value={createDraft.issuer} onChange={(v) => update("issuer", v)} isRequired />
          <TextField label={t("signups.acquisition.audience")} value={createDraft.audience} onChange={(v) => update("audience", v)} isRequired />
          <div className="md:col-span-2"><SigningSeedField label={t("signups.acquisition.signer")} description={t("signups.acquisition.signerHelp")} inputRef={createSigner} required /></div>
        </>}
      </div>
    );
  }

  function submitError(error: unknown, fallback: string): Notice {
    return { kind: "error", text: error instanceof ApiError && error.status === 409 ? t("signups.acquisition.stale") : safeFailure(error) === "denied" ? t("signups.acquisition.denied") : fallback };
  }

  function closeCreate() {
    if (createSigner.current) createSigner.current.value = "";
    setCreateDraft(emptyCreate);
    setCreateReason("");
    setCreateNotice(null);
    setCreating(false);
  }

  async function createSource(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setCreateNotice(null);
    const port = Number(createDraft.port);
    if (!validSourceOriginPort(createDraft.origin.trim(), createDraft.port)) {
      setCreateNotice({ kind: "error", text: t(createDraft.origin.trim().startsWith("https://") ? "signups.acquisition.portMismatch" : "signups.acquisition.invalidOrigin") });
      return;
    }
    if (!createReason.trim() || createReason.trim().length < 5) {
      setCreateNotice({ kind: "error", text: t("signups.acquisition.reasonRequired") });
      return;
    }
    let signingKey = createSigner.current?.value ?? "";
    if (!validSeed(signingKey)) {
      setCreateNotice({ kind: "error", text: t("signups.acquisition.signerRequired") });
      return;
    }
    setCreateBusy(true);
    try {
      const request = callAction("shippin.tenancy.acquisition_source.create", {}, {
        label: createDraft.label.trim(), origin: createDraft.origin.trim(), dial_ip: createDraft.dial_ip.trim(), port,
        ca_pem: createDraft.ca_pem.trim(), source_tenant: createDraft.source_tenant.trim(), source_project: createDraft.source_project.trim(),
        issuer: createDraft.issuer.trim(), audience: createDraft.audience.trim(), signing_key: signingKey, reason: createReason.trim(),
      });
      if (createSigner.current) createSigner.current.value = "";
      signingKey = "";
      const created = await request;
      setSelectedID(created.id);
      setActiveTab("source");
      closeCreate();
      setSourceNotice({ kind: "status", text: t("signups.acquisition.createSuccess") });
      await qc.invalidateQueries({ queryKey: sourceListKey });
    } catch (error) {
      if (createSigner.current) createSigner.current.value = "";
      setCreateNotice(submitError(error, t("signups.acquisition.createFailed")));
    } finally {
      if (createSigner.current) createSigner.current.value = "";
      signingKey = "";
      setCreateBusy(false);
    }
  }

  async function updateSource(event: React.FormEvent<HTMLFormElement>, paused: boolean, reason: string, stateChange: boolean) {
    event.preventDefault();
    const setNotice = stateChange ? setStateNotice : setUpdateNotice;
    setNotice(null);
    if (!selectedID || !selected || !detail.data || stale || loadedGeneration === null) return;
    const currentDraft = stateChange ? { label: detail.data.summary.label, origin: detail.data.origin, dial_ip: detail.data.dial_ip, port: String(detail.data.port), ca_pem: detail.data.ca_pem } : updateDraft;
    const portText = currentDraft.port;
    const port = Number(portText);
    if (!validSourceOriginPort(currentDraft.origin.trim(), portText)) {
      setNotice({ kind: "error", text: t(currentDraft.origin.trim().startsWith("https://") ? "signups.acquisition.portMismatch" : "signups.acquisition.invalidOrigin") });
      return;
    }
    if (reason.trim().length < 5) {
      setNotice({ kind: "error", text: t("signups.acquisition.reasonRequired") });
      return;
    }
    let signingKey = stateChange ? "" : updateSigner.current?.value ?? "";
    if (signingKey && !validSeed(signingKey)) {
      setNotice({ kind: "error", text: t("signups.acquisition.signerRequired") });
      return;
    }
    setUpdateBusy(true);
    try {
      const input: AcquisitionSourceUpdateInput & { reason: string } = stateChange
        ? { ...persistedSourceStateUpdate(detail.data, paused), reason: reason.trim() }
        : { expected_config_generation: loadedGeneration, label: currentDraft.label.trim(), origin: currentDraft.origin.trim(), dial_ip: currentDraft.dial_ip.trim(), port,
          ca_pem: currentDraft.ca_pem.trim(), paused, reason: reason.trim(), ...(signingKey ? { signing_key: signingKey } : {}) };
      const request = callAction("shippin.tenancy.acquisition_source.update", { acquisitionSourceId: selectedID }, input);
      if (updateSigner.current) updateSigner.current.value = "";
      signingKey = "";
      const updated = await request;
      setStale(false);
      setNotice({ kind: "status", text: t(paused === selected.paused ? "signups.acquisition.updateSuccess" : paused ? "signups.acquisition.pauseSuccess" : "signups.acquisition.resumeSuccess") });
      if (stateChange) setStateReason(""); else setUpdateReason("");
      setSelectedID(updated.id);
      await Promise.all([
        qc.invalidateQueries({ queryKey: sourceListKey }),
        qc.invalidateQueries({ queryKey: sourceDetailKey(selectedID) }),
        qc.invalidateQueries({ queryKey: leadListKey(selectedID) }),
      ]);
      if (!stateChange) setUpdateDraft(currentDraft);
      setLoadedID(updated.id);
      setLoadedGeneration(updated.config_generation);
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) setStale(true);
      setNotice(submitError(error, t("signups.acquisition.updateFailed")));
    } finally {
      if (updateSigner.current) updateSigner.current.value = "";
      signingKey = "";
      setUpdateBusy(false);
    }
  }

  async function syncSource(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSyncNotice(null);
    if (!selectedID || !selected || selected.paused) return;
    if (syncReason.trim().length < 5) {
      setSyncNotice({ kind: "error", text: t("signups.acquisition.reasonRequired") });
      return;
    }
    setSyncBusy(true);
    try {
      const result = await callAction("shippin.tenancy.acquisition_source.sync", { acquisitionSourceId: selectedID }, { reason: syncReason.trim() });
      const text = t("signups.acquisition.syncSuccess", { count: result.applied, cursor: result.cursor }) + (result.more ? ` ${t("signups.acquisition.more")}` : "");
      setSyncNotice({ kind: "status", text });
      setSyncReason("");
      await Promise.all([
        qc.invalidateQueries({ queryKey: sourceListKey }),
        qc.invalidateQueries({ queryKey: sourceDetailKey(selectedID) }),
        qc.invalidateQueries({ queryKey: leadListKey(selectedID) }),
      ]);
    } catch (error) {
      setSyncNotice(submitError(error, t("signups.acquisition.syncFailed")));
    } finally {
      setSyncBusy(false);
    }
  }

  async function reloadAfterConflict() {
    const result = await detail.refetch();
    if (result.data) {
      setUpdateDraft({ label: result.data.summary.label, origin: result.data.origin, dial_ip: result.data.dial_ip, port: String(result.data.port), ca_pem: result.data.ca_pem });
      setLoadedID(selectedID);
      setLoadedGeneration(result.data.summary.config_generation);
      setUpdateReason("");
      setStale(false);
      setUpdateNotice(null);
    }
  }

  function renderSourceTab() {
    if (!caps.sourceRead) return <p role="alert" className="p-5 text-sm text-warning">{t("signups.acquisition.noRead")}</p>;
    if (sources.isPending) return <p role="status" className="p-5 text-sm text-fg-muted">{t("signups.acquisition.loadingSources")}</p>;
    if (sources.isError) return <p role="alert" className="p-5 text-sm text-danger">{sources.error instanceof ApiError && sources.error.status === 403 ? t("signups.acquisition.denied") : t("signups.acquisition.sourceError")}</p>;
    if (!sources.data?.items.length) {
      return caps.sourceConfigure ? renderCreateSourceForm() : <EmptyState title={t("signups.acquisition.noSource.title")} description={t("signups.acquisition.noSource.body")} />;
    }
    return (
      <div className="space-y-5 p-4 md:p-5">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div className="min-w-0 flex-1">
            <Select label={t("signups.acquisition.sourceSelect")} value={selectedID} onChange={setSelectedID} options={sources.data.items.map((source) => ({ id: source.id, label: `${source.label} · ${source.id.slice(0, 8)}` }))} />
          </div>
          {caps.sourceConfigure && <Button tooltip={t("signups.acquisition.addSource")} onPress={() => { setCreateDraft(emptyCreate); setCreating(true); setCreateNotice(null); }}>{t("signups.acquisition.addSource")}</Button>}
        </div>
        {sourceNotice && <p role={sourceNotice.kind === "error" ? "alert" : "status"} className={`text-sm ${sourceNotice.kind === "error" ? "text-danger" : "text-success"}`}>{sourceNotice.text}</p>}
        {creating && renderCreateSourceForm()}
        {selected && <>
          <div className="space-y-3">
            <h4 className="text-sm font-semibold text-fg">{selected.label}</h4>
            {renderSourceState(selected)}
            <p className="text-sm text-fg-muted">{t("signups.acquisition.sourceConfigured")}</p>
            <p role="note" className="rounded-lg border border-warning/30 bg-warning/10 p-3 text-sm text-fg-muted">{t("signups.acquisition.restoreWarning")}</p>
          </div>
          {!caps.sourceConfigure ? <p className="text-sm text-fg-subtle">{t("signups.acquisition.configureNeedsRead")}</p> : detail.isPending ? <p role="status" className="text-sm text-fg-muted">{t("signups.acquisition.loadingSources")}</p> : detail.isError ? <p role="alert" className="text-sm text-danger">{detail.error instanceof ApiError && detail.error.status === 403 ? t("signups.acquisition.denied") : t("signups.acquisition.sourceError")}</p> : detail.data && <>
            <dl className="grid gap-2 rounded-lg border border-line p-4 text-sm sm:grid-cols-2">
              <ReadonlyValue label={t("signups.acquisition.tenant")} value={detail.data.source_tenant} />
              <ReadonlyValue label={t("signups.acquisition.project")} value={detail.data.source_project} />
              <ReadonlyValue label={t("signups.acquisition.issuer")} value={detail.data.issuer} />
              <ReadonlyValue label={t("signups.acquisition.audience")} value={detail.data.audience} />
            </dl>
            <p className="text-xs text-fg-subtle">{t("signups.acquisition.immutable")}</p>
            <form aria-label={t("signups.acquisition.editTitle")} onSubmit={(event) => void updateSource(event, selected.paused, updateReason, false)} className="space-y-4 rounded-lg border border-line bg-canvas/30 p-4">
              <div className="flex flex-wrap items-center justify-between gap-2"><h4 className="font-semibold text-fg">{t("signups.acquisition.editTitle")}</h4><Button type="button" variant="ghost" tooltip={t("signups.acquisition.edit")} onPress={() => { setLoadedID(""); void detail.refetch(); }}>{t("signups.acquisition.edit")}</Button></div>
              {sourceFormFields(updateDraft, (key, value) => setUpdateDraft((draft) => ({ ...draft, [key]: value })), false)}
              <SigningSeedField label={t("signups.acquisition.signerOptional")} description={t("signups.acquisition.signerHelp")} inputRef={updateSigner} />
              <TextField label={t("signups.acquisition.reason")} value={updateReason} onChange={setUpdateReason} isRequired />
              {stale && <div role="alert" className="flex flex-wrap items-center gap-3 text-sm text-warning"><span>{t("signups.acquisition.stale")}</span><Button type="button" variant="ghost" tooltip={t("signups.acquisition.refresh")} onPress={() => void reloadAfterConflict()}>{t("signups.acquisition.refresh")}</Button></div>}
              {updateNotice && <p role={updateNotice.kind === "error" ? "alert" : "status"} className={`text-sm ${updateNotice.kind === "error" ? "text-danger" : "text-success"}`}>{updateNotice.text}</p>}
              <Button type="submit" variant="primary" tooltip={t("signups.acquisition.save")} isDisabled={stale || updateBusy || updateReason.trim().length < 5 || !validSourceOriginPort(updateDraft.origin.trim(), updateDraft.port)}>{t("signups.acquisition.save")}</Button>
            </form>
            <form aria-label={t(selected.paused ? "signups.acquisition.resume" : "signups.acquisition.pause")} onSubmit={(event) => void updateSource(event, !selected.paused, stateReason, true)} className="flex flex-wrap items-end gap-3 rounded-lg border border-line p-4">
              <div className="min-w-0 flex-1"><TextField label={t("signups.acquisition.reason")} value={stateReason} onChange={setStateReason} isRequired /></div>
              {stateNotice && <p role={stateNotice.kind === "error" ? "alert" : "status"} className={`w-full text-sm ${stateNotice.kind === "error" ? "text-danger" : "text-success"}`}>{stateNotice.text}</p>}
              <Button type="submit" variant={selected.paused ? "primary" : "secondary"} tooltip={t(selected.paused ? "signups.acquisition.resume" : "signups.acquisition.pause")} isDisabled={!caps.sourceConfigure || updateBusy || stateReason.trim().length < 5}>{t(selected.paused ? "signups.acquisition.resume" : "signups.acquisition.pause")}</Button>
            </form>
          </>}
          {caps.sourceSync && <form aria-label={t("signups.acquisition.pull")} onSubmit={(event) => void syncSource(event)} className="flex flex-wrap items-end gap-3 rounded-lg border border-line p-4">
            <div className="min-w-0 flex-1"><TextField label={t("signups.acquisition.reason")} value={syncReason} onChange={setSyncReason} isRequired /></div>
            {syncNotice && <p role={syncNotice.kind === "error" ? "alert" : "status"} className={`w-full text-sm ${syncNotice.kind === "error" ? "text-danger" : "text-success"}`}>{syncNotice.text}</p>}
            {selected.paused && <p className="w-full text-sm text-warning">{t("signups.acquisition.paused")}</p>}
            <Button type="submit" variant="primary" tooltip={t("signups.acquisition.pull")} isDisabled={selected.paused || syncBusy || syncReason.trim().length < 5}>{t("signups.acquisition.pull")}</Button>
          </form>}
          {!caps.sourceSync && <p className="text-sm text-fg-subtle">{t("signups.acquisition.noSync")}</p>}
        </>}
      </div>
    );
  }

  function renderCreateSourceForm() {
    return (
      <form aria-label={t("signups.acquisition.createTitle")} onSubmit={(event) => void createSource(event)} className="space-y-4 rounded-lg border border-line bg-canvas/30 p-4">
        <div className="flex items-center justify-between gap-3"><h4 className="font-semibold text-fg">{t("signups.acquisition.createTitle")}</h4>{sources.data?.items.length ? <Button type="button" variant="ghost" tooltip={t("signups.acquisition.cancel")} onPress={closeCreate}>{t("signups.acquisition.cancel")}</Button> : null}</div>
        {sourceFormFields(createDraft, (key, value) => setCreateDraft((draft) => ({ ...draft, [key]: value })), true)}
        <TextField label={t("signups.acquisition.reason")} value={createReason} onChange={setCreateReason} isRequired />
        {createNotice && <p role={createNotice.kind === "error" ? "alert" : "status"} className={`text-sm ${createNotice.kind === "error" ? "text-danger" : "text-success"}`}>{createNotice.text}</p>}
        <Button type="submit" variant="primary" tooltip={t("signups.acquisition.create")} isDisabled={createBusy || createReason.trim().length < 5 || !validSourceOriginPort(createDraft.origin.trim(), createDraft.port)}>{t("signups.acquisition.create")}</Button>
      </form>
    );
  }

  function renderLeadsTab() {
    if (!caps.sourceRead) return <p role="alert" className="p-5 text-sm text-warning">{t("signups.acquisition.leadsDenied")}</p>;
    if (!sources.data?.items.length && !sources.isPending) return <EmptyState title={t("signups.acquisition.noSource.title")} description={t("signups.acquisition.noSource.body")} />;
    if (!selectedID) return <p role="status" className="p-5 text-sm text-fg-muted">{t("signups.acquisition.noSelectedSource")}</p>;
    if (leads.isPending) return <p role="status" className="p-5 text-sm text-fg-muted">{t("signups.acquisition.loadingLeads")}</p>;
    if (leads.isError) return <p role="alert" className="p-5 text-sm text-danger">{leads.error instanceof ApiError && leads.error.status === 403 ? t("signups.acquisition.leadsDenied") : t("signups.acquisition.leadsError")}</p>;
    const result = leads.data;
    if (!result) return <p role="status" className="p-5 text-sm text-fg-muted">{t("signups.acquisition.loadingLeads")}</p>;
    const state = leadPreviewState(result.total, result.items.length, result.truncated);
    const columns: ColumnDef<AcquisitionLeadPreview>[] = [
      { id: "email", label: t("signups.acquisition.email"), isRowHeader: true, render: (lead) => <span className="break-all text-fg">{lead.email}</span> },
      { id: "verification", label: t("signups.acquisition.emailState"), render: (lead) => <Badge tone={lead.email_verified ? "success" : "warning"}>{t(lead.email_verified ? "signups.acquisition.verified" : "signups.acquisition.unverified")}</Badge> },
      { id: "status", label: t("signups.acquisition.leadStatus"), render: (lead) => <span>{lead.status}</span> },
      { id: "consent", label: t("signups.acquisition.consent"), render: (lead) => <span>{lead.consent_version}</span> },
      { id: "consent-at", label: t("signups.acquisition.consentRecorded"), render: (lead) => formatTimestamp(lead.consent_at) },
      { id: "received", label: t("signups.acquisition.received"), render: (lead) => formatTimestamp(lead.created_at) },
      { id: "updated", label: t("signups.acquisition.updated"), render: (lead) => formatTimestamp(lead.updated_at) },
      { id: "expires", label: t("signups.acquisition.expires"), render: (lead) => formatTimestamp(lead.expires_at) },
    ];
    return (
      <div className="space-y-4 p-4 md:p-5">
        <p role="note" className="rounded-lg border border-warning/30 bg-warning/10 p-3 text-sm text-fg-muted">{t("signups.acquisition.addressWarning")}</p>
        {selected?.paused && <p role="status" className="rounded-lg border border-line bg-canvas/40 p-3 text-sm text-fg-muted">{t("signups.acquisition.previewPaused")}</p>}
        <p role="status" className="text-sm text-fg-muted">{state === "latest" ? t("signups.acquisition.previewLatest", { count: result.items.length, total: result.total }) : t("signups.acquisition.previewComplete", { count: result.items.length })}</p>
        <DataTable label={t("signups.acquisition.tab.leads")} columns={columns} rows={result.items} dependencies={[result.total, result.truncated]} empty={<EmptyState title={t("signups.acquisition.noLeads.title")} description={t("signups.acquisition.noLeads.body")} />} />
      </div>
    );
  }

  return (
    <Card className="mb-6">
      <CardHeader title={t("signups.acquisition.title")} description={t("signups.acquisition.description")} />
      <Tabs tabs={[
        { id: "source", label: t("signups.acquisition.tab.source"), content: renderSourceTab() },
        { id: "leads", label: t("signups.acquisition.tab.leads"), content: renderLeadsTab() },
      ]} label={t("signups.acquisition.title")} selected={activeTab} onSelect={setActiveTab} />
    </Card>
  );
}

function ReadonlyValue({ label, value }: { label: string; value: string }) {
  return <div className="min-w-0"><dt className="text-xs text-fg-subtle">{label}</dt><dd className="break-all text-sm text-fg">{value}</dd></div>;
}
