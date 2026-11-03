import { useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { apiGet } from "../api/client";
import { textFor, type UiConfig } from "./model";
import type { TextKey } from "./text-keys";

export const uiConfigKey = ["ui-config"];

export const useUiConfig = () =>
  useQuery({ queryKey: uiConfigKey, queryFn: () => apiGet<UiConfig>("/v1/settings/ui"), staleTime: 300_000 });

/** The configured wording of panel text; the built-in default until config has loaded. */
export function useText() {
  const ui = useUiConfig().data;
  return useCallback((key: TextKey, vars?: Record<string, string | number>) => textFor(ui, key, vars), [ui]);
}
