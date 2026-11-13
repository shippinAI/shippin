import { apiGet, apiSend } from "./client";
import { actions, type ActionId } from "./actions.gen";

export type { ActionId };

/** An action's path with its {parameters} filled in. Paths come from the generated map, never from a string here. */
export function actionPath(id: ActionId, params: Record<string, string> = {}): string {
  return actions[id].path.replace(/\{(\w+)\}/g, (_, name: string) => {
    const v = params[name];
    if (v === undefined) throw new Error(`${id} needs ${name}`);
    return encodeURIComponent(v);
  });
}

/** Runs an action by its MCPL ID, as a person in the panel and an AI client both do. A change carries its
 * reason in the body, where the server's audit log picks it up. */
export function callAction<T>(id: ActionId, params: Record<string, string> = {}, body?: Record<string, unknown>): Promise<T> {
  const a = actions[id];
  const path = actionPath(id, params);
  return a.method === "GET" ? apiGet<T>(path) : apiSend<T>(a.method, path, body);
}
