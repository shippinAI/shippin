import assert from "node:assert/strict";
import type { AcquisitionSourceDetail } from "../../../../lib/api/actions.gen";
import { acquisitionCapabilities, leadPreviewState, persistedSourceStateUpdate, sourceDraftIsStale, validSourceOriginPort } from "./acquisition-model";

assert.equal(acquisitionCapabilities(["acquisition.read"]).enterSignups, true);
assert.equal(acquisitionCapabilities(["acquisition.read"]).sourceConfigure, false);
assert.equal(acquisitionCapabilities(["acquisition.read", "platform.settings.write"]).sourceConfigure, true);
assert.equal(acquisitionCapabilities(["acquisition.sync"]).sourceSync, true);
assert.equal(acquisitionCapabilities(["platform.settings.write"]).waitlist, false);

assert.equal(validSourceOriginPort("https://partner.example", "443"), true);
assert.equal(validSourceOriginPort("https://partner.example:8443", "8443"), true);
assert.equal(validSourceOriginPort("https://partner.example:8443", "443"), false);
assert.equal(validSourceOriginPort("http://partner.example", "443"), false);
assert.equal(validSourceOriginPort("https://user:pass@partner.example", "443"), false);

assert.equal(sourceDraftIsStale(7, 7), false);
assert.equal(sourceDraftIsStale(7, 8), true, "a newer server generation cannot be silently overwritten");
assert.equal(sourceDraftIsStale(null, 7), true, "an uninitialized form is not saveable");

const detail: AcquisitionSourceDetail = {
  summary: { id: "source-1", label: "Saved label", paused: false, config_generation: 12, cursor: 1, active_lead_count: 3, signer_configured: true, ca_configured: true },
  origin: "https://saved.example", dial_ip: "192.0.2.5", port: 9443, source_tenant: "tenant", source_project: "project", issuer: "issuer", audience: "audience", ca_pem: "saved CA",
};
assert.deepEqual(persistedSourceStateUpdate(detail, true), {
  expected_config_generation: 12,
  label: "Saved label",
  origin: "https://saved.example",
  dial_ip: "192.0.2.5",
  port: 9443,
  ca_pem: "saved CA",
  paused: true,
}, "pause/resume payloads derive from saved config, not unsaved form values, and omit signer data");

assert.equal(leadPreviewState(0, 0, false), "empty");
assert.equal(leadPreviewState(110, 100, true), "latest");
assert.equal(leadPreviewState(2, 2, false), "complete");

console.log("acquisition UI model ok");
