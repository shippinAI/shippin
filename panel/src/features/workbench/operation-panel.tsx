import type { FormEvent } from "react";
import type { OpenApiOperation } from "../../shared/contracts";

export function OperationPanel(props: {
  routeLabel: string;
  operations: OpenApiOperation[];
  selectedOperation: OpenApiOperation | null;
  selectedOperationId: string;
  pathParameters: string[];
  tenantId: string;
  pathParamValues: Record<string, string>;
  requestBody: string;
  isSubmitting: boolean;
  operatorModeBlocked: boolean;
  capabilityBlocked: boolean;
  requiredCapability: string;
  requestError: string;
  onSelectOperation: (value: string) => void;
  onTenantChange: (value: string) => void;
  onPathParameterChange: (key: string, value: string) => void;
  onRequestBodyChange: (value: string) => void;
  onExecute: (event: FormEvent) => void;
}) {
  return (
    <section className="panel">
      <h3>{props.routeLabel} operations</h3>
      <form onSubmit={props.onExecute}>
        <label className="full-width">
          Operation
          <select value={props.selectedOperationId} onChange={(event) => props.onSelectOperation(event.target.value)} disabled={!props.operations.length}>
            {props.operations.map((operation) => (
              <option key={operation.operationId} value={operation.operationId}>
                {operation.method} {operation.path} - {operation.summary || operation.operationId}
              </option>
            ))}
          </select>
        </label>
        {props.pathParameters.length ? (
          <div className="field-grid">
            {props.pathParameters.map((name) => (
              <label key={name}>
                {name}
                <input value={name === "tenantId" ? props.tenantId : props.pathParamValues[name] ?? ""}
                  onChange={(event) => name === "tenantId" ? props.onTenantChange(event.target.value) : props.onPathParameterChange(name, event.target.value)}
                  placeholder={`value for ${name}`} />
              </label>
            ))}
          </div>
        ) : null}
        {props.selectedOperation && !["GET", "HEAD"].includes(props.selectedOperation.method) ? (
          <label className="full-width">
            Request body (JSON)
            <textarea value={props.requestBody} onChange={(event) => props.onRequestBodyChange(event.target.value)} rows={6} placeholder='{"name":"example"}' />
          </label>
        ) : null}
        <div className="action-row">
          <button type="submit" disabled={props.isSubmitting || !props.selectedOperation || props.operatorModeBlocked || props.capabilityBlocked}>
            {props.isSubmitting ? "Executing..." : "Execute against API"}
          </button>
          {props.selectedOperation ? (
            <span className="muted">Status: {props.selectedOperation.implementationStatus} · Capability: {props.requiredCapability}</span>
          ) : <span className="muted">No operation available for current mode.</span>}
        </div>
      </form>
      {props.capabilityBlocked ? <p className="error-banner">Action blocked: current identity does not allow capability {props.requiredCapability}.</p> : null}
      {props.requestError ? <p className="error-banner">{props.requestError}</p> : null}
    </section>
  );
}
