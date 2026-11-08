import { useState } from "react";
import { Button } from "../../../../lib/design/components/button/button";
import { Drawer } from "../../../../lib/design/components/drawer/drawer";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Select } from "../../../../lib/design/components/select/select";
import { Switch } from "../../../../lib/design/components/switch/switch";
import type { Template } from "../../../../lib/api/types";
import { useSaveTemplate } from "../../../../lib/hooks/use-templates";
import { iconKeys } from "../../../devspaces/ui/create/template-icon";

const blank: Partial<Template> = { slug: "", name: "", description: "", icon: "debian", image: "", toolchain: [], cpu: 2, memory_gib: 4, disk_gib: 20, repo: "", startup: "", ports: [], enabled: true, sort: 100 };
const list = (s: string) => s.split(",").map((x) => x.trim()).filter(Boolean);

/** Create/edit a template in a side sheet. The API validates everything again. */
export function TemplateForm({ initial, onClose, prefix }: { initial?: Partial<Template>; onClose: () => void; prefix: string }) {
  const [t, setT] = useState<Partial<Template>>(initial ?? { ...blank, image: prefix });
  const [tool, setTool] = useState((initial?.toolchain ?? []).join(", "));
  const [ports, setPorts] = useState((initial?.ports ?? []).join(", "));
  const save = useSaveTemplate();
  const f = <K extends keyof Template>(k: K) => (v: Template[K]) => setT({ ...t, [k]: v });
  const n = (k: "cpu" | "memory_gib" | "disk_gib" | "sort") => ({ value: String(t[k] ?? ""), onChange: (v: string) => f(k)(Number(v) || 0) });
  const submit = () =>
    save.mutate({ ...t, toolchain: list(tool), ports: list(ports).map(Number) }, { onSuccess: onClose });
  return (
    <Drawer
      isOpen
      onOpenChange={(o) => !o && onClose()}
      title={t.id ? `Edit ${t.name}` : "New template"}
      subtitle={`Image must be a ${prefix} alias published on the host.`}
      footer={
        <>
          <Switch isSelected={!!t.enabled} onChange={f("enabled")} tooltip="Disabled templates are hidden from the gallery">Shown in gallery</Switch>
          <div className="flex-1" />
          <Button variant="ghost" tooltip="Discard changes" onPress={onClose}>Cancel</Button>
          <Button variant="primary" isDisabled={save.isPending} tooltip="Validate and save" onPress={submit}>{save.isPending ? "Saving…" : "Save template"}</Button>
        </>
      }
    >
      <div className="grid grid-cols-2 gap-4 px-6 py-5">
        <TextField label="Name" value={t.name ?? ""} onChange={f("name")} isRequired />
        <TextField label="Slug" value={t.slug ?? ""} onChange={f("slug")} mono isRequired />
        <div className="col-span-2"><TextField label="Description" value={t.description ?? ""} onChange={f("description")} multiline /></div>
        <Select label="Icon" options={iconKeys.map((k) => ({ id: k, label: k }))} value={t.icon ?? "debian"} onChange={f("icon")} />
        <TextField label="Base image" value={t.image ?? ""} onChange={f("image")} mono isRequired />
        <div className="col-span-2"><TextField label="Toolchain (comma-separated)" value={tool} onChange={setTool} /></div>
        <TextField label="vCPU" {...n("cpu")} />
        <TextField label="RAM (GiB)" {...n("memory_gib")} />
        <TextField label="Disk (GiB)" {...n("disk_gib")} />
        <TextField label="Gallery order" {...n("sort")} />
        <div className="col-span-2"><TextField label="Default git repo (optional)" value={t.repo ?? ""} onChange={f("repo")} mono placeholder="https://github.com/owner/repo" /></div>
        <div className="col-span-2"><TextField label="Exposed ports (comma-separated)" value={ports} onChange={setPorts} mono description="Become private preview URLs on create." /></div>
        <div className="col-span-2"><TextField label="Startup script (bash, runs as dev in the project dir)" value={t.startup ?? ""} onChange={f("startup")} multiline mono /></div>
      </div>
    </Drawer>
  );
}
