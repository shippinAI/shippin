import type { DeploymentEvent } from "../../lib/hooks/use-deployments";

const eventLabels: Record<string, string> = {
  requested: "Request recorded",
  review_allowed: "AgentHub authorization allowed",
  review_denied: "AgentHub authorization denied",
  operation_started: "Provider operation started",
  provider_observed: "Provider state observed",
  health_observed: "Application health observed",
  operation_failed: "Operation stopped",
  operation_cancelled: "Operation cancelled",
};

const eventFailureMessages: Record<string, string> = {
  application_health_probe_unavailable: "No target-scoped application probe is configured; health remains unknown.",
  provider_state_absent: "The provider did not find the reviewed workload during read-back.",
  provider_state_mismatch: "The provider state does not match the requested operation.",
  runtime_unsupported: "This service runtime is not fully supported; earlier provider steps may have completed.",
};

export function currentGenerationFailure(events: DeploymentEvent[], generation: number) {
  return [...events].reverse().find((event) =>
    event.generation === generation && event.event_type === "operation_failed");
}

export function deploymentFailureMessage(failureCode: string | undefined) {
  if (failureCode === "runtime_unsupported") {
    return "The worker could not complete this deployment runtime. Earlier provider steps may have completed; inspect the operation history and provider state before retrying.";
  }
  return `The reconciler stopped with code ${failureCode ?? "unknown"}. Check event history for the recorded outcome.`;
}

export function deploymentEventLabel(eventType: string) {
  return eventLabels[eventType] ?? "Deployment event";
}

export function deploymentEventSummary(event: {
  event_type: string;
  observed_state?: string;
  health_status?: string;
  health_duration_ms?: number;
  failure_code?: string;
}) {
  const details: string[] = [];
  if (event.event_type === "provider_observed") {
    details.push(`Provider state: ${event.observed_state ?? "unknown"}.`);
  }
  if (event.event_type === "health_observed") {
    const status = event.health_status ?? "unknown";
    const duration = event.health_duration_ms === undefined ? "" : ` (${event.health_duration_ms} ms)`;
    details.push(`Application health: ${status}${duration}.`);
  }
  if (event.failure_code) details.push(deploymentEventFailureMessage(event.failure_code));
  return details.join(" ");
}

function deploymentEventFailureMessage(failureCode: string) {
  return eventFailureMessages[failureCode] ?? `Recorded failure code: ${failureCode}.`;
}

export function deploymentObservedStateTone(state: string) {
  if (state === "ready" || state === "absent") return "success" as const;
  if (state === "failed") return "danger" as const;
  return "warning" as const;
}
