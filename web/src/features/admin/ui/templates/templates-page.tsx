import { Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { Button } from "../../../../lib/design/components/button/button";
import { IconButton } from "../../../../lib/design/components/button/icon-button";
import { Card } from "../../../../lib/design/components/card/card";
import { DataTable, type ColumnDef } from "../../../../lib/design/components/data-table/data-table";
import { ConfirmDialog } from "../../../../lib/design/components/dialog/confirm-dialog";
import { EmptyState } from "../../../../lib/design/components/empty-state/empty-state";
import type { Template } from "../../../../lib/api/types";
import { useAdminTemplates, useDeleteTemplate } from "../../../../lib/hooks/use-templates";
import { PageHeader } from "../../../../template/page-header";
import { TemplateIcon } from "../../../devspaces/ui/create/template-icon";
import { QueryView } from "../query-view";
import { TemplateForm } from "./template-form";

export function TemplatesPage() {
  const q = useAdminTemplates();
  const del = useDeleteTemplate();
  const [edit, setEdit] = useState<Partial<Template> | null>(null);
  const [gone, setGone] = useState<Template>();
  const cols: ColumnDef<Template>[] = [
    { id: "name", label: "Template", isRowHeader: true, render: (t) => (
      <div className="flex items-center gap-3"><TemplateIcon icon={t.icon} size={30} />
        <div><div className="font-medium text-fg">{t.name}</div><div className="font-mono text-xs text-fg-subtle">{t.slug}</div></div></div>
    ) },
    { id: "image", label: "Image", render: (t) => <span className="font-mono text-xs">{t.image}</span> },
    { id: "tool", label: "Toolchain", render: (t) => <span className="text-xs">{t.toolchain.join(", ")}</span> },
    { id: "size", label: "Default size", width: "150px", render: (t) => `${t.cpu} vCPU · ${t.memory_gib}/${t.disk_gib} GiB` },
    { id: "ports", label: "Ports", width: "90px", render: (t) => <span className="font-mono text-xs">{t.ports.join(", ") || "—"}</span> },
    { id: "on", label: "Gallery", width: "100px", render: (t) => <Badge tone={t.enabled ? "success" : "neutral"}>{t.enabled ? "Shown" : "Hidden"}</Badge> },
    { id: "act", label: "", width: "84px", render: (t) => (
      <div className="flex justify-end gap-0.5">
        <IconButton icon={Pencil} label={`Edit ${t.name}`} onPress={() => setEdit(t)} />
        <IconButton icon={Trash2} label={`Delete ${t.name}`} onPress={() => setGone(t)} />
      </div>
    ) },
  ];
  return (
    <>
      <PageHeader eyebrow="Provider" title="Templates" description="Blueprints customers pick when creating a workspace: image, toolchain, size, repo, startup script and preview ports."
        actions={<Button variant="primary" size="sm" tooltip="Add a template to the gallery" onPress={() => setEdit({})}><Plus className="size-4" /> New template</Button>} />
      <Card className="overflow-hidden">
        <QueryView isPending={q.isPending} error={q.error} onRetry={() => void q.refetch()}>
          {() => <DataTable label="Templates" dense columns={cols} rows={q.data?.items ?? []} onRowAction={(id) => setEdit(q.data?.items.find((t) => t.id === id) ?? null)}
            empty={<EmptyState title="No templates" description="Create one, or restart the console to seed the defaults." />} />}
        </QueryView>
      </Card>
      {edit && <TemplateForm initial={edit.id ? edit : undefined} onClose={() => setEdit(null)} prefix={q.data?.limits.image_prefix ?? ""} />}
      <ConfirmDialog isOpen={!!gone} onOpenChange={(o) => !o && setGone(undefined)} title={`Delete ${gone?.name}?`}
        body="It disappears from the gallery. Workspaces created from it keep running (their template link is cleared)." confirmLabel="Delete template"
        typeToConfirm={gone?.slug} onConfirm={() => gone && del.mutate(gone)} />
    </>
  );
}
