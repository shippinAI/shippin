import assert from "node:assert/strict";
import type { SecretStoreConnection, SettingsFieldView, SettingsView, Version } from "../../../../../lib/api/actions.gen";
import {
  ago,
  disconnectBlock,
  displayValue,
  fieldOrigin,
  fromInput,
  historyEntries,
  initialDraft,
  list,
  noneId,
  pickOptions,
  planSave,
  problemOf,
  reasonOK,
  same,
  secretNeeded,
  sentence,
  storeBody,
  storeChanged,
  storeForm,
  storeFormProblem,
  storeStatus,
  toInput,
  whereLocked,
} from "./model";

const now = Date.parse("2026-11-15T12:00:00Z");

// ---- Reasons, times, sentences.
assert.equal(reasonOK("abcd"), false);
assert.equal(reasonOK("  abcd  "), false);
assert.equal(reasonOK("abcde"), true);
assert.equal(ago(null, now), "never");
assert.equal(ago("not a time", now), "never");
assert.equal(ago("2026-11-15T11:59:30Z", now), "just now");
assert.equal(ago("2026-11-15T11:55:00Z", now), "5 min ago");
assert.equal(ago("2026-11-15T09:00:00Z", now), "3 h ago");
assert.equal(ago("2026-11-13T12:00:00Z", now), "2 d ago");
assert.equal(sentence("signing in failed; check the client ID"), "Signing in failed; check the client ID.");
assert.equal(sentence("Done."), "Done.");
assert.equal(sentence("  "), "");

// ---- Problems: each status reads as what to do next.
assert.equal(
  problemOf({ status: 409, message: "This section changed since you opened it. Reload it to see the change, then save again." }).kind,
  "stale",
);
assert.equal(
  problemOf({ status: 409, message: "This section was saved by someone else just now. Reload it, then save again." }).kind,
  "stale",
);
assert.deepEqual(problemOf({ status: 409, message: "no secret store is connected: connect Vaultix in Settings > Secret store first" }), {
  kind: "no_store",
  message: "No secret store is connected: connect Vaultix in Settings > Secret store first.",
});
assert.equal(problemOf({ status: 409 }).kind, "stale");
assert.deepEqual(problemOf({ status: 400, message: "Workers must be between 1 and 64." }), {
  kind: "invalid",
  message: "Workers must be between 1 and 64.",
});
assert.deepEqual(problemOf({ status: 502, message: "Vaultix didn't save private_key: timeout" }), {
  kind: "vaultix",
  message: "Vaultix didn't save private_key: timeout. Nothing was changed.",
});
assert.equal(problemOf({ status: 502 }).kind, "vaultix");
assert.equal(problemOf({ status: 403, message: "forbidden" }).kind, "denied");
assert.equal(problemOf(new Error("Network down")).message, "Network down");
assert.equal(problemOf(undefined).message, "That didn't work. Try again in a moment.");

// ---- Secret store.
const conn = (over: Partial<SecretStoreConnection> = {}): SecretStoreConnection => ({
  connected: true,
  kind: "vaultix",
  api_url: "https://vaultix.example.com",
  project_id: "proj-1",
  environment: "prod",
  path: "/shippin",
  client_id: "id-1",
  has_client_secret: true,
  checked_at: "2026-11-15T11:55:00Z",
  check_error: "",
  keys_held: 0,
  ...over,
});
const off: SecretStoreConnection = {
  connected: false,
  kind: "vaultix",
  api_url: "",
  project_id: "",
  environment: "",
  path: "",
  client_id: "",
  has_client_secret: false,
  keys_held: 0,
};

assert.equal(storeStatus(undefined, now).label, "Checking…");
assert.equal(storeStatus(off, now).label, "Not connected");
assert.equal(storeStatus(off, now).tone, "warning");
assert.deepEqual(storeStatus(conn(), now), { label: "Connected", tone: "success", detail: "It worked when last checked, 5 min ago." });
assert.equal(storeStatus(conn({ checked_at: null }), now).detail, "Not checked yet.");
assert.deepEqual(storeStatus(conn({ check_error: "reading the check value back failed" }), now), {
  label: "Last check failed",
  tone: "danger",
  detail: "Reading the check value back failed.",
});

// The form is never prefilled with a client secret; a new one gets the default folder.
assert.deepEqual(storeForm(undefined), {
  api_url: "",
  project_id: "",
  environment: "",
  path: "/shippin",
  client_id: "",
  client_secret: "",
});
assert.deepEqual(storeForm(off), storeForm(undefined));
assert.equal(storeForm(conn()).client_secret, "");
assert.equal(storeForm(conn()).api_url, "https://vaultix.example.com");

const filled = {
  api_url: "https://vaultix.example.com/",
  project_id: "proj-1",
  environment: "prod",
  path: "",
  client_id: "id-1",
  client_secret: "s3cret",
};
assert.equal(storeFormProblem(filled, off), null);
assert.match(storeFormProblem({ ...filled, api_url: "vaultix" }, off) ?? "", /Vaultix address/);
assert.match(storeFormProblem({ ...filled, api_url: "https://user:pw@vaultix.example.com" }, off) ?? "", /Vaultix address/);
assert.match(storeFormProblem({ ...filled, project_id: "" }, off) ?? "", /project ID/);
assert.match(storeFormProblem({ ...filled, environment: "Prod" }, off) ?? "", /slug/);
assert.match(storeFormProblem({ ...filled, path: "shippin" }, off) ?? "", /folder/);
assert.match(storeFormProblem({ ...filled, client_id: " " }, off) ?? "", /client ID/);
// A new connection needs the secret; a saved one keeps it unless the identity changes.
assert.match(storeFormProblem({ ...filled, client_secret: "" }, off) ?? "", /client secret/);
assert.equal(storeFormProblem({ ...filled, client_secret: "" }, conn()), null);
assert.equal(secretNeeded({ ...filled, client_id: "id-2" }, conn()), true);
assert.equal(secretNeeded(filled, conn({ has_client_secret: false })), true);
assert.match(storeFormProblem({ ...filled, client_id: "id-2", client_secret: "" }, conn()) ?? "", /client secret/);
// While keys are held, only the identity may change.
assert.equal(whereLocked(conn({ keys_held: 2 })), true);
assert.equal(whereLocked(conn()), false);
assert.equal(whereLocked(off), false);
assert.match(storeFormProblem({ ...filled, environment: "staging" }, conn({ keys_held: 1 })) ?? "", /can't change/);
assert.equal(storeFormProblem({ ...filled, client_id: "id-2" }, conn({ keys_held: 1 })), null);
// The body is trimmed, takes the default folder, and leaves the secret out when none was typed.
assert.deepEqual(storeBody(filled), {
  api_url: "https://vaultix.example.com",
  project_id: "proj-1",
  environment: "prod",
  path: "/shippin",
  client_id: "id-1",
  client_secret: "s3cret",
});
assert.equal("client_secret" in storeBody({ ...filled, client_secret: "  " }), false);
assert.equal(storeChanged({ ...filled, client_secret: "" }, conn()), false);
assert.equal(storeChanged(filled, conn()), true);
assert.equal(storeChanged({ ...filled, client_secret: "", environment: "staging" }, conn()), true);
assert.equal(storeChanged({ ...filled, client_secret: "" }, off), true);
// Disconnecting is refused, in words, while keys are held.
assert.equal(disconnectBlock(conn()), null);
assert.match(disconnectBlock(conn({ keys_held: 1 })) ?? "", /^One settings section keeps keys/);
assert.match(disconnectBlock(conn({ keys_held: 3 })) ?? "", /^3 settings sections keep keys/);
assert.equal(disconnectBlock(off), "No secret store is connected.");

// ---- Fields: inputs and values.
const fld = (over: Partial<SettingsFieldView>): SettingsFieldView => ({ name: "f", label: "Field", kind: "text", ...over });
assert.equal(toInput(fld({ kind: "bool" }), true), true);
assert.equal(toInput(fld({ kind: "bool" }), null), false);
assert.equal(toInput(fld({ kind: "integer" }), 5), "5");
assert.equal(toInput(fld({ kind: "list" }), ["a", "b"]), "a\nb");
assert.equal(toInput(fld({ kind: "list" }), null), "");
assert.equal(toInput(fld({ kind: "map" }), { contents: "write", issues: "read" }), "contents=write\nissues=read");
assert.equal(toInput(fld({ kind: "text" }), null), "");

const workers = fld({ name: "workers", label: "Jobs at once", kind: "integer", min: 1, max: 64 });
assert.deepEqual(fromInput(workers, " 8 "), { value: 8 });
assert.deepEqual(fromInput(workers, "0"), { error: "Jobs at once must be between 1 and 64." });
assert.deepEqual(fromInput(workers, "2.5"), { error: "Jobs at once must be a whole number." });
assert.deepEqual(fromInput(workers, ""), { error: "Jobs at once must be a whole number." });
assert.deepEqual(fromInput(fld({ kind: "integer", min: 5 }), "4"), { error: "Field must be at least 5." });
assert.deepEqual(fromInput(fld({ kind: "bool" }), true), { value: true });
assert.deepEqual(fromInput(fld({ kind: "list" }), " a \n\n b "), { value: ["a", "b"] });
assert.deepEqual(fromInput(fld({ kind: "map" }), "contents = write\nissues=read\n"), { value: { contents: "write", issues: "read" } });
assert.match((fromInput(fld({ kind: "map" }), "no equals") as { error: string }).error, /name=value/);
assert.deepEqual(fromInput(fld({ kind: "text" }), "  x  "), { value: "x" });
assert.deepEqual(fromInput(fld({ kind: "text" }), " "), { error: "Field is required." });
assert.deepEqual(fromInput(fld({ kind: "text", optional: true }), ""), { value: "" });
assert.deepEqual(fromInput(fld({ kind: "text", max: 3 }), "abcd"), { error: "Field is longer than 3 characters." });
assert.deepEqual(fromInput(fld({ kind: "url" }), "https://api.github.com"), { value: "https://api.github.com" });
assert.match((fromInput(fld({ kind: "url" }), "ftp://x") as { error: string }).error, /http or https/);
assert.match((fromInput(fld({ kind: "url" }), "https://x.example.com/?q=1") as { error: string }).error, /query/);
assert.deepEqual(fromInput(fld({ kind: "long_text" }), "a\nb"), { value: "a\nb" });
assert.match((fromInput(fld({ kind: "text" }), "a\u0007b") as { error: string }).error, /one line/);
const choice = fld({ kind: "choice", choices: [{ value: "a", label: "Alpha" }] });
assert.deepEqual(fromInput(choice, "a"), { value: "a" });
assert.match((fromInput(choice, "b") as { error: string }).error, /pick one/);
assert.equal(displayValue(choice, "a"), "Alpha");
assert.equal(displayValue(fld({ kind: "bool" }), false), "off");
assert.equal(displayValue(fld({}), ""), "nothing");
assert.equal(displayValue(fld({ kind: "list" }), []), "an empty list");
assert.equal(displayValue(fld({ kind: "map" }), { a: "b" }), "a=b");
assert.equal(same({ a: 1, b: [1, 2] }, { b: [1, 2], a: 1 }), true);
assert.equal(same([1, 2], [2, 1]), false);
assert.equal(list(["a"]), "a");
assert.equal(list(["a", "b", "c"]), "a, b and c");

// ---- Where a value comes from.
const key = (over: Partial<SettingsFieldView>) =>
  fld({ name: "private_key", label: "Private key", kind: "long_key", env: "SHIPPIN_GITHUB_PRIVATE_KEY", ...over });
assert.deepEqual(fieldOrigin({ version: 3 }, key({ key: { saved: true, in: "secret_store", name: "/shippin/github/PRIVATE_KEY_V3" } })), {
  label: "In Vaultix",
  tone: "success",
  hint: "Saved in Vaultix as /shippin/github/PRIVATE_KEY_V3.",
});
assert.equal(fieldOrigin({ version: 0 }, key({ key: { saved: true, in: "environment" } })).label, "In the environment");
assert.match(fieldOrigin({ version: 0 }, key({ key: { saved: true, in: "environment" } })).hint, /SHIPPIN_GITHUB_PRIVATE_KEY/);
assert.equal(fieldOrigin({ version: 0 }, key({ key: { saved: false, in: "" } })).label, "Not set");
assert.equal(fieldOrigin({ version: 0 }, workers).label, "Default");
assert.equal(fieldOrigin({ version: 0 }, { ...workers, value: 4, file_value: 4 }).label, "From the file");
assert.deepEqual(fieldOrigin({ version: 2 }, { ...workers, value: 8, file_value: 4 }), {
  label: "Set in the panel",
  tone: "primary",
  hint: "The bootstrap file has 4.",
});
assert.equal(fieldOrigin({ version: 2 }, { ...workers, value: 4, file_value: 4 }).hint, "The same as the bootstrap file.");
assert.equal(fieldOrigin({ version: 2 }, { ...workers, value: 4 }).hint, "The bootstrap file doesn't set it.");

// ---- Saving: only what changed, against the version.
const view = (over: Partial<SettingsView> = {}): SettingsView => ({
  key: "github",
  title: "GitHub App",
  summary: "",
  version: 0,
  source: "file",
  fields: [
    fld({ name: "enabled", label: "Use the GitHub App", kind: "bool", value: false, file_value: false }),
    fld({ name: "app_id", label: "App ID", kind: "text", optional: true, value: "", file_value: "" }),
    { ...workers, value: 4, file_value: 4 },
    fld({ name: "tenants", label: "Only these organizations", kind: "list", optional: true, value: ["t1"], file_value: ["t1"] }),
    fld({
      name: "token_permissions",
      label: "Token permissions",
      kind: "map",
      value: { contents: "write" },
      file_value: { contents: "write" },
    }),
    key({ key: { saved: true, in: "environment" } }),
    fld({ name: "webhook_secret", label: "Webhook secret", kind: "key", key: { saved: false, in: "" } }),
  ],
  ...over,
});

const v0 = view();
const d0 = initialDraft(v0);
assert.deepEqual(d0.inputs, { enabled: false, app_id: "", workers: "4", tenants: "t1", token_permissions: "contents=write" });
assert.deepEqual(d0.keys, {});
// Nothing changed: nothing to send (the environment key would move, but only with a change).
let p = planSave(v0, d0, true);
assert.deepEqual(p.changed, []);
assert.deepEqual(p.errors, {});
// Retyping the same value, or the same list with spacing, is no change.
p = planSave(v0, { ...d0, inputs: { ...d0.inputs, workers: " 4", tenants: "t1\n" } }, true);
assert.deepEqual(p.changed, []);
assert.equal(p.body.values, undefined);
// A change sends only that field, with the version; on a first save the environment key moves too.
p = planSave(v0, { ...d0, inputs: { ...d0.inputs, workers: "8", enabled: true } }, true);
assert.deepEqual(p.body, { version: 0, values: { enabled: true, workers: 8 }, import: ["private_key"] });
assert.deepEqual(p.changed, ["Use the GitHub App", "Jobs at once"]);
assert.deepEqual(p.moved, ["Private key"]);
assert.equal(p.blocked, null);
// Without a secret store, that first save would lose the environment key, so it is blocked, in words.
p = planSave(v0, { ...d0, inputs: { ...d0.inputs, workers: "8" } }, false);
assert.match(p.blocked ?? "", /^Saving this section for the first time moves Private key from the environment into Vaultix/);
// An invalid value is a field error and is not sent.
p = planSave(v0, { ...d0, inputs: { ...d0.inputs, workers: "100" } }, true);
assert.deepEqual(p.errors, { workers: "Jobs at once must be between 1 and 64." });
assert.equal(p.body.values, undefined);
// Keys: a new value, a removal (null), a move; never a value of the old one.
p = planSave(
  view({ version: 3, source: "panel" }),
  {
    ...d0,
    keys: { webhook_secret: { op: "replace", value: "  whsec  " }, private_key: { op: "remove" } },
  },
  true,
);
assert.deepEqual(p.body, { version: 3, keys: { private_key: null, webhook_secret: "whsec" } });
assert.deepEqual(p.changed, ["Private key (removed)", "Webhook secret (new key)"]);
assert.deepEqual(p.moved, []);
// A long key keeps its lines and inner spacing.
p = planSave(
  view({ version: 3 }),
  { ...d0, keys: { private_key: { op: "replace", value: "-----BEGIN-----\nabc\n-----END-----\n" } } },
  true,
);
assert.deepEqual(p.body.keys, { private_key: "-----BEGIN-----\nabc\n-----END-----\n" });
// An empty replacement or a multi-line short key is refused before sending.
p = planSave(view({ version: 3 }), { ...d0, keys: { webhook_secret: { op: "replace", value: "  " } } }, true);
assert.match(p.errors.webhook_secret ?? "", /Enter the new webhook secret/);
p = planSave(view({ version: 3 }), { ...d0, keys: { webhook_secret: { op: "replace", value: "a\nb" } } }, true);
assert.equal(p.errors.webhook_secret, "Webhook secret must be one line.");
// Moving one key by hand is sent as import; a new key without a store is blocked.
p = planSave(v0, { ...d0, keys: { private_key: { op: "import" } } }, true);
assert.deepEqual(p.body, { version: 0, import: ["private_key"] });
assert.deepEqual(p.changed, ["Private key (moved to Vaultix)"]);
p = planSave(view({ version: 3 }), { ...d0, keys: { webhook_secret: { op: "replace", value: "x" } } }, false);
assert.equal(p.blocked, "Keys are saved in Vaultix, so connect the secret store first.");
// Removing needs no store.
p = planSave(view({ version: 3 }), { ...d0, keys: { private_key: { op: "remove" } } }, false);
assert.equal(p.blocked, null);
// A map and a list are sent as their values.
p = planSave(view({ version: 1 }), { ...d0, inputs: { ...d0.inputs, tenants: "t1\nt2", token_permissions: "contents=read" } }, true);
assert.deepEqual(p.body.values, { tenants: ["t1", "t2"], token_permissions: { contents: "read" } });

// ---- Picking from a panel list.
assert.deepEqual(pickOptions([{ id: "f1", label: "Main" }], "", true), [
  { id: noneId, label: "None" },
  { id: "f1", label: "Main" },
]);
assert.deepEqual(pickOptions([{ id: "f1", label: "" }], "gone", false), [
  { id: "f1", label: "f1" },
  { id: "gone", label: "gone (not found)" },
]);

// ---- History: newest first, each against the one before it, the first against the file. Keys are named only.
const versions: Version[] = [
  {
    version: 1,
    values: { enabled: false, workers: 8, app_id: "" },
    keys_changed: ["private_key"],
    changed_by: "ana",
    reason: "First setup",
    changed_at: "2026-11-14T10:00:00Z",
  },
  {
    version: 2,
    values: { enabled: true, workers: 8, app_id: "" },
    keys_changed: [],
    changed_by: "bo",
    reason: "Turn it on",
    changed_at: "2026-11-15T10:00:00Z",
  },
];
const h = historyEntries(v0, versions);
assert.deepEqual(
  h.map((e) => e.version),
  [2, 1],
);
assert.deepEqual(h[0].changes, ["Use the GitHub App: off → on"]);
assert.deepEqual(h[1].changes, ["Jobs at once: 4 → 8", "Private key: key changed"]);
assert.equal(h[1].by, "ana");
assert.equal(historyEntries(v0, []).length, 0);

console.log("settings model ok");
