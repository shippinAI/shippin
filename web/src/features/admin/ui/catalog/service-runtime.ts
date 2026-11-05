import type { CatalogDeploymentSource, CatalogRuntimeContract } from "../../../../lib/api/catalog-types";

export type RuntimeKindDraft = "none" | "incus_oci_container";

export function runtimeKindDraft(runtime?: CatalogRuntimeContract | null): RuntimeKindDraft {
  return runtime?.kind === "incus_oci_container" && runtime.version === 1 &&
    runtime.providers.length === 1 && runtime.providers[0] === "incus" ? runtime.kind : "none";
}

export function runtimeContractFromDraft(kind: RuntimeKindDraft, source: CatalogDeploymentSource | null): CatalogRuntimeContract | null {
  if (kind === "none" || source?.kind !== "oci") return null;
  return { version: 1, kind: "incus_oci_container", providers: ["incus"] };
}

export function validRuntimeDraft(kind: RuntimeKindDraft, source: CatalogDeploymentSource | null): boolean {
  return kind === "none" || (kind === "incus_oci_container" && source?.kind === "oci");
}
