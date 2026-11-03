import { Card, CardHeader } from "../../../../lib/design/components/card/card";
import { usePlatformSettings } from "../../../../lib/hooks/use-platform-settings";
import { useAdminAccess } from "../../../../lib/hooks/use-admin";
import { PageHeader } from "../../../../template/page-header";
import { PlatformSettingRow } from "./platform-setting-row";
import { PlatformProbeSettings } from "./platform-probe-settings";

/** Provider panel: the config keys ops may override (the config file stays the default). */
export function PlatformSettingsPage() {
  const q = usePlatformSettings();
  const canWrite = (useAdminAccess().data?.staff?.permissions ?? []).includes("platform.settings.write");
  return (
    <>
      <PageHeader title="Platform settings" description="Overrides of selected config keys. Every change needs a reason and is audited." />
      {q.isError && <p role="alert" className="text-sm text-danger">{q.error.message}</p>}
      {q.data && (
        <Card>
          <CardHeader title="Settings" description="Reset returns a key to the value in the config file." />
          <ul className="divide-y divide-border" aria-label="Platform settings">
            {q.data.items.filter((s) => s.kind === "array" && s.key === "services.probes").map((s) => <PlatformProbeSettings key={s.key} setting={s} canWrite={canWrite} />)}
            {q.data.items.filter((s) => s.kind !== "json" && s.kind !== "array").map((s) => <PlatformSettingRow key={s.key} s={s} canWrite={canWrite} />)}
          </ul>
        </Card>
      )}
    </>
  );
}
