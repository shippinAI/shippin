import { useQuery } from "@tanstack/react-query";
import { apiGet } from "../../../lib/api/client";
import { useLocation } from "../../../lib/router/router";
import type { A2UIBlock } from "../../../lib/chat/chat-types";
import { A2UIRenderer } from "../a2ui/a2ui-renderer";

/** Passive knowledge: tip panels for the page the user is on (server catalog, no AI call). */
export function TipsPanel() {
  const { pathname } = useLocation();
  const q = useQuery({
    queryKey: ["chat", "tips", pathname],
    queryFn: () => apiGet<{ items: A2UIBlock[] }>(`/v1/chat/tips?route=${encodeURIComponent(pathname)}`),
    staleTime: 300_000,
  });
  const items = q.data?.items ?? [];
  if (!items.length) return null;
  return <div className="mb-4"><A2UIRenderer blocks={items} onPrompt={() => {}} /></div>;
}
