import { useState } from "react";
import type { CatalogService } from "../../../../lib/api/catalog-types";
import { Button } from "../../../../lib/design/components/button/button";
import { Drawer } from "../../../../lib/design/components/drawer/drawer";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Switch } from "../../../../lib/design/components/switch/switch";
import { catalogPaths, useCatalogWrite } from "../../../../lib/hooks/use-catalog";

const obj = (s: string) => { try { const v = JSON.parse(s); return v && typeof v === "object" && !Array.isArray(v) ? v : null; } catch { return null; } };

/** Create or edit a catalog service, including its JSON config schema and defaults. */
export function ServiceForm({ initial, onClose }: { initial?: CatalogService; onClose: () => void }) {
  const [s, setS] = useState({ id: initial?.id ?? "", label: initial?.label ?? "", description: initial?.description ?? "", kind: initial?.kind ?? "app",
    docs_url: initial?.docs_url ?? "", required: initial?.required ?? false, active: initial?.active ?? true });
  const [schema, setSchema] = useState(JSON.stringify(initial?.config_schema ?? { type: "object", properties: {} }, null, 2));
  const [defaults, setDefaults] = useState(JSON.stringify(initial?.default_config ?? {}, null, 2));
  const save = useCatalogWrite(initial ? "Service updated" : "Service added");
  const del = useCatalogWrite("Service removed");
  const ok = obj(schema) && obj(defaults) && s.id && s.label;
  const set = (k: keyof typeof s) => (v: string | boolean) => setS({ ...s, [k]: v });
  return (
    <Drawer isOpen onOpenChange={(o) => !o && onClose()} title={initial ? `Edit ${initial.label}` : "New service"}
      subtitle="Defaults are the system layer; tiers and customers change them within each lock mode."
      footer={<>
        {initial && <Button tooltip="Delete" variant="danger" onPress={() => del.mutate({ method: "DELETE", path: catalogPaths.service(s.id) }, { onSuccess: onClose })}>Delete</Button>}
        <div className="flex-1" />
        <Button tooltip="Cancel" variant="ghost" onPress={onClose}>Cancel</Button>
        <Button tooltip="Save service" variant="primary" isDisabled={!ok || save.isPending}
          onPress={() => save.mutate({ method: initial ? "PUT" : "POST", path: catalogPaths.service(initial?.id),
            body: { ...s, config_schema: obj(schema), default_config: obj(defaults) } }, { onSuccess: onClose })}>Save service</Button>
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
        <div className="flex gap-6">
          <Switch isSelected={s.required} onChange={set("required")} tooltip="Required services are locked on for every plan">Required</Switch>
          <Switch isSelected={s.active} onChange={set("active")} tooltip="Inactive services are left out of effective config">Active</Switch>
        </div>
      </div>
    </Drawer>
  );
}
