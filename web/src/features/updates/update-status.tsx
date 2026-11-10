import { useQuery } from "@tanstack/react-query";
import { ArrowUpCircle, CheckCircle2 } from "lucide-react";
import { Button } from "react-aria-components";
import { apiGet } from "../../lib/api/client";
import { cn } from "../../lib/design/components/cn/cn";
import { Popover } from "../../lib/design/components/popover/popover";
import { useText } from "../../lib/ui-config/use-ui-config";

type UpdateInfo = {
  short: string;
  running: { commit: string; built_at?: string; modified?: boolean };
  command?: string;
  source?: string;
  update?: { checked: boolean; available: boolean; behind: number; latest_commit?: string; latest_title?: string; latest_date?: string; compare_url?: string; error?: string };
};

const useUpdateInfo = () =>
  useQuery({
    queryKey: ["version"],
    queryFn: () => apiGet<UpdateInfo>("/v1/version"),
    retry: false,
    // Quick until the first check has finished, then only as often as the server re-reads the source.
    refetchInterval: (q) => (q.state.data?.update && !q.state.data.update.checked ? 5_000 : 300_000),
  });

/** Which build is running, and whether a new push is waiting to be pulled. Operators only. */
export function UpdateStatus() {
  const t = useText();
  const { data } = useUpdateInfo();
  if (!data) return null;
  const u = data.update;
  const available = !!u?.available;
  const label = available ? t("update.available", { count: u!.behind }) : data.short ? t("update.build", { build: data.short }) : t("update.unstamped");
  return (
    <Popover title={available ? t("update.title") : t("update.currentTitle")} placement="bottom end"
      trigger={
        <Button aria-label={label} data-testid="update-status"
          className={cn("focus-ring hidden h-8 items-center gap-1.5 rounded-full border px-2.5 text-xs outline-none sm:inline-flex",
            available ? "border-primary/40 bg-primary/15 text-primary" : "border-line bg-raised text-fg-muted")}>
          {available ? <ArrowUpCircle className="size-3.5" aria-hidden /> : <CheckCircle2 className="size-3.5" aria-hidden />}
          {label}
        </Button>
      }>
      <div className="space-y-2 text-sm" data-testid="update-details">
        <p className="text-fg-muted">{t("update.running")} <code className="font-mono">{data.short || t("update.unstamped")}</code>{data.running.modified ? ` ${t("update.modified")}` : ""}</p>
        {u?.error && <p className="text-warning">{u.error}</p>}
        {!u && <p className="text-fg-muted">{t("update.disabled")}</p>}
        {u && !u.checked && <p className="text-fg-muted">{t("update.checking")}</p>}
        {u?.checked && !u.error && !available && <p className="text-fg-muted">{t("update.current", { source: data.source ?? "" })}</p>}
        {available && (
          <>
            <p className="text-fg">{u!.latest_title}</p>
            {u!.latest_date && <p className="text-fg-subtle">{new Date(u!.latest_date).toLocaleString()}</p>}
            {u!.compare_url && <a className="text-primary underline" href={u!.compare_url} target="_blank" rel="noreferrer">{t("update.changes")}</a>}
            {data.command
              ? <><p className="pt-1 text-fg-muted">{t("update.howTo")}</p><pre className="whitespace-pre-wrap break-all rounded-md bg-canvas p-2 font-mono text-xs text-fg">{data.command}</pre></>
              : <p className="pt-1 text-fg-muted">{t("update.howToUnknown")}</p>}
          </>
        )}
      </div>
    </Popover>
  );
}
