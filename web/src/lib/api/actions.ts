import { apiGet, apiSend } from "./client";
import { routes, type ActionId, type ActionTypes } from "./actions.gen";

export type { ActionId, ActionTypes };
/** What an action returns, from its Go type. */
export type ActionOutput<Id extends ActionId> = ActionTypes[Id]["output"];

const split = (id: ActionId) => {
  const route: string = routes[id];
  const at = route.indexOf(" ");
  return { method: route.slice(0, at) as "GET" | "POST" | "PUT" | "DELETE", path: route.slice(at + 1) };
};

/** An action's path with its {parameters} filled in. Paths come from the generated map, never from a string here. */
export function actionPath(id: ActionId, params: Record<string, string> = {}): string {
  return split(id).path.replace(/\{(\w+)\}/g, (_, name: string) => {
    const v = params[name];
    if (v === undefined) throw new Error(`${id} needs ${name}`);
    return encodeURIComponent(v);
  });
}

/** Runs an action by its MCPL ID, as a person in the panel and an AI client both do. Its parameters, body
 * and answer are typed from the action's Go types; a staff change must carry its reason in the body, where
 * the server's audit log picks it up. */
export function callAction<Id extends ActionId>(id: Id, ...args: ActionTypes[Id]["args"]): Promise<ActionOutput<Id>> {
  const [params, body] = args as unknown as [Record<string, string> | undefined, Record<string, unknown> | undefined];
  const { method } = split(id);
  const path = actionPath(id, params);
  return method === "GET" ? apiGet<ActionOutput<Id>>(path) : apiSend<ActionOutput<Id>>(method, path, body);
}
