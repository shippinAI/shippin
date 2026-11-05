import type { CatalogRuntimeContract } from "../../lib/api/catalog-types";

export type RuntimeProviderDeclaration = { providers: readonly string[] };

export function supportsRuntimeTarget(runtime: RuntimeProviderDeclaration | undefined, provider: string): boolean {
  return runtime?.providers.includes(provider) ?? false;
}

export function requiresIncusStorageSelection(runtime: CatalogRuntimeContract | undefined | null): boolean {
  return runtime?.version === 2 || runtime?.version === 3;
}
