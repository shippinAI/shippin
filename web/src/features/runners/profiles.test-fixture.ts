import type { ProfileSchema } from "./profiles";
export const apiFixtureSchema: ProfileSchema = {
  "x-contract-version": "1", "x-enforcement-status": "unverified",
  required: ["image_sha256", "kernel_sha256", "runner_version"],
  properties: {
    vcpu_count: { type: "integer", description: "vCPU count for the development alpha", minimum: 1, maximum: 2, default: 2 },
    memory_mib: { type: "integer", description: "Guest RAM in MiB", minimum: 256, maximum: 4096, default: 4096 },
    disk_gib: { type: "integer", description: "Maximum job disk in GiB", minimum: 1, maximum: 20, default: 20 },
    timeout_seconds: { type: "integer", description: "Maximum attempt duration in seconds", minimum: 1, maximum: 1800, default: 1800 },
    image_sha256: { type: "string", description: "Pinned root filesystem SHA-256", pattern: "^[a-f0-9]{64}$" },
    kernel_sha256: { type: "string", description: "Pinned guest kernel SHA-256", pattern: "^[a-f0-9]{64}$" },
    runner_version: { type: "string", description: "Pinned GitHub runner version", pattern: "^v?[0-9]+\\.[0-9]+\\.[0-9]+$" },
  },
};
