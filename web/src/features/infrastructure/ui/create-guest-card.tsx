import { useEffect, useState } from "react";
import { Plus } from "lucide-react";
import { Button } from "../../../lib/design/components/button/button";
import { Card } from "../../../lib/design/components/card/card";
import { TextField } from "../../../lib/design/components/field/text-field";
import { Select } from "../../../lib/design/components/select/select";
import { Switch } from "../../../lib/design/components/switch/switch";
import { useGuestImages, type NewGuest } from "../../../lib/hooks/use-hosts";
import type { InfraProvider } from "../../../lib/hooks/use-infra";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { guestNameOK, imagesFor } from "../model";

const sizeOK = (v: string) => /^\d*$/.test(v);
const size = (v: string) => Number(v || 0);

/** Makes a guest. What it asks for comes from the driver, and the images from the host and configuration. */
export function CreateGuestCard({ base, provider, onCreate, busy }: { base: string; provider: InfraProvider; onCreate: (g: NewGuest) => void; busy: boolean }) {
  const t = useText();
  const images = useGuestImages(base);
  const has = (f: string) => (provider.create_fields ?? []).includes(f);
  const [kind, setKind] = useState("ct");
  const [name, setName] = useState("");
  const [image, setImage] = useState("");
  const [memory, setMemory] = useState("");
  const [cores, setCores] = useState("");
  const [disk, setDisk] = useState("");
  const [start, setStart] = useState(true);
  const options = imagesFor(images.data?.items ?? [], kind);
  useEffect(() => {
    if (!options.some((o) => o.id === image)) setImage(options[0]?.id ?? "");
    // Choose again only when what can be chosen changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [kind, images.data]);
  const needsImage = !(kind === "vm" && has("blank_vm"));
  const valid = guestNameOK(name) && (!needsImage || image !== "") && [memory, cores, disk].every(sizeOK);
  const kinds = [{ id: "ct", label: t("infra.guest.ct") }, { id: "vm", label: t("infra.guest.vm") }];
  return (
    <Card className="p-5" data-testid="create-guest">
      <h2 className="mb-3 text-sm font-semibold text-fg">{t("infra.create.title")}</h2>
      <form className="space-y-3" onSubmit={(e) => {
        e.preventDefault();
        if (valid) onCreate({ kind, name, image, memory: size(memory), cores: size(cores), disk: size(disk), start });
      }}>
        <TextField label={t("infra.create.name")} value={name} onChange={setName} description={t("infra.create.nameHelp")} />
        <Select label={t("infra.create.kind")} options={kinds} value={kind} onChange={setKind} />
        {options.length > 0 ? (
          <Select label={t("infra.create.image")} options={options.map((o) => ({ id: o.id, label: o.label }))} value={image} onChange={setImage} />
        ) : needsImage ? (
          <p className="text-[13px] text-fg-muted" data-testid="no-images">{images.isError ? images.error.message : t("infra.create.noImages")}</p>
        ) : null}
        <div className="grid grid-cols-3 gap-3">
          {has("memory") && <TextField label={t("infra.create.memory")} value={memory} onChange={setMemory} placeholder={t("infra.create.default")} />}
          {has("cores") && <TextField label={t("infra.create.cores")} value={cores} onChange={setCores} placeholder={t("infra.create.default")} />}
          {has("disk") && <TextField label={t("infra.create.disk")} value={disk} onChange={setDisk} placeholder={t("infra.create.default")} />}
        </div>
        <Switch isSelected={start} onChange={setStart} tooltip={t("infra.create.startTooltip")}>{t("infra.create.start")}</Switch>
        <Button type="submit" variant="primary" size="sm" tooltip={t("infra.create.tooltip")} isDisabled={!valid || busy}>
          <Plus className="size-4" /> {t("infra.create.submit")}
        </Button>
      </form>
    </Card>
  );
}
