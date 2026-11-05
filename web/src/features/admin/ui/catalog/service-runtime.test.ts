import assert from "node:assert/strict";
import { runtimeContractFromDraft, runtimeKindDraft, validRuntimeDraft } from "./service-runtime";

const source = { kind: "oci" as const, reference: "registry.example.net/team/app@sha256:" + "a".repeat(64) };
const git = { kind: "git" as const, reference: "https://git.example.net/team/app", commit: "a".repeat(40) };
const runtime = { version: 1 as const, kind: "incus_oci_container" as const, providers: ["incus"] as ["incus"] };

assert.equal(validRuntimeDraft("incus_oci_container", source), true);
assert.deepEqual(runtimeContractFromDraft("incus_oci_container", source), runtime);
assert.equal(validRuntimeDraft("incus_oci_container", git), false);
assert.equal(runtimeContractFromDraft("incus_oci_container", git), null);
assert.equal(runtimeContractFromDraft("none", source), null);
assert.equal(runtimeKindDraft(runtime), "incus_oci_container");
assert.equal(runtimeKindDraft({ ...runtime, providers: ["proxmox"] as unknown as ["incus"] }), "none");

console.log("catalog runtime declaration model ok");
