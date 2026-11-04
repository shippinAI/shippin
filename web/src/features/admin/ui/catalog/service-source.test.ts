import assert from "node:assert/strict";
import { deploymentSourceFromDraft, emptyDeploymentSource, sourceDraft, validDeploymentSource } from "./service-source";

const commit = "0123456789abcdef0123456789abcdef01234567";
const digest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";

assert.equal(validDeploymentSource(null), true, "services without an installer source remain supported");
assert.equal(validDeploymentSource({ kind: "github", reference: "https://github.com/acme/postgres", commit, context_path: "deploy/postgres", credential_ref: "vaultix://org/project/runtime/git-token" }), true);
assert.equal(validDeploymentSource({ kind: "git", reference: "https://git.example.net/acme/cache", commit }), true);
assert.equal(validDeploymentSource({ kind: "oci", reference: `registry.example.net/data/postgres@sha256:${digest}` }), true);
assert.equal(validDeploymentSource({ kind: "github", reference: "https://git.example.net/acme/app", commit }), false, "GitHub provider must point at GitHub");
assert.equal(validDeploymentSource({ kind: "git", reference: "https://user:secret@git.example.net/acme/app", commit }), false, "credentials cannot be embedded in the URL");
assert.equal(validDeploymentSource({ kind: "git", reference: "https://git.example.net/acme/app", commit: "main" }), false, "git refs must be immutable commits");
assert.equal(validDeploymentSource({ kind: "git", reference: "https://git.example.net/acme/app", commit, context_path: "../private" }), false, "build context cannot escape the repository");
assert.equal(validDeploymentSource({ kind: "oci", reference: "registry.example.net/data/postgres:latest" }), false, "OCI tags float and must be pinned by digest");
assert.equal(validDeploymentSource({ kind: "git", reference: "https://git.example.net/acme/app", commit, credential_ref: "plaintext-token" }), false, "credentials are references only");

const draft = sourceDraft({ kind: "oci", reference: `registry.example.net/data/postgres@sha256:${digest}` });
assert.deepEqual(deploymentSourceFromDraft(draft), { kind: "oci", reference: `registry.example.net/data/postgres@sha256:${digest}` });
assert.deepEqual(deploymentSourceFromDraft(emptyDeploymentSource), null);

console.log("catalog deployment source model ok");
