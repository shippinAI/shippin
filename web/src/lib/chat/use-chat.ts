import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import { streamMessage } from "./chat-stream";
import type { ChatMessage, ChatProvider, ChatReady, ChatSession } from "./chat-types";

export const useChatConfig = () =>
  useQuery({ queryKey: ["chat", "config"], queryFn: () => apiGet<{ providers: ChatProvider[]; catalog: string[] }>("/v1/chat/config"), staleTime: 300_000 });

export const useChatSessions = () => useQuery({ queryKey: ["chat", "sessions"], queryFn: () => apiGet<{ items: ChatSession[] }>("/v1/chat/sessions") });

export const useChatSession = (id: string | null) =>
  useQuery({
    queryKey: ["chat", "session", id],
    queryFn: () => apiGet<{ session: ChatSession; messages: ChatMessage[] }>(`/v1/chat/sessions/${id}`),
    enabled: !!id,
  });

export const useChatReady = (id: string | null) =>
  useQuery({ queryKey: ["chat", "ready", id], queryFn: () => apiGet<ChatReady>(`/v1/chat/sessions/${id}/ready`), enabled: !!id, staleTime: 60_000 });

export function useCreateChat() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (b: { workspace_id: string; provider: string }) => apiSend<ChatSession>("POST", "/v1/chat/sessions", b),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["chat", "sessions"] }),
  });
}

/** Sends a message and exposes the reply as it streams; the stored rows replace it when done. */
export function useSendChat(id: string | null) {
  const qc = useQueryClient();
  const [draft, setDraft] = useState<string | null>(null);
  const [pending, setPending] = useState<string | null>(null);
  const send = useMutation({
    mutationFn: async (text: string) => {
      setPending(text);
      setDraft("");
      await streamMessage(id!, text, (e) => {
        if (e.event === "delta") setDraft((d) => (d ?? "") + e.data.text);
        if (e.event === "user" || e.event === "done") qc.invalidateQueries({ queryKey: ["chat", "session", id] });
      });
    },
    onSettled: async () => {
      await qc.invalidateQueries({ queryKey: ["chat", "session", id] });
      await qc.invalidateQueries({ queryKey: ["chat", "sessions"] });
      setDraft(null);
      setPending(null);
    },
  });
  return { send, draft, pending };
}
