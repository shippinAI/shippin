import assert from "node:assert/strict";
import { currentGenerationFailure, deploymentFailureMessage } from "./deployment-status";

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
  "No application runtime is configured for this service and target. The worker made no provider changes.");
assert.equal(deploymentFailureMessage(undefined), "The reconciler stopped with code unknown. Check event history for the recorded outcome.");
console.log("deployment execution status model ok");
