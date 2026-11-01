import { useCallback, useEffect, useState } from "react";
import { applyAll, applyMessage, setPath, type Surfaces } from "../../lib/a2ui/a2ui-model";
import type { A2UIMessage } from "../../lib/a2ui/a2ui-types";

/** Holds surfaces built from an A2UI message list; re-applies when the list changes, keeping any
 * local data-model edits made by input components. */
export function useSurfaces(messages: A2UIMessage[]) {
  const [surfaces, setSurfaces] = useState<Surfaces>(() => applyAll(messages));
  useEffect(() => { setSurfaces(applyAll(messages)); }, [messages]);
  const push = useCallback((m: A2UIMessage) => setSurfaces((s) => applyMessage(s, m)), []);
  const edit = useCallback((surfaceId: string, path: string, value: unknown) =>
    setSurfaces((s) => (s[surfaceId] ? { ...s, [surfaceId]: { ...s[surfaceId], data: setPath(s[surfaceId].data, path, value) } } : s)), []);
  return { surfaces, push, edit };
}
