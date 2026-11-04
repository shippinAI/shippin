import type { CatalogDeploymentSource } from "../../../../lib/api/catalog-types";

export type DeploymentSourceDraft = {
  kind: "none" | CatalogDeploymentSource["kind"];
  reference: string;
  commit: string;
  context_path: string;
  credential_ref: string;
};

export const emptyDeploymentSource: DeploymentSourceDraft = {
  kind: "none",
  reference: "",
  commit: "",
  context_path: "",
  credential_ref: "",
};

export function sourceDraft(source?: CatalogDeploymentSource | null): DeploymentSourceDraft {
  return source ? {
    kind: source.kind,
    reference: source.reference,
    commit: source.commit ?? "",
    context_path: source.context_path ?? "",
    credential_ref: source.credential_ref ?? "",
  } : emptyDeploymentSource;
}

export function deploymentSourceFromDraft(draft: DeploymentSourceDraft): CatalogDeploymentSource | null {
  if (draft.kind === "none") return null;
  if (draft.kind === "oci") {
    return {
      kind: "oci",
      reference: draft.reference.trim(),
      ...(draft.credential_ref.trim() && { credential_ref: draft.credential_ref.trim() }),
    };
  }
  return {
    kind: draft.kind,
    reference: draft.reference.trim(),
    commit: draft.commit.trim(),
    ...(draft.context_path.trim() && { context_path: draft.context_path.trim() }),
    ...(draft.credential_ref.trim() && { credential_ref: draft.credential_ref.trim() }),
  };
}

export function validDeploymentSource(source: CatalogDeploymentSource | null): boolean {
  if (!source) return true;
  if (!source.reference || source.reference.length > 2048 || !validCredentialRef(source.credential_ref ?? "")) return false;
  if (source.context_path && !validContextPath(source.context_path)) return false;
  if (source.kind === "oci") {
    return /^[-A-Za-z0-9.]+(?::\d{1,5})?\/[A-Za-z0-9._/-]+@sha256:[a-f0-9]{64}$/.test(source.reference) &&
      !source.commit && !source.context_path;
  }
  if (source.kind !== "github" && source.kind !== "git") return false;
  if (!/^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(source.commit ?? "")) return false;
  try {
    const url = new URL(source.reference);
    const parts = url.pathname.replace(/\.git$/, "").replace(/^\/+|\/+$/g, "").split("/");
    return url.protocol === "https:" && !!url.hostname && !url.username && !url.password && !url.search && !url.hash &&
      parts.length >= 2 && parts.at(-1) !== "" && (source.kind !== "github" || url.hostname.toLowerCase() === "github.com");
  } catch {
    return false;
  }
}

function validContextPath(value: string): boolean {
  return value.length <= 512 && !value.startsWith("/") && !value.includes("\\") &&
    value.split("/").every((segment) => !!segment && segment !== "." && segment !== "..");
}

function validCredentialRef(value: string): boolean {
  if (!value) return true;
  if (value.length > 1024) return false;
  try {
    const url = new URL(value);
    if (!(url.protocol === "vaultix:" || url.protocol === "sops:") || url.username || url.password || url.search || (!url.host && !url.pathname)) return false;
    return url.protocol === "vaultix:" ? !url.hash : url.hash.startsWith("#/");
  } catch {
    return false;
  }
}
