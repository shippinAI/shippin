import { useState } from "react";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { Button } from "../../../../lib/design/components/button/button";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Select } from "../../../../lib/design/components/select/select";
import { Switch } from "../../../../lib/design/components/switch/switch";
import { type PlatformSetting, useSetPlatformSetting } from "../../../../lib/hooks/use-platform-settings";

/** One ops-editable config key: new value plus a reason (both audited), or reset to the config file. */
export function PlatformSettingRow({ s, canWrite }: { s: PlatformSetting; canWrite: boolean }) {
  const save = useSetPlatformSetting();
  const [value, setValue] = useState<string | boolean>(s.value as string | boolean);
  const [reason, setReason] = useState("");
  const parsedNumber = Number(value);
  const validInteger = Number.isInteger(parsedNumber) && parsedNumber >= (s.min ?? 1) && parsedNumber <= (s.max ?? Number.MAX_SAFE_INTEGER);
  const ok = reason.trim().length >= 5 && value !== s.value && (s.kind !== "integer" || validInteger);
  return (
    <li className="space-y-3 px-4 py-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="font-medium text-fg">{s.label}</p>
          <p className="text-base text-fg-muted">{s.help}</p>
          <p className="mt-1 font-mono text-xs text-fg-subtle">{s.key} · config file: {String(s.config_value) || "(empty)"}</p>
        </div>
        <Badge tone={s.overridden ? "warning" : "neutral"}>{s.overridden ? "Overridden" : "From config"}</Badge>
      </div>
      {s.kind === "bool" ? (
        <Switch isSelected={value === true} onChange={setValue} isDisabled={!canWrite} tooltip="Turn on or off; saved with a reason">
          {value ? "On" : "Off"}
        </Switch>
      ) : s.kind === "choice" && s.choices ? (
        <Select label="Value" value={String(value)} onChange={setValue} isDisabled={!canWrite} options={s.choices.map((c) => ({ id: c.value, label: c.label }))} />
      ) : (
        <TextField label="Value" type={s.kind === "integer" ? "number" : "text"} value={String(value)} onChange={setValue} isDisabled={!canWrite} min={s.kind === "integer" ? s.min : undefined} max={s.kind === "integer" ? s.max : undefined} step={s.kind === "integer" ? 1 : undefined} />
      )}
      {canWrite && (
        <div className="flex items-end gap-2">
          <div className="flex-1"><TextField label="Reason (audited)" value={reason} onChange={setReason} placeholder="Why this changes" /></div>
          <Button size="sm" variant="primary" isDisabled={!ok || save.isPending} tooltip={ok ? "Save the override (audited)" : "Change the value and give a reason"}
            onPress={() => save.mutate({ key: s.key, value: s.kind === "integer" ? parsedNumber : value, reason }, { onSuccess: () => setReason("") })}>Save</Button>
          {s.overridden && (
            <Button size="sm" variant="ghost" isDisabled={reason.trim().length < 5 || save.isPending} tooltip="Clear the override; the config file value applies again"
              onPress={() => save.mutate({ key: s.key, value: null, reason }, { onSuccess: () => { setReason(""); setValue(String(s.config_value)); } })}>Reset to config</Button>
          )}
        </div>
      )}
    </li>
  );
}
