import { useState } from "react";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { Button } from "../../../../lib/design/components/button/button";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Switch } from "../../../../lib/design/components/switch/switch";
import { type PlatformSetting, useSetPlatformSetting } from "../../../../lib/hooks/use-platform-settings";

/** One ops-editable config key: new value plus a reason (both audited), or reset to the config file. */
export function PlatformSettingRow({ s, canWrite }: { s: PlatformSetting; canWrite: boolean }) {
  const save = useSetPlatformSetting();
  const [value, setValue] = useState<string | boolean>(s.value);
  const [reason, setReason] = useState("");
  const ok = reason.trim().length >= 5 && value !== s.value;
  return (
    <li className="space-y-3 px-4 py-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="font-medium text-fg">{s.label}</p>
          <p className="text-sm text-fg-muted">{s.help}</p>
          <p className="mt-1 font-mono text-[12px] text-fg-subtle">{s.key} · config file: {String(s.config_value) || "(empty)"}</p>
        </div>
        <Badge tone={s.overridden ? "warning" : "neutral"}>{s.overridden ? "Overridden" : "From config"}</Badge>
      </div>
      {s.kind === "bool" ? (
        <Switch isSelected={value === true} onChange={setValue} isDisabled={!canWrite} tooltip="Turn on or off; saved with a reason">
          {value ? "On" : "Off"}
        </Switch>
      ) : (
        <TextField label="Value" value={String(value)} onChange={setValue} isDisabled={!canWrite} />
      )}
      {canWrite && (
        <div className="flex items-end gap-2">
          <div className="flex-1"><TextField label="Reason (audited)" value={reason} onChange={setReason} placeholder="Why this changes" /></div>
          <Button size="sm" variant="primary" isDisabled={!ok || save.isPending} tooltip={ok ? "Save the override (audited)" : "Change the value and give a reason"}
            onPress={() => save.mutate({ key: s.key, value, reason }, { onSuccess: () => setReason("") })}>Save</Button>
          {s.overridden && (
            <Button size="sm" variant="ghost" isDisabled={reason.trim().length < 5 || save.isPending} tooltip="Clear the override; the config file value applies again"
              onPress={() => save.mutate({ key: s.key, value: null, reason }, { onSuccess: () => { setReason(""); setValue(s.config_value); } })}>Reset to config</Button>
          )}
        </div>
      )}
    </li>
  );
}
