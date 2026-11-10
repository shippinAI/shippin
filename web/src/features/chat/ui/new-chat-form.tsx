import { useEffect, useState } from "react";
import { useQueries } from "@tanstack/react-query";
import { apiGet } from "../../../lib/api/client";
import type { Workspace } from "../../../lib/api/types";
import { useMe } from "../../../lib/hooks/use-api";
import { useChatConfig, useCreateChat } from "../../../lib/chat/use-chat";
import { Button } from "../../../lib/design/components/button/button";
import { Select } from "../../../lib/design/components/select/select";

/** Start a chat: pick the workspace whose AI CLI answers and which of your subscriptions to use. */
export function NewChatForm({ onCreated }: { onCreated: (id: string) => void }) {
  const tenants = useMe().data?.tenants ?? [];
  const lists = useQueries({
    queries: tenants.map((t) => ({ queryKey: ["workspaces", t.tenant_id], queryFn: () => apiGet<{ items: Workspace[] }>(`/v1/tenants/${t.tenant_id}/workspaces`) })),
  });
  const workspaces = lists.flatMap((l) => l.data?.items ?? []).filter((w) => w.state === "running");
  const providers = useChatConfig().data?.providers ?? [];
  const [ws, setWs] = useState("");
  const [prov, setProv] = useState("");
  useEffect(() => { if (!ws && workspaces[0]) setWs(workspaces[0].id); }, [ws, workspaces]);
  useEffect(() => { if (!prov && providers[0]) setProv(providers[0].id); }, [prov, providers]);
  const create = useCreateChat();
  return (
    <div className="space-y-3 p-4">
      <p className="text-base text-fg-muted">Chat runs your own AI CLI inside your workspace, signed in with your own subscription.</p>
      <Select label="Workspace" value={ws} onChange={setWs} options={workspaces.map((w) => ({ id: w.id, label: w.name }))} />
      <Select label="AI" value={prov} onChange={setProv} options={providers.map((p) => ({ id: p.id, label: p.name }))} />
      {workspaces.length === 0 && <p className="text-base text-fg-muted">Start a workspace first; chat answers from inside it.</p>}
      {create.error && <p role="alert" className="text-base text-danger">{create.error.message}</p>}
      <Button variant="primary" tooltip="Start a new conversation" isDisabled={!ws || !prov || create.isPending}
        onPress={() => create.mutate({ workspace_id: ws, provider: prov }, { onSuccess: (s) => onCreated(s.id) })}>
        Start chat
      </Button>
    </div>
  );
}
