import type { ApiExecutionResult } from "../../shared/contracts";

export function ResultPanel({ result }: { result: ApiExecutionResult | null }) {
  return (
    <section className="panel">
      <h3>Result</h3>
      {result ? (
        <div className="result-block">
          <p><strong>{result.response.status} {result.response.statusText}</strong></p>
          <p className="muted">{result.request.method} {result.request.url}</p>
          <pre>{result.response.body || "(empty response body)"}</pre>
        </div>
      ) : <p className="muted">No request executed in this session.</p>}
    </section>
  );
}
