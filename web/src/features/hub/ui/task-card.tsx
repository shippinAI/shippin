import { Bot, Check, Clock, X } from "lucide-react";
import { Button } from "../../../lib/design/components/button/button";
import { toast } from "../../../lib/design/components/toast/toast-queue";
import type { HubTask } from "../hub-types";
import { useDecide } from "../use-hub";
import { TaskHistory } from "./task-history";

const ago = (iso: string) => {
  const s = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
  return s < 60 ? `${s}s ago` : s < 3600 ? `${Math.round(s / 60)}m ago` : `${Math.round(s / 3600)}h ago`;
};

/** One card. The owner sees Approve/Reject on cards that wait for sign-off. */
export function TaskCard({ task, isOwner }: { task: HubTask; isOwner: boolean }) {
  const decide = useDecide();
  const act = (approve: boolean) =>
    decide.mutate(
      { id: task.id, approve },
      {
        onSuccess: () => toast.success(approve ? "Approved" : "Rejected", task.title),
        onError: (e) => toast.error("Decision failed", e.message),
      },
    );
  const waiting = task.status === "needs_approval";
  return (
    <article
      aria-label={task.title}
      className={`group rounded-xl border bg-surface p-3.5 shadow-e1 transition hover:shadow-e2 ${waiting ? "border-warning/40 ring-1 ring-warning/20" : "border-line"}`}
    >
      <h3 className="text-sm leading-snug font-semibold text-fg">{task.title}</h3>
      {task.body && <p className="mt-1 line-clamp-2 text-sm text-fg-muted">{task.body}</p>}
      {task.progress && <p className="mt-2 rounded-md bg-raised px-2 py-1 text-xs text-fg-muted">{task.progress}</p>}
      {waiting && task.approval_note && (
        <p className="mt-2 rounded-md border border-warning/25 bg-warning/8 px-2 py-1 text-xs text-fg">{task.approval_note}</p>
      )}
      <footer className="mt-3 flex items-center justify-between gap-2 text-xs text-fg-subtle">
        <span className="inline-flex min-w-0 items-center gap-1 truncate">
          <Bot className="size-3.5 shrink-0" aria-hidden />
          {task.lease_owner || task.decided_by || task.created_by}
        </span>
        <span className="inline-flex shrink-0 items-center gap-1"><Clock className="size-3" aria-hidden />{ago(task.updated_at)}</span>
      </footer>
      <TaskHistory taskId={task.id} />
      {waiting && isOwner && (
        <div className="mt-3 grid grid-cols-2 gap-2">
          <Button variant="primary" size="sm" tooltip="Approve this task" isDisabled={decide.isPending} onPress={() => act(true)}>
            <Check className="size-3.5" aria-hidden /> Approve
          </Button>
          <Button variant="secondary" size="sm" tooltip="Reject this task" isDisabled={decide.isPending} onPress={() => act(false)}>
            <X className="size-3.5" aria-hidden /> Reject
          </Button>
        </div>
      )}
    </article>
  );
}
