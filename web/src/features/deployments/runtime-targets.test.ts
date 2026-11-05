import assert from "node:assert/strict";
import { supportsRuntimeTarget } from "./runtime-targets";

const incusRuntime = { providers: ["incus"] };
assert.equal(supportsRuntimeTarget(incusRuntime, "incus"), true);
assert.equal(supportsRuntimeTarget(incusRuntime, "proxmox"), false);
assert.equal(supportsRuntimeTarget(incusRuntime, "unknown"), false);
assert.equal(supportsRuntimeTarget(undefined, "incus"), false);

console.log("deployment runtime target compatibility ok");
