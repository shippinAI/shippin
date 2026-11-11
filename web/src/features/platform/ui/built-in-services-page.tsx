import { useState } from "react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { StrictDialog } from "../../../lib/design/components/dialog/strict-dialog";
import { TextField } from "../../../lib/design/components/field/text-field";
import { Select } from "../../../lib/design/components/select/select";
import { PageHeader } from "../../../template/page-header";
import { useBuiltInServices, useSetClickHouse, type BuiltInService, type ServiceMode } from "../api";
import { RecoveryCard } from "./recovery-card";
import { serviceBadge } from "./service-wording";

const modeLabel: Record<ServiceMode, string> = { "built-in": "Built-in", external: "External", off: "Off" };
const clickHouseModes = [{ id: "built-in", label: "Built-in" }, { id: "external", label: "External server" }, { id: "off", label: "Off" }];

function ChangeClickHouse({ onClose }: { onClose: () => void }) {
  const change = useSetClickHouse();
  const [mode, setMode] = useState<ServiceMode>("external");
  const [url, setUrl] = useState("");
  return (
    <StrictDialog isOpen onClose={onClose} title="Change ClickHouse" confirm="Change ClickHouse" busy={change.isPending}
      ready={mode !== "external" || url.startsWith("http")}
      warning="New metrics go to the new place straight away. History stays behind in the old one, and the panel will not show it."
      understand="I understand this changes where my metrics and event history live"
      onConfirm={(reason) => change.mutate({ mode, url: mode === "external" ? url : "", reason }, { onSuccess: onClose })}>
      <Select label="Use" options={clickHouseModes} value={mode} onChange={(v) => setMode(v as ServiceMode)} />
      {mode === "external" && <TextField label="Address" mono type="url" placeholder="https://user:password@host:8443/shippin" description="The panel tests it and creates its tables before switching." value={url} onChange={setUrl} />}
    </StrictDialog>
  );
}

function ServiceCard({ service }: { service: BuiltInService }) {
  const [changing, setChanging] = useState(false);
  return (
    <Card className="overflow-hidden" data-testid={`service-${service.id}`}>
      <CardHeader title={service.name} description={service.endpoint || "Not connected"}
        actions={<div className="flex items-center gap-2"><Badge>{modeLabel[service.mode]}</Badge>{serviceBadge(service)}</div>} />
      <div className="space-y-3 px-5 py-4">
        {service.problem && <p role="alert" className="text-base text-danger">{service.problem}</p>}
        <dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {service.version && <div><dt className="text-sm text-fg-subtle">Version</dt><dd className="font-mono text-base text-fg">{service.version}</dd></div>}
          {service.facts.map((f) => <div key={f.label}><dt className="text-sm text-fg-subtle">{f.label}</dt><dd className="text-base text-fg">{f.value}</dd></div>)}
        </dl>
        {service.changeable
          ? <Button size="sm" variant="secondary" tooltip={`Change or turn off ${service.name}`} onPress={() => setChanging(true)}>Change</Button>
          : <p className="text-sm text-fg-subtle">Set when the panel was installed. To move it, see the deploy guide.</p>}
      </div>
      {changing && service.id === "clickhouse" && <ChangeClickHouse onClose={() => setChanging(false)} />}
    </Card>
  );
}

/** The services this installation runs on, and how to get it back after losing them. */
export function BuiltInServicesPage() {
  const services = useBuiltInServices();
  return (
    <>
      <PageHeader title="Built-in services" description="What this panel runs on, live. Changing one asks you to confirm what it holds." />
      {services.isError && <p role="alert" className="mb-4 text-base text-danger">{services.error.message}</p>}
      <div className="space-y-4">
        {(services.data?.items ?? []).map((s) => <ServiceCard key={s.id} service={s} />)}
      </div>
      <div className="mt-8"><RecoveryCard /></div>
    </>
  );
}
