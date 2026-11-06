import assert from "node:assert/strict";
import { currentGenerationFailure, deploymentEventLabel, deploymentEventSummary, deploymentFailureMessage, deploymentObservedStateTone } from "./deployment-status";

const event = (generation: number, event_type: string, failure_code?: string) => ({
  id: `event-${generation}-${event_type}`, deployment_id: "deployment", tenant_id: "tenant", generation,
  event_type, failure_code, actor: "operator", recorded_at: "2026-11-05T00:00:00Z",
});

const events = [event(1, "operation_failed", "runtime_unsupported"), event(2, "operation_started"),
  event(2, "operation_failed", "provider_timeout")];
assert.equal(currentGenerationFailure(events, 2)?.failure_code, "provider_timeout");
assert.equal(currentGenerationFailure(events, 1)?.failure_code, "runtime_unsupported");
assert.equal(currentGenerationFailure(events, 3), undefined);
assert.equal(deploymentFailureMessage("runtime_unsupported"),
	"The worker could not complete this deployment runtime. Earlier provider steps may have completed; inspect the operation history and provider state before retrying.");
assert.equal(deploymentFailureMessage(undefined), "The reconciler stopped with code unknown. Check event history for the recorded outcome.");
assert.equal(deploymentEventLabel("health_observed"), "Application health observed");
assert.equal(deploymentEventLabel("unexpected"), "Deployment event");
assert.equal(deploymentEventSummary({ event_type: "provider_observed", observed_state: "Running" }), "Provider state: Running.");
assert.equal(deploymentEventSummary({ event_type: "health_observed", health_status: "unknown",
  failure_code: "application_health_probe_unavailable" }),
"Application health: unknown. No target-scoped application probe is configured; health remains unknown.");
assert.equal(deploymentEventSummary({ event_type: "health_observed", health_status: "passing", health_duration_ms: 37 }),
  "Application health: passing (37 ms).");
assert.equal(deploymentEventSummary({ event_type: "health_observed", health_status: "failing", health_duration_ms: 250,
  failure_code: "provider_state_mismatch" }),
"Application health: failing (250 ms). The provider state does not match the requested operation.");
assert.equal(deploymentObservedStateTone("absent"), "success");
assert.equal(deploymentObservedStateTone("ready"), "success");
assert.equal(deploymentObservedStateTone("failed"), "danger");
assert.equal(deploymentObservedStateTone("unknown"), "warning");
console.log("deployment execution status model ok");
