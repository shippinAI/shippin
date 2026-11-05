import assert from "node:assert/strict";
import { validDeploymentSource } from "./service-source";

const commit = "0123456789abcdef0123456789abcdef01234567";
const reference = "https://git.example.net/acme/app";

assert.equal(validDeploymentSource({ kind: "git", reference, commit, credential_ref: "sops://runtime/secrets.yaml#/registry/token" }), true,
  "SOPS references select a key by JSON pointer");
assert.equal(validDeploymentSource({ kind: "git", reference, commit, credential_ref: "sops://runtime/secrets.yaml" }), false,
  "a SOPS file reference needs a key selector");
assert.equal(validDeploymentSource({ kind: "git", reference, commit, credential_ref: "vaultix://production/project/key#/token" }), false,
  "Vaultix references do not use fragments");

console.log("catalog secret reference model ok");
