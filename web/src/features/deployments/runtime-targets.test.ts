import assert from "node:assert/strict";
import { requiresIncusStorageSelection, supportsRuntimeTarget } from "./runtime-targets";

const incusRuntime = { providers: ["incus"] };
assert.equal(supportsRuntimeTarget(incusRuntime, "incus"), true);
assert.equal(supportsRuntimeTarget(incusRuntime, "proxmox"), false);
assert.equal(supportsRuntimeTarget(incusRuntime, "unknown"), false);
assert.equal(supportsRuntimeTarget(undefined, "incus"), false);

assert.equal(requiresIncusStorageSelection({ version: 1, kind: "incus_oci_container", providers: ["incus"] }), false);
assert.equal(requiresIncusStorageSelection({ version: 2, kind: "incus_oci_container", providers: ["incus"],
  resources: { cpu_cores: 2, memory_mib: 1024 }, storage: { size_gib: 8, mount_path: "/data" },
  network: { exposure: "private", ports: [{ name: "app", protocol: "tcp", container_port: 8080 }] },
  health: { kind: "tcp", port: 8080, interval_seconds: 30, timeout_seconds: 5, failure_threshold: 3 } }), true);
assert.equal(requiresIncusStorageSelection({ version: 3, kind: "incus_oci_container", providers: ["incus"],
  resources: { cpu_cores: 2, memory_mib: 1024 }, storage: { size_gib: 8, mount_path: "/data" },
  network: { exposure: "private", ports: [{ name: "app", protocol: "tcp", container_port: 8080 }] },
  health: { kind: "tcp", port: 8080, interval_seconds: 30, timeout_seconds: 5, failure_threshold: 3 },
  environment: [] }), true);

console.log("deployment runtime target compatibility ok");
