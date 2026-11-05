import type { DeploymentEvent } from "../../lib/hooks/use-deployments";

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

export function deploymentObservedStateTone(state: string) {
  if (state === "ready" || state === "absent") return "success" as const;
  if (state === "failed") return "danger" as const;
  return "warning" as const;
}
