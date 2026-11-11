import { Check, Circle } from "lucide-react";
import type { ReactNode } from "react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { LinkButton } from "../../../lib/design/components/button/link-button";
import { Card } from "../../../lib/design/components/card/card";
import { Checkbox } from "../../../lib/design/components/checkbox/checkbox";
import { useInfraOverview } from "../../../lib/hooks/use-infra";
import { PageHeader } from "../../../template/page-header";
import { tabHref, NETWORKS_TAB, OVERVIEW_TAB } from "../../infrastructure/model";
import { useAcknowledgeStep, useBackups, useBuiltInServices, useSetupSteps } from "../api";
import { serviceBadge, understandings } from "./service-wording";

function Step({ number, title, done, optional, children }: { number: number; title: string; done?: boolean; optional?: boolean; children: ReactNode }) {
  return (
    <li>
      <Card className="p-5">
        <div className="flex items-start gap-4">
          <span aria-hidden className={`flex size-7 shrink-0 items-center justify-center rounded-full border text-sm font-semibold ${done ? "border-success/40 bg-success/12 text-success" : "border-line-strong text-fg-muted"}`}>
            {done ? <Check className="size-4" /> : number}
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-lg font-semibold text-fg">{title}</h2>
              {done ? <Badge tone="success">Done</Badge> : optional ? <Badge>Optional</Badge> : <Badge tone="warning"><Circle className="size-2" aria-hidden /> To do</Badge>}
            </div>
            <div className="mt-2 space-y-3 text-base text-fg-muted">{children}</div>
          </div>
        </div>
      </Card>
    </li>
  );
}

/** The first thing an owner sees: each step reads what is really true of this installation. */
export function GetStartedPage() {
  const services = useBuiltInServices();
  const steps = useSetupSteps();
  const backups = useBackups();
  const overview = useInfraOverview();
  const acknowledge = useAcknowledgeStep();
  const acknowledged = new Set((steps.data?.items ?? []).map((s) => s.step));
  const items = services.data?.items ?? [];
  const inUse = items.filter((s) => s.state !== "disabled");
  const allAcknowledged = inUse.length > 0 && inUse.every((s) => acknowledged.has(s.id));
  const hosts = (overview.data?.providers ?? []).reduce((n, p) => n + p.nodes.length, 0);
  const https = window.location.protocol === "https:";
  const backedUp = (backups.data?.items.length ?? 0) > 0;
  const required = [allAcknowledged, https, hosts > 0, backedUp];
  return (
    <>
      <PageHeader title="Get started" description={`${required.filter(Boolean).length} of ${required.length} steps done. Each one checks what is really set up here.`} />
      <ol className="max-w-4xl space-y-4">
        <Step number={1} title="Know what this panel runs on" done={allAcknowledged}>
          <p>These services came with the install. You can change them later, but each holds something you would miss.</p>
          <ul className="space-y-2">
            {inUse.map((s) => (
              <li key={s.id} className="rounded-lg border border-line p-3">
                <div className="mb-1 flex items-center gap-2 text-fg"><span className="font-medium">{s.name}</span>{serviceBadge(s)}</div>
                <Checkbox isSelected={acknowledged.has(s.id)} isDisabled={acknowledged.has(s.id) || acknowledge.isPending} onChange={() => acknowledge.mutate(s.id)}>
                  {understandings[s.id] ?? `I understand this panel depends on ${s.name}`}
                </Checkbox>
              </li>
            ))}
          </ul>
          <LinkButton size="sm" variant="secondary" href="/admin/built-in" tooltip="See and change the built-in services">Built-in services</LinkButton>
        </Step>
        <Step number={2} title="Decide how people reach this panel" done={https}>
          <p>{https ? <>HTTPS is on at <span className="font-mono text-fg">{window.location.host}</span>. To reach it privately, add a Cloudflare Tunnel or Tailscale connection.</> : "This panel is not on HTTPS. Sign-in cookies need it, so set up a certificate or a proxy first."}</p>
          <LinkButton size="sm" variant="secondary" href={tabHref(NETWORKS_TAB, "/admin/infrastructure")} tooltip="Add a tunnel or private network">Network connections</LinkButton>
        </Step>
        <Step number={3} title="Add your first server" done={hosts > 0}>
          <p>{hosts > 0 ? `${hosts} ${hosts === 1 ? "server is" : "servers are"} connected.` : "Connect an Incus or Proxmox server. It can be a virtual machine or a whole physical machine."}</p>
          <LinkButton size="sm" variant={hosts > 0 ? "secondary" : "primary"} href={tabHref(OVERVIEW_TAB, "/admin/infrastructure")} tooltip="Open hosts and guests">Hosts & guests</LinkButton>
        </Step>
        <Step number={4} title="Make a recovery point" done={backedUp}>
          <p>{backedUp ? "You have a backup. Keep one somewhere other than this machine, with the recovery key." : "Back up the database and save the recovery key before you depend on this panel."}</p>
          <LinkButton size="sm" variant={backedUp ? "secondary" : "primary"} href="/admin/built-in" tooltip="Back up and save the recovery key">Disaster recovery</LinkButton>
        </Step>
        <Step number={5} title="Add your people" optional>
          <p>Create accounts, set their roles, or connect a sign-in provider.</p>
          <LinkButton size="sm" variant="secondary" href="/admin/team" tooltip="Manage who can use this panel">Team</LinkButton>
        </Step>
      </ol>
    </>
  );
}
