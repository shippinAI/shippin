export type RuntimeProviderDeclaration = { providers: readonly string[] };

export function supportsRuntimeTarget(runtime: RuntimeProviderDeclaration | undefined, provider: string): boolean {
  return runtime?.providers.includes(provider) ?? false;
}
