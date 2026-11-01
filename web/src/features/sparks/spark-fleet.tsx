import { useSparks } from "./use-sparks";
import { SparkEndpoint } from "./spark-endpoint";

export function SparkFleet({ tenantId }: { tenantId: string }) {
  const q = useSparks(tenantId);
  if (q.isError) return <p role="alert" className="mt-6 text-sm text-danger">Spark access could not be loaded: {q.error.message}</p>;
  if (!q.data?.nodes.length) return null;
  return (
    <section className="mt-8" aria-label="Sparks">
      <h2 className="mb-2 text-[15px] font-semibold">Sparks</h2>
      <p className="mb-4 text-sm text-fg-subtle">Your model API and management pages. Only connected HTTPS destinations can be opened.</p>
      {q.data.model_api && (
        <div className="mb-4 rounded-[var(--radius-card)] border border-line bg-surface p-5">
          <SparkEndpoint endpoint={q.data.model_api} modelAPI />
        </div>
      )}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {q.data.nodes.map((node) => (
          <article key={node.id} className="space-y-4 rounded-[var(--radius-card)] border border-line bg-surface p-5">
            <div><h3 className="font-semibold">{node.name}</h3><p className="mt-1 text-xs text-fg-subtle">{node.role}</p></div>
            {node.management.map((endpoint) => <SparkEndpoint key={endpoint.name} endpoint={endpoint} />)}
          </article>
        ))}
      </div>
    </section>
  );
}
