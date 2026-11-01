import { Plus } from "lucide-react";
import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { TextField } from "../../../lib/design/components/field/text-field";
import { Select } from "../../../lib/design/components/select/select";
import { useCreateAgent } from "../../../lib/hooks/use-agents";
import { useWorkspaces } from "../../../lib/hooks/use-workspaces";

const none = "__none__";

/** Owner/admin form: name + optional workspace. The API re-checks the role, the cap and the name. */
export function CreateAgentForm({ tenantId, canCreate, reason }: { tenantId: string; canCreate: boolean; reason: string }) {
  const create = useCreateAgent(tenantId);
  const ws = useWorkspaces(tenantId).data?.items ?? [];
  const [name, setName] = useState("");
  const [workspace, setWorkspace] = useState(none);
  const n = name.trim();
  const valid = n.length >= 1 && n.length <= 120;
  const options = [{ id: none, label: "Any workspace" }, ...ws.map((w) => ({ id: w.id, label: w.name }))];
  const submit = () =>
    create.mutate({ name: n, ...(workspace !== none ? { workspace_id: workspace } : {}) }, { onSuccess: () => setName("") });
  return (
    <Card>
      <CardHeader title="New agent" description="Delegated by you. It can never do more than you can." />
      <div className="space-y-4 p-5">
        <TextField label="Name" value={name} onChange={setName} placeholder="build-bot" />
        <Select label="Workspace" options={options} value={workspace} onChange={setWorkspace} />
        <div className="flex justify-end">
          <Button
            variant="primary"
            size="sm"
            isDisabled={!canCreate || !valid || create.isPending}
            tooltip={!canCreate ? reason : valid ? "Create the service user in your org" : "Enter a name (1–120 characters)"}
            onPress={submit}
          >
            <Plus className="size-4" /> Create agent
          </Button>
        </div>
      </div>
    </Card>
  );
}
