import { isWaitingOnBoard } from "../../../lib/api/client";
import { Button } from "../../../lib/design/components/button/button";
import { useApproveCard } from "../../../lib/hooks/use-infra";
import { useNavigate } from "../../../lib/router/router";
import { useText } from "../../../lib/ui-config/use-ui-config";

/** A change waiting for approval, and the way through it: approve here and run it, or go to the board. */
export function BoardNotice({ error, retry }: { error: Error | null | undefined; retry: () => void }) {
  const nav = useNavigate();
  const t = useText();
  const approve = useApproveCard();
  if (!isWaitingOnBoard(error)) return null;
  const card = error.cardId;
  return (
    <div role="status" data-testid="board-notice" className="rounded-lg border border-warning/30 bg-warning/10 p-4">
      <h2 className="mb-1 text-sm font-semibold text-fg">{t("infra.board.title")}</h2>
      <p className="text-sm text-fg-muted">{error.message}{!card && ` ${t("infra.board.waitingOther")}`}</p>
      {approve.isError && <p role="alert" className="mt-2 break-words text-sm text-danger">{approve.error.message}</p>}
      <div className="mt-3 flex flex-wrap gap-2">
        {/* Run it again whatever the answer: the card may already have been approved on the board. */}
        {card && (
          <Button size="sm" variant="primary" tooltip={t("infra.board.approveTooltip")} isDisabled={approve.isPending} onPress={() => approve.mutate(card, { onSettled: retry })}>
            {t("infra.board.approve")}
          </Button>
        )}
        <Button size="sm" variant={card ? "ghost" : "primary"} tooltip={t("infra.board.openTooltip")} onPress={() => nav("/hub")}>{t("infra.board.open")}</Button>
      </div>
    </div>
  );
}
