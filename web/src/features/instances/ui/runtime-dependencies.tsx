import { useQuery } from "@tanstack/react-query";
import { Card, CardHeader } from "../../../lib/design/components/card/card";

type Dependency = "up" | "down" | "disabled";
type Readiness = { db: boolean; cache: Dependency; clickhouse: Dependency };
const connection = (state: Dependency) => (state === "up" ? "Connected" : state === "disabled" ? "Not configured" : "Unavailable");

export function RuntimeDependencies() {
  const state = useQuery({ queryKey: ["runtime-readiness"], refetchInterval: 15000, queryFn: async () => {
    const res = await fetch("/readyz");
    if (res.status !== 200 && res.status !== 503) throw new Error("Readiness unavailable");
    return await res.json() as Readiness;
  } });
  const unknown = !state.data || state.isError;
  const rows = [
    { name: "PostgreSQL", purpose: "Platform records, encrypted sessions, durable jobs", status: unknown ? "Unknown" : state.data?.db ? "Connected" : "Unavailable" },
    { name: "ClickHouse", purpose: "Metrics and event history", status: unknown ? "Unknown" : connection(state.data!.clickhouse) },
    { name: "Dragonfly / Redis", purpose: "Shared authentication rate limits", status: unknown ? "Unknown" : state.data?.cache === "up" ? "Connected" : state.data?.cache === "disabled" ? "Not configured (process-local limits)" : "Unavailable" },
  ];
  return <Card className="mb-5 overflow-hidden">
    <CardHeader title="Control plane dependencies" description="Live checks of the services this panel uses. Local or hosted connections are selected in installation configuration." />
    <table className="w-full text-left text-base"><thead><tr className="border-b border-line"><th className="px-5 py-3">Service</th><th className="px-5 py-3">Used for</th><th className="px-5 py-3">Connection</th></tr></thead><tbody>{rows.map((row) => <tr key={row.name} className="border-b border-line"><td className="px-5 py-3 font-medium">{row.name}</td><td className="px-5 py-3 text-fg-muted">{row.purpose}</td><td className="px-5 py-3">{row.status}</td></tr>)}</tbody></table>
  </Card>;
}
