import { Button } from "../../../lib/design/components/button/button";
import { useText } from "../../../lib/ui-config/use-ui-config";

type Props = { updatedAt: number; refreshing: boolean; error: Error | null; onRefresh: () => void };

/** Inventory stays visible after a failed refresh, with its age and failure made explicit. */
export function GuestInventoryStatus({ updatedAt, refreshing, error, onRefresh }: Props) {
  const t = useText();
  return (
    <div className="space-y-2 border-b border-line px-5 py-3 text-sm">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="space-y-1 text-fg-muted">
          <p>{t("infra.inventory.description")}</p>
          {updatedAt > 0 && <p>{t("infra.inventory.checked", { time: new Date(updatedAt).toLocaleString() })}</p>}
        </div>
        <Button size="sm" variant="secondary" isDisabled={refreshing} tooltip={t("infra.inventory.refreshTooltip")} onPress={onRefresh}>
          {refreshing ? t("infra.inventory.refreshing") : t("infra.inventory.refresh")}
        </Button>
      </div>
      {error && (
        <p role="alert" className="text-danger">
          {t(updatedAt > 0 ? "infra.inventory.stale" : "infra.inventory.failed")} {error.message}
        </p>
      )}
    </div>
  );
}
