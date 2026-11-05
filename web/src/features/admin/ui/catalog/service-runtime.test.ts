import assert from "node:assert/strict";
import { emptyRuntimeContractDraft, exampleRuntimeContract, runtimeContractFromDraft, runtimeKindDraft, validRuntimeContract, validRuntimeDraft } from "./service-runtime";

const source = { kind: "oci" as const, reference: "registry.example.net/team/app@sha256:" + "a".repeat(64) };
const git = { kind: "git" as const, reference: "https://git.example.net/team/app", commit: "a".repeat(40) };
const runtime = { version: 1 as const, kind: "incus_oci_container" as const, providers: ["incus"] as ["incus"] };

const runtimeJSON = JSON.stringify(runtime);
assert.equal(validRuntimeDraft("incus_oci_container", source, runtimeJSON), true);
assert.deepEqual(runtimeContractFromDraft("incus_oci_container", source, runtimeJSON), runtime);
assert.equal(validRuntimeDraft("incus_oci_container", git, runtimeJSON), false);
assert.equal(runtimeContractFromDraft("incus_oci_container", git, runtimeJSON), null);
assert.equal(runtimeContractFromDraft("none", source, runtimeJSON), null);
assert.equal(runtimeKindDraft(runtime), "incus_oci_container");
assert.equal(runtimeKindDraft({ ...runtime, providers: ["proxmox"] as unknown as ["incus"] }), "none");

const recipe = exampleRuntimeContract();
assert.equal(validRuntimeContract(recipe), true);
assert.equal(validRuntimeContract(emptyRuntimeContractDraft()), false, "new recipes must require explicit app-specific configuration");
const { environment: _environment, ...runtimeV2 } = recipe;
assert.equal(validRuntimeContract({ ...runtimeV2, version: 2 }), true, "existing v2 recipes remain valid");
assert.equal(validRuntimeContract({ ...recipe, environment: [{ name: "bad-name", config_path: "/password" }] }), false);
assert.equal(validRuntimeContract({ ...recipe, environment: [{ name: "DB_PASSWORD", config_path: "/password/~2" }] }), false);
assert.equal(validRuntimeContract({ ...recipe, environment: [] }), true, "an empty v3 list is valid");
assert.equal(validRuntimeContract({ ...recipe, environment: [{ name: "DB_PASSWORD", config_path: "/one" }, { name: "DB_PASSWORD", config_path: "/two" }] }), false);
assert.equal(validRuntimeContract({ ...recipe, environment: Array.from({ length: 17 }, (_, i) => ({ name: `VAR_${i}`, config_path: `/field${i}` })) }), false);
assert.equal(validRuntimeDraft("incus_oci_container", source, JSON.stringify(recipe)), true);
assert.equal(validRuntimeContract({ ...recipe, storage: { ...recipe.storage, mount_path: "/var/lib/../etc" } }), false);
assert.equal(validRuntimeContract({ ...recipe, resources: { ...recipe.resources, cpu_cores: 0 } }), false);
assert.equal(validRuntimeContract({ ...recipe, health: { ...recipe.health, port: 5432 } }), false);
assert.equal(validRuntimeContract({ ...recipe, network: { ...recipe.network, ports: [...recipe.network.ports, ...recipe.network.ports] } }), false);
assert.equal(runtimeContractFromDraft("incus_oci_container", source, "{}"), null);
assert.equal(runtimeContractFromDraft("incus_oci_container", source, "null"), null);
assert.equal(runtimeContractFromDraft("incus_oci_container", source, JSON.stringify({ ...recipe, network: { ...recipe.network, ports: [null] } })), null);

console.log("catalog runtime declaration model ok");
