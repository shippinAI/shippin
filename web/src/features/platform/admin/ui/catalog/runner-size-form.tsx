import { useState } from "react";
import { callAction } from "../../../../../lib/api/actions";
import type { RunnerSize } from "../../../../../lib/api/actions.gen";
import { Button } from "../../../../../lib/design/components/button/button";
import { ModalDialog } from "../../../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../../../lib/design/components/field/text-field";
import { Switch } from "../../../../../lib/design/components/switch/switch";
import { useCatalogAction } from "../../../../../lib/hooks/use-catalog";
import { blankSize, sizeFields, sizeProblem, sizeSummary, type SizeFields } from "./runner-sizes";

const num = (v: string) => (v.trim() === "" ? Number.NaN : Number(v));

/** Create, edit, turn off or delete a runner size. Every change carries a reason for the audit log. */
export function RunnerSizeForm({ initial, onClose }: { initial?: RunnerSize; onClose: () => void }) {
  const [f, setF] = useState<SizeFields>(initial ? sizeFields(initial) : blankSize);
  const [reason, setReason] = useState("");
  const save = useCatalogAction(initial ? "Runner size updated" : "Runner size created");
  const del = useCatalogAction("Runner size deleted");
  const set =
    <K extends keyof SizeFields>(k: K) =>
    (v: SizeFields[K]) =>
      setF({ ...f, [k]: v });
  const problem = sizeProblem(f);
  const reasonOK = reason.trim().length >= 5;
  const { id, ...fields } = f;
  const input = { ...fields, reason };
  const error = save.error ?? del.error;
  return (
    <ModalDialog
      isOpen
      onOpenChange={(o) => !o && onClose()}
      title={initial ? `Edit ${initial.label}` : "New runner size"}
      description="A job asks for a size with runs-on: shippin-<id>. Ids are permanent; everything else can change. A plan includes sizes in the plan matrix."
    >
      <div className="grid grid-cols-2 gap-4 px-6 py-5">
        <div className="col-span-2">
          <TextField
            label="Reason (audited)"
            value={reason}
            onChange={setReason}
            placeholder="Why this changes"
            description="Kept in the audit log with the change. At least 5 characters."
          />
        </div>
        <TextField
          label="Id"
          value={f.id}
          onChange={set("id")}
          isDisabled={!!initial}
          mono
          isRequired
          description={f.id ? `runs-on: shippin-${f.id}` : "Such as 2x or large"}
        />
        <TextField
          label="Label"
          value={f.label}
          onChange={set("label")}
          isRequired
          maxLength={80}
          description="What a customer reads, such as 2 cores, 8 GB"
        />
        <TextField
          label="Cores"
          type="number"
          min={1}
          max={128}
          value={Number.isFinite(f.cores) ? String(f.cores) : ""}
          onChange={(v) => set("cores")(num(v))}
        />
        <TextField
          label="Memory (MiB)"
          type="number"
          min={512}
          max={1048576}
          step={512}
          value={Number.isFinite(f.memory_mib) ? String(f.memory_mib) : ""}
          onChange={(v) => set("memory_mib")(num(v))}
          description="8192 MiB is 8 GiB"
        />
        <TextField
          label="Disk (GiB)"
          type="number"
          min={8}
          max={4096}
          value={Number.isFinite(f.disk_gib) ? String(f.disk_gib) : ""}
          onChange={(v) => set("disk_gib")(num(v))}
        />
        <div className="self-end pb-2">
          <Switch
            isSelected={f.active}
            onChange={set("active")}
            tooltip="Off: new jobs asking for this size are refused; running ones finish"
          >
            Offered
          </Switch>
        </div>
        <div className="col-span-2">
          <TextField
            label="Runner image"
            value={f.image}
            onChange={set("image")}
            mono
            maxLength={200}
            description="The provider image with the GitHub Actions runner installed. While it is empty, jobs asking for this size are refused as not ready."
          />
        </div>
        {!problem && <p className="col-span-2 text-sm text-fg-muted">Each job gets {sizeSummary(f)}.</p>}
        {problem && (f.id || f.label) && (
          <p role="alert" className="col-span-2 text-sm text-danger">
            {problem}
          </p>
        )}
        {error && (
          <p role="alert" className="col-span-2 text-sm text-danger">
            {error.message}
          </p>
        )}
      </div>
      <div className="flex gap-2 border-t border-line px-6 py-4">
        {initial && (
          <Button
            tooltip={reasonOK ? "Delete this size; refused while a job waits for or runs on it" : "Give a reason first"}
            variant="danger"
            isDisabled={!reasonOK || del.isPending}
            onPress={() =>
              del.mutate(() => callAction("shippin.services.runner_size.delete", { runnerSizeId: id }, { reason }), { onSuccess: onClose })
            }
          >
            Delete
          </Button>
        )}
        <div className="flex-1" />
        <Button tooltip="Cancel" variant="ghost" onPress={onClose}>
          Cancel
        </Button>
        <Button
          tooltip={problem ?? (reasonOK ? "Save runner size" : "Give a reason first")}
          variant="primary"
          isDisabled={save.isPending || !!problem || !reasonOK}
          onPress={() =>
            save.mutate(
              () =>
                initial
                  ? callAction("shippin.services.runner_size.update", { runnerSizeId: id }, input)
                  : callAction("shippin.services.runner_size.create", {}, { id, ...input }),
              { onSuccess: onClose },
            )
          }
        >
          Save size
        </Button>
      </div>
    </ModalDialog>
  );
}
