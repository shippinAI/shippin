import { useEffect, useState } from "react";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { Button } from "../../../../lib/design/components/button/button";
import { useSetPlatformSetting, type PlatformSetting } from "../../../../lib/hooks/use-platform-settings";

type ProbeDefinition = { name: string; kind: string; instance?: string; url?: string; tcp?: string; host?: string; timeout_seconds?: number };
const emptyProbe: ProbeDefinition = { name: "", kind: "", instance: "", url: "", tcp: "", host: "", timeout_seconds: 3 };

/** Edits the installation's real health targets through audited platform settings. */
export function PlatformProbeSettings({ setting, canWrite }: { setting: PlatformSetting; canWrite: boolean }) {
  const save = useSetPlatformSetting();
  const [probes, setProbes] = useState<ProbeDefinition[]>(Array.isArray(setting.value) ? setting.value as ProbeDefinition[] : []);
  const [draft, setDraft] = useState<ProbeDefinition | null>(null);
  // Held apart from the address: a TCP target with nothing typed yet is still a TCP target.
  const [target, setTarget] = useState<"http" | "tcp">("http");
  const [editIndex, setEditIndex] = useState<number | null>(null);
  const [reason, setReason] = useState("");
  useEffect(() => { setProbes(Array.isArray(setting.value) ? setting.value as ProbeDefinition[] : []); }, [setting.value]);
  const update = (field: keyof ProbeDefinition, value: string) => setDraft((current) => current && ({ ...current, [field]: value }));
  const saveAll = () => save.mutate({ key: setting.key, value: probes, reason }, { onSuccess: () => setReason("") });
  return (
    <li className="space-y-4 px-4 py-4">
      <div className="flex items-start justify-between gap-3">
        <div><p className="font-medium text-fg">{setting.label}</p><p className="text-sm text-fg-muted">{setting.help}</p></div>
        <Badge tone={setting.overridden ? "warning" : "neutral"}>{setting.overridden ? "Overridden" : "From config"}</Badge>
      </div>
      <ul className="space-y-2" aria-label="Configured service probes">
        {probes.map((probe, index) => (
          <li key={`${probe.name}-${index}`} className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-line px-3 py-2">
            <div className="min-w-0"><p className="font-medium text-fg">{probe.name}</p><p className="truncate font-mono text-xs text-fg-subtle">{probe.url || probe.tcp || "Target missing"}</p></div>
            {canWrite && <div className="flex gap-2"><Button size="sm" variant="ghost" tooltip="Edit this health target" onPress={() => { setEditIndex(index); setTarget(probe.tcp ? "tcp" : "http"); setDraft({ ...probe }); }}>Edit</Button><Button size="sm" variant="ghost" tooltip="Remove this health target from the proposed configuration" onPress={() => setProbes((current) => current.filter((_, i) => i !== index))}>Remove</Button></div>}
          </li>
        ))}
      </ul>
      {canWrite && (
        <>
          {draft ? (
            <div className="grid gap-3 rounded-lg border border-line p-3 sm:grid-cols-2">
              <label className="text-sm">Display name<input className="mt-1 w-full rounded-md border border-line bg-surface px-3 py-2" value={draft.name} onChange={(e) => update("name", e.target.value)} /></label>
              <label className="text-sm">Service kind<input className="mt-1 w-full rounded-md border border-line bg-surface px-3 py-2" value={draft.kind} placeholder="identity, secrets, mail, dns, proxy, database or your own" onChange={(e) => update("kind", e.target.value)} /></label>
              <label className="text-sm">Health target<select className="mt-1 w-full rounded-md border border-line bg-surface px-3 py-2" value={target} onChange={(e) => { const next = e.target.value as "http" | "tcp"; setTarget(next); setDraft({ ...draft, url: next === "http" ? draft.url : "", tcp: next === "tcp" ? draft.tcp : "" }); }}><option value="http">HTTP / HTTPS</option><option value="tcp">TCP</option></select></label>
              {target === "tcp" ? <label className="text-sm">TCP host:port<input className="mt-1 w-full rounded-md border border-line bg-surface px-3 py-2" placeholder="host:port" value={draft.tcp} onChange={(e) => update("tcp", e.target.value)} /></label> : <label className="text-sm">Health URL<input className="mt-1 w-full rounded-md border border-line bg-surface px-3 py-2" placeholder="https://service.example/health" value={draft.url} onChange={(e) => update("url", e.target.value)} /></label>}
              <label className="text-sm">Incus instance name (optional)<input className="mt-1 w-full rounded-md border border-line bg-surface px-3 py-2" value={draft.instance ?? ""} onChange={(e) => update("instance", e.target.value)} /></label>
              <label className="text-sm">HTTP Host header (optional)<input className="mt-1 w-full rounded-md border border-line bg-surface px-3 py-2" value={draft.host ?? ""} onChange={(e) => update("host", e.target.value)} /></label>
              <label className="text-sm">Probe timeout (seconds)<input className="mt-1 w-full rounded-md border border-line bg-surface px-3 py-2" type="number" min={1} max={10} value={draft.timeout_seconds ?? 3} onChange={(e) => setDraft((current) => current && ({ ...current, timeout_seconds: Number(e.target.value) }))} /></label>
              <div className="flex gap-2 sm:col-span-2"><Button size="sm" variant="primary" tooltip="Apply this edit to the proposed configuration" onPress={() => { if (!draft.name.trim() || !draft.kind.trim() || !(draft.url || draft.tcp) || !Number.isInteger(draft.timeout_seconds) || draft.timeout_seconds! < 1 || draft.timeout_seconds! > 10) return; setProbes(editIndex === null ? [...probes, draft] : probes.map((p, i) => i === editIndex ? draft : p)); setDraft(null); setEditIndex(null); }}>Apply to list</Button><Button size="sm" variant="ghost" tooltip="Discard this edit" onPress={() => { setDraft(null); setEditIndex(null); }}>Cancel</Button></div>
            </div>
          ) : <Button size="sm" variant="ghost" tooltip="Add an HTTP or TCP health target" onPress={() => { setEditIndex(null); setTarget("http"); setDraft({ ...emptyProbe }); }}>Add service probe</Button>}
          <div className="flex flex-wrap items-end gap-2"><label className="min-w-64 flex-1 text-sm">Reason (audited)<input className="mt-1 w-full rounded-md border border-line bg-surface px-3 py-2" value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Why are these targets changing?" /></label>
            <Button size="sm" variant="primary" tooltip="Save all probe changes with an audit reason" isDisabled={!reason.trim() || reason.trim().length < 5 || save.isPending} onPress={saveAll}>Save probe configuration</Button>
            {setting.overridden && <Button size="sm" variant="ghost" tooltip="Restore probe definitions from installation config" isDisabled={reason.trim().length < 5 || save.isPending} onPress={() => save.mutate({ key: setting.key, value: null, reason }, { onSuccess: () => { setReason(""); setDraft(null); } })}>Reset to config</Button>}
          </div>
        </>
      )}
    </li>
  );
}
