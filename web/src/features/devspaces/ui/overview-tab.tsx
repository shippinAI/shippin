import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { CopyField } from "../../../lib/design/components/field/copy-field";
import { Disclosure } from "../../../lib/design/components/disclosure/disclosure";
import type { Workspace } from "../../../lib/api/types";
import { formatBytes } from "../../../lib/format";

/** Specs and endpoints, with internal identifiers tucked behind Details. */
export function OverviewTab({ ws }: { ws: Workspace }) {
  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader title="Endpoints" description="How to reach this workspace." />
          <CopyField label="Browser IDE" value={ws.url} mono hint="HTTPS, sign-in required" />
          <CopyField label="Public IPv4" value={ws.public_ip} mono />
          <CopyField label="Private IPv4" value={ws.ipv4 ?? ""} mono hint="Reachable inside your private network" />
          <CopyField label="SSH port" value={String(ws.ssh_port)} mono />
        </Card>
        <Card>
          <CardHeader title="Specs" />
          <CopyField label="Image" value="Debian 13 · browser IDE, git, Go, Node" />
          <CopyField label="Memory" value={formatBytes(ws.memory_bytes)} />
          <CopyField label="Region" value={ws.region} />
          <CopyField label="SSH keys" value={`${ws.ssh_key_count} from your account`} />
        </Card>
      </div>
      <Disclosure title="Details">
        <CopyField label="Workspace ID" value={ws.id} mono />
        <CopyField label="Instance" value={ws.instance} mono />
        <CopyField label="Tenant ID" value={ws.tenant_id} mono />
      </Disclosure>
    </div>
  );
}
