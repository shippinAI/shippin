import { useEffect, useMemo, useState } from "react";
import { useMe } from "../../../lib/hooks/use-api";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { TextField } from "../../../lib/design/components/field/text-field";
import { PageHeader } from "../../../template/page-header";
import { availableOperations, bodyError, hasBody, matchOperations, pathParameters, resolvePath, type ApiOperation, type OperationIndex } from "../operations";

type Result = { method: string; url: string; status: number; statusText: string; body: string };

async function send(method: string, url: string, body: string): Promise<Result> {
  const withBody = hasBody(method) && body.trim().length > 0;
  const res = await fetch(url, {
    method,
    credentials: "same-origin",
    cache: "no-store",
    headers: { Accept: "application/json", ...(withBody ? { "Content-Type": "application/json" } : {}) },
    body: withBody ? body : undefined,
  });
  return { method, url, status: res.status, statusText: res.statusText, body: await res.text() };
}

/** Provider panel: call any documented operation as yourself. The API enforces every permission. */
export function ApiExplorerPage() {
  const [index, setIndex] = useState<OperationIndex | null>(null);
  const [query, setQuery] = useState("");
  const [operationId, setOperationId] = useState("");
  const [params, setParams] = useState<Record<string, string>>({});
  const [body, setBody] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<Result | null>(null);

  useEffect(() => {
    void import("../openapi-index.json").then((m) => setIndex(m.default as OperationIndex));
  }, []);

  const caps = useMe().data?.capabilities;
  const matches = useMemo(() => matchOperations(availableOperations(index?.operations ?? [], caps ?? {}), query), [index, caps, query]);
  const selected: ApiOperation | undefined = matches.find((o) => o.operationId === operationId) ?? matches[0];
  const names = selected ? pathParameters(selected.path) : [];

  const run = async () => {
    if (!selected || !index) return;
    setError("");
    const invalid = hasBody(selected.method) ? bodyError(body) : null;
    if (invalid) {
      setError(invalid);
      return;
    }
    setBusy(true);
    try {
      setResult(await send(selected.method, index.basePath + resolvePath(selected.path, params), body));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <PageHeader title="API explorer" description="Every operation in the published contract. Requests run as you, against this installation, and the API enforces every permission." />
      <Card className="mb-6 p-5">
        <div className="space-y-4">
          <TextField label="Find an operation" value={query} onChange={setQuery} placeholder="method, path, tag or summary" />
          <label className="flex flex-col gap-1.5 text-sm font-medium text-fg">
            Operation ({matches.length} of {index?.operationCount ?? 0})
            <select aria-label="Operation" className="h-9 rounded-md border border-line bg-canvas px-2 font-mono text-sm font-normal" value={selected?.operationId ?? ""}
              onChange={(e) => { setOperationId(e.target.value); setParams({}); setBody(""); setError(""); }}>
              {matches.map((o) => <option key={o.operationId} value={o.operationId}>{o.method} {o.path}</option>)}
            </select>
          </label>
          {selected && (
            <p className="flex flex-wrap items-center gap-2 text-base text-fg-muted">
              <Badge tone={selected.implementationStatus === "registered" ? "success" : "neutral"}>{selected.implementationStatus}</Badge>
              {selected.summary || selected.operationId}
            </p>
          )}
          {names.length > 0 && (
            <div className="grid gap-4 sm:grid-cols-2">
              {names.map((n) => <TextField key={n} label={n} mono value={params[n] ?? ""} onChange={(v) => setParams((p) => ({ ...p, [n]: v }))} />)}
            </div>
          )}
          {selected && hasBody(selected.method) && (
            <TextField label="Request body (JSON)" multiline mono value={body} onChange={setBody} />
          )}
          {error && <p role="alert" className="text-base text-danger">{error}</p>}
          <Button variant="primary" size="sm" isDisabled={!selected || busy} tooltip="Send this request now, as you" onPress={() => void run()}>
            {busy ? "Sending…" : "Send request"}
          </Button>
        </div>
      </Card>
      <Card className="overflow-hidden">
        <CardHeader title="Response" description={result ? `${result.method} ${result.url}` : "Nothing sent yet."}
          actions={result ? <Badge tone={result.status < 300 ? "success" : result.status < 500 ? "warning" : "danger"}>{result.status} {result.statusText}</Badge> : undefined} />
        {result && <pre className="max-h-[60vh] overflow-auto whitespace-pre-wrap break-all px-5 pb-5 font-mono text-xs text-fg-muted">{result.body || "(empty response body)"}</pre>}
      </Card>
    </>
  );
}
