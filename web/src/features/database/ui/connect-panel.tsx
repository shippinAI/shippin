import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { CodeBlock } from "../../../lib/design/components/code-block/code-block";
import { CopyField } from "../../../lib/design/components/field/copy-field";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import type { DbInfo } from "../../../lib/api/database-types";

export function ConnectPanel({ info, loading }: { info?: DbInfo; loading: boolean }) {
  if (loading || !info) {
    return <Card><div className="space-y-3 p-5">{Array.from({ length: 4 }, (_, i) => <Skeleton key={i} className="h-10" />)}</div></Card>;
  }
  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-[1fr_1fr]">
      <Card className="overflow-hidden">
        <CardHeader title="Connection" description={info.password_note} />
        <CopyField label="URI" value={info.uri} mono hint="Password is a placeholder" />
        <CopyField label="Host" value={info.host} mono />
        <CopyField label="Port" value={String(info.port)} mono />
        <CopyField label="Database" value={info.database} mono />
        <CopyField label="User" value={info.role} mono hint="Your database role, not a superuser" />
        <CopyField label="SSL" value={info.sslmode} mono />
      </Card>
      <div className="space-y-4">
        <Card className="p-5">
          <h3 className="text-base font-semibold text-fg">Before you paste this</h3>
          <ul className="mt-2 space-y-2 text-sm leading-relaxed text-fg-muted">
            {info.hints.map((h) => <li key={h}>{h}</li>)}
            <li>{info.login_ready ? "This console can sign in as the role above." : "The console does not have the role password yet. Hostd keeps it in sops until a pooler is connected."}</li>
          </ul>
        </Card>
        <Card className="space-y-4 p-5">
          <CodeBlock label="JavaScript" code={info.snippets.javascript} />
          <CodeBlock label="Python" code={info.snippets.python} />
          <CodeBlock label="Go" code={info.snippets.go} />
        </Card>
      </div>
    </div>
  );
}
