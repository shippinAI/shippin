import { Select } from "../../../../lib/design/components/select/select";

export type Target = { tenantId: string; tenantName: string; workspaces: { id: string; name: string }[]; workspaceId?: string };

const WHOLE = "tenant";

/** Scope picker: the whole tenant or one of its workspaces. */
export function ScopeSelect({ target, value, onChange }: { target: Target; value: string; onChange: (ws: string) => void }) {
  const options = [{ id: WHOLE, label: `Whole tenant (${target.tenantName})` }, ...target.workspaces.map((w) => ({ id: w.id, label: `Workspace ${w.name}` }))];
  return <Select label="Scope" options={options} value={value || WHOLE} onChange={(v) => onChange(v === WHOLE ? "" : v)} />;
}
