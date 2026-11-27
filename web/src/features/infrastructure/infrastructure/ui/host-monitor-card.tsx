import { useEffect, useState } from "react";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { Button } from "../../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../../lib/design/components/card/card";
import { ModalDialog } from "../../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { useHostMonitor, useSetHostMonitor } from "../../../../lib/hooks/use-hosts";
import { useInfraCan } from "../../../../lib/hooks/use-infra";
import { useText } from "../../../../lib/ui-config/use-ui-config";
import { noteOK } from "../model";

const DEFAULT_IMAGE = "images:alpine/3.21";

/** The host monitor of one server: whether the panel can read the host's live load, CPU and memory, and
 *  the switch for it. A kind of server that reports its own load shows nothing here. */
export function HostMonitorCard({ serverId }: { serverId: string }) {
  const t = useText();
  const state = useHostMonitor(serverId);
  const set = useSetHostMonitor(serverId);
  const may = useInfraCan();
  const [open, setOpen] = useState(false);
  const [image, setImage] = useState(DEFAULT_IMAGE);
  const [note, setNote] = useState("");
  const on = state.data?.enabled === true;
  useEffect(() => {
    if (open) setNote("");
  }, [open]);
  if (!state.data?.supported) return null;
  const ready = noteOK(note) && (on || image.trim() !== "") && !set.isPending;
  const submit = () => {
    if (!ready) return;
    set.mutate({ enabled: !on, image: on ? undefined : image.trim(), note: note.trim() }, { onSuccess: () => setOpen(false) });
  };
  return (
    <Card className="mb-6 overflow-hidden" data-testid="host-monitor">
      <CardHeader
        title={t("infra.monitor.title")}
        description={on ? t("infra.monitor.onBody") : t("infra.monitor.offBody")}
        actions={
          <div className="flex items-center gap-2">
            <Badge tone={on ? (state.data.running ? "success" : "warning") : "neutral"}>
              {on ? (state.data.running ? t("infra.monitor.running") : t("infra.monitor.stopped")) : t("infra.monitor.off")}
            </Badge>
            {may("infra.hosts.write") && (
              <Button
                size="sm"
                variant="secondary"
                tooltip={on ? t("infra.monitor.turnOffTooltip") : t("infra.monitor.turnOnTooltip")}
                onPress={() => setOpen(true)}
              >
                {on ? t("infra.monitor.turnOff") : t("infra.monitor.turnOn")}
              </Button>
            )}
          </div>
        }
      />
      <ModalDialog
        isOpen={open}
        onOpenChange={setOpen}
        title={on ? t("infra.monitor.turnOff") : t("infra.monitor.turnOn")}
        description={on ? t("infra.monitor.turnOffBody") : t("infra.monitor.turnOnBody")}
      >
        <form
          className="space-y-4 px-6 py-5"
          data-testid="host-monitor-form"
          onSubmit={(e) => {
            e.preventDefault();
            submit();
          }}
        >
          {!on && (
            <TextField label={t("infra.monitor.image")} description={t("infra.monitor.imageHelp")} value={image} onChange={setImage} />
          )}
          <TextField label={t("infra.note.label")} description={t("infra.note.help")} value={note} onChange={setNote} autoFocus />
          <div className="flex justify-end gap-2 border-t border-line pt-4">
            <Button variant="ghost" tooltip={t("infra.dialog.cancelTooltip")} onPress={() => setOpen(false)}>
              {t("infra.dialog.cancel")}
            </Button>
            <Button type="submit" variant="primary" isDisabled={!ready} tooltip={t("infra.note.tooltip")}>
              {on ? t("infra.monitor.turnOff") : t("infra.monitor.turnOn")}
            </Button>
          </div>
        </form>
      </ModalDialog>
    </Card>
  );
}
