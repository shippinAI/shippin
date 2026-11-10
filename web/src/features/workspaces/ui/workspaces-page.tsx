import { FolderGit2, Plus } from "lucide-react";
import { useState } from "react";
import { Form } from "react-aria-components";
import { useNavigate } from "../../../lib/router/router";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { Card } from "../../../lib/design/components/card/card";
import { ConfirmDialog } from "../../../lib/design/components/dialog/confirm-dialog";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../lib/design/components/field/text-field";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { useTenant } from "../../../lib/hooks/use-tenant";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { PageHeader } from "../../../template/page-header";
import { type OrganizationWorkspace, useDeleteOrganizationWorkspace, useOrganizationWorkspaces, useSaveOrganizationWorkspace } from "../use-organization-workspaces";

export function OrganizationWorkspacesPage() {
  const { tenant } = useTenant();
  const tenantId = tenant?.tenant_id ?? "";
  const q = useOrganizationWorkspaces(tenantId);
  const save = useSaveOrganizationWorkspace(tenantId);
  const remove = useDeleteOrganizationWorkspace(tenantId);
  const nav = useNavigate();
  const text = useText();
  const [editor, setEditor] = useState<OrganizationWorkspace | "new" | null>(null);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [deleting, setDeleting] = useState<OrganizationWorkspace>();
  const edit = (value: OrganizationWorkspace | "new") => {
    save.reset(); setEditor(value); setName(value === "new" ? "" : value.name); setDescription(value === "new" ? "" : value.description);
  };
  return <>
    <PageHeader eyebrow={tenant?.name} title={text("spaces.title")} description={text("spaces.description")} />
    <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
      <p className="text-base text-fg-muted">{text("spaces.access")}</p>
      {tenant?.can_manage && tenant.roles.length > 0 && <Button variant="primary" tooltip={text("spaces.create")} onPress={() => edit("new")}><Plus className="size-4" />{text("spaces.create")}</Button>}
    </div>
    {q.isPending && <Skeleton className="h-40" />}
    {q.isError && <p role="alert" className="text-danger">{q.error.message}</p>}
    {remove.isError && <p role="alert" className="mb-4 text-danger">{remove.error.message}</p>}
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
      {q.data?.map((space) => <Card key={space.id} elevation={2} className="flex flex-col gap-4 p-5">
        <div className="flex items-center gap-3"><FolderGit2 className="size-5 text-primary" /><h2 className="min-w-0 flex-1 truncate font-semibold">{space.name}</h2>{space.is_default && <Badge tone="primary">{text("spaces.default")}</Badge>}</div>
        <p className="min-h-10 text-base text-fg-muted">{space.description || text("spaces.emptyDescription")}</p>
        <p className="text-base text-fg-subtle">{text("spaces.projectCount", { count: space.projects })}</p>
        <div className="mt-auto flex flex-wrap gap-2">
          <Button variant="primary" size="sm" tooltip={text("spaces.open")} onPress={() => nav(`/t/${tenantId}/workspaces/${space.id}/projects`)}>{text("spaces.open")}</Button>
          {tenant?.can_manage && tenant.roles.length > 0 && <>
            <Button size="sm" tooltip={text("spaces.edit")} onPress={() => edit(space)}>{text("spaces.edit")}</Button>
            {!space.is_default && space.projects === 0 && <Button variant="ghost" size="sm" tooltip={text("spaces.delete")} isDisabled={remove.isPending} onPress={() => setDeleting(space)}>{text("spaces.delete")}</Button>}
          </>}
        </div>
      </Card>)}
    </div>
    <ModalDialog isOpen={editor !== null} onOpenChange={(open) => { if (!open && !save.isPending) setEditor(null); }} title={text(editor === "new" ? "spaces.create" : "spaces.edit")}>
      <Form className="space-y-4 p-6" onSubmit={(e) => { e.preventDefault(); save.mutate({ id: editor && editor !== "new" ? editor.id : undefined, name, description }, { onSuccess: () => setEditor(null) }); }}>
        <TextField label={text("spaces.name")} value={name} onChange={setName} isRequired maxLength={80} autoFocus />
        <TextField label={text("spaces.details")} value={description} onChange={setDescription} maxLength={500} multiline />
        {save.isError && <p role="alert" className="text-base text-danger">{save.error.message}</p>}
        <Button type="submit" variant="primary" tooltip={text("spaces.save")} isDisabled={save.isPending || !name.trim()}>{text("spaces.save")}</Button>
      </Form>
    </ModalDialog>
    <ConfirmDialog isOpen={!!deleting} onOpenChange={(open) => { if (!open) setDeleting(undefined); }} title={text("spaces.delete")} body={text("spaces.deleteBody")} confirmLabel={text("spaces.delete")} typeToConfirm={deleting?.name} onConfirm={() => { if (deleting) remove.mutate(deleting.id); }} />
  </>;
}
