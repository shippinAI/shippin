import type { DeploymentEvent } from "../../lib/hooks/use-deployments";

export function currentGenerationFailure(events: DeploymentEvent[], generation: number) {
  return [...events].reverse().find((event) =>
    event.generation === generation && event.event_type === "operation_failed");
}

export function deploymentFailureMessage(failureCode: string | undefined) {
  if (failureCode === "runtime_unsupported") {
    return "No application runtime is configured for this service and target. The worker made no provider changes.";
  }
  return `The reconciler stopped with code ${failureCode ?? "unknown"}. Check event history for the recorded outcome.`;
}
