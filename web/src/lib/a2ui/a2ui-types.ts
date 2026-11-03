/**
 * Google A2UI v0.9 (a2ui.org, stable; v0.9.1 current) server-to-client messages, the subset the
 * console renders. Components are a flat adjacency list; one has id "root".
 */
export const A2UI_VERSION = "v0.9";
export const SHIPPIN_CATALOG = "https://shippin.cloud/a2ui/v0_9/console-catalog.json";

export type DataPath = { path: string };
export type Dynamic<T> = T | DataPath;
export type ChildList = string[] | { path: string; componentId: string };

export type A2UIAction =
  | { event: { name: string; context?: Record<string, unknown> } }
  | { functionCall: { call: "openUrl"; args: { url: Dynamic<string> } } };

type Base = { id: string; weight?: number };
export type A2UIComponent = Base &
  (
    | { component: "Text"; text: Dynamic<string>; variant?: "h1" | "h2" | "h3" | "h4" | "h5" | "caption" | "body" }
    /** Shippin catalog extension: a monospace command block. */
    | { component: "Code"; text: Dynamic<string> }
    | { component: "Button"; child: string; variant?: "primary" | "borderless" | "default"; action: A2UIAction; tooltip?: string }
    | { component: "Card"; child: string }
    | { component: "Column" | "Row" | "List"; children: ChildList; align?: string; justify?: string }
    | { component: "TextField"; label: Dynamic<string>; value: DataPath; variant?: "shortText" | "longText" | "number" | "obscured" }
    | { component: "CheckBox"; label: Dynamic<string>; value: DataPath }
    | { component: "ChoicePicker"; label?: Dynamic<string>; options: { label: string; value: string }[]; value: DataPath; variant?: "mutuallyExclusive" | "multipleSelection" }
    | { component: "Divider"; axis?: "horizontal" | "vertical" }
  );

export type A2UIMessage =
  | { version: "v0.9"; createSurface: { surfaceId: string; catalogId: string; theme?: Record<string, unknown>; sendDataModel?: boolean } }
  | { version: "v0.9"; updateComponents: { surfaceId: string; components: A2UIComponent[] } }
  | { version: "v0.9"; updateDataModel: { surfaceId: string; path?: string; value?: unknown } }
  | { version: "v0.9"; deleteSurface: { surfaceId: string } };

/** Client-to-server action (v0.9 `action`), handed to the host instead of a network call. */
export type A2UIUserAction = {
  name: string;
  surfaceId: string;
  sourceComponentId: string;
  timestamp: string;
  context: Record<string, unknown>;
};

export type Surface = {
  id: string;
  catalogId: string;
  components: Record<string, A2UIComponent>;
  data: unknown;
};
