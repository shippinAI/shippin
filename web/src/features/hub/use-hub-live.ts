import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

export type LiveState = "connecting" | "live" | "offline";

/** Server-Sent Events from /v1/hub/stream: every change refreshes the board and that card's history. */
export function useHubLive(): LiveState {
  const qc = useQueryClient();
  const [state, setState] = useState<LiveState>("connecting");
  useEffect(() => {
    const es = new EventSource("/v1/hub/stream", { withCredentials: true });
    es.onopen = () => setState("live");
    es.onerror = () => setState(es.readyState === EventSource.CLOSED ? "offline" : "connecting");
    es.addEventListener("change", (e) => {
      const { task_id } = JSON.parse((e as MessageEvent<string>).data) as { task_id: string };
      void qc.invalidateQueries({ queryKey: ["hub", "tasks"] });
      void qc.invalidateQueries({ queryKey: ["hub", "events", task_id] });
    });
    return () => es.close();
  }, [qc]);
  return state;
}
