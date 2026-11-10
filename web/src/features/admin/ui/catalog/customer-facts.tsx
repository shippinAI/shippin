import type { Customer, Intent } from "../../../../lib/api/catalog-types";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { Card, CardHeader } from "../../../../lib/design/components/card/card";
import { TermTip } from "./term-tip";

/** Contacts, endpoints (only when the API returned them: customers.contacts.read) and recorded provision intents. */
export function CustomerFacts({ customer: c, intents }: { customer: Customer; intents: Intent[] }) {
  return (
    <div className="grid gap-4 lg:grid-cols-3">
      {c.contacts && <Card>
        <CardHeader title="Contacts" />
        <ul className="divide-y divide-line text-base">
          {c.contacts.length === 0 && <li className="px-5 py-3 text-fg-muted">No contacts yet. Edit the customer to add who to reach.</li>}
          {c.contacts.map((x) => (
            <li key={x.email + x.name} className="px-5 py-2.5">
              <div className="font-medium text-fg">{x.name || "—"} <span className="text-xs text-fg-subtle">{x.role}</span></div>
              <div className="text-xs text-fg-muted">{x.email}</div>
            </li>
          ))}
        </ul>
      </Card>}
      {c.endpoints && <Card>
        <CardHeader title="Endpoints" />
        <ul className="divide-y divide-line text-base">
          {c.endpoints.length === 0 && <li className="px-5 py-3 text-fg-muted">No endpoints. Add LAN, tailnet or public hostnames.</li>}
          {c.endpoints.map((e) => (
            <li key={e.kind + e.host} className="flex items-center gap-2 px-5 py-2.5">
              <Badge>{e.kind}</Badge><span className="truncate font-mono text-sm">{e.host}</span>
            </li>
          ))}
        </ul>
      </Card>}
      <Card>
        <CardHeader title={<span className="inline-flex items-center gap-1.5">Provision intents <TermTip term="provision_intent" /></span>} />
        <ul className="divide-y divide-line text-base">
          {intents.length === 0 && <li className="px-5 py-3 text-fg-muted">None yet. Record one when the config looks right.</li>}
          {intents.slice(0, 5).map((i) => (
            <li key={i.id} className="px-5 py-2.5">
              <div className="flex items-center gap-2"><Badge tone="warning">{i.status}</Badge>
                <span className="text-xs text-fg-subtle">{new Date(i.created_at).toLocaleString()}</span></div>
              <div className="mt-1 text-xs text-fg-muted">{i.enabled_services.join(", ")}</div>
            </li>
          ))}
        </ul>
      </Card>
    </div>
  );
}
