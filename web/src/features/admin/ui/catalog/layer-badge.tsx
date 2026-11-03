import { Button } from "react-aria-components";
import type { Layer, Lock } from "../../../../lib/api/catalog-types";
import { Badge, type BadgeTone } from "../../../../lib/design/components/badge/badge";
import { Tooltip } from "../../../../lib/design/components/tooltip/tooltip";
import { explain } from "./term-tip";

const layerTone: Record<Layer, BadgeTone> = { system: "neutral", plan: "info", tenant: "primary", workspace: "success" };
const lockTone: Record<Lock, BadgeTone> = { locked: "danger", overridable: "neutral", mergeable: "info", bounded: "warning" };

/** Where a value came from ("from: plan team"), with the layer explained on hover/focus. */
export function LayerBadge({ layer, refId }: { layer: Layer; refId?: string }) {
  return (
    <Tooltip content={explain(layer)}>
      <Button className="focus-ring cursor-help rounded-full" aria-label={`From ${layer} ${refId ?? ""}`}>
        <Badge tone={layerTone[layer]}>{layer}{refId ? ` · ${refId.length > 12 ? refId.slice(0, 8) + "…" : refId}` : ""}</Badge>
      </Button>
    </Tooltip>
  );
}

export function LockBadge({ lock }: { lock: Lock }) {
  return (
    <Tooltip content={explain(lock)}>
      <Button className="focus-ring cursor-help rounded-full" aria-label={`Lock mode ${lock}`}>
        <Badge tone={lockTone[lock]}>{lock}</Badge>
      </Button>
    </Tooltip>
  );
}
