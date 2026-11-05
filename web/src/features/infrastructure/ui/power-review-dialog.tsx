import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { apiGet, apiSend } from "../../../lib/api/client";
import { Button } from "../../../lib/design/components/button/button";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../lib/design/components/field/text-field";
import { noteOK } from "../model";

type Verb = "start" | "stop" | "shutdown" | "reboot";
type Review = { id: string; agenthub_ticket_id: string; state: string; expires_at: string };
type ReviewView = { review: Review; agenthub_status: string; agenthub_ticket_sha256?: string };

/** Guest power remains an explicit two-step AgentHub workflow in both operator and tenant views. */
export function PowerReviewDialog({ base, guest, verb, confirmSelf = false, onClose, onExecuted }: {
  base: string; guest: string; verb: Verb; confirmSelf?: boolean; onClose: () => void; onExecuted?: () => void;
}) {
  const [note, setNote] = useState("");
  const [selfConfirmed, setSelfConfirmed] = useState(false);
  const [review, setReview] = useState<Review | null>(null);
  const [error, setError] = useState("");
  const [requesting, setRequesting] = useState(false);
  const [executing, setExecuting] = useState(false);
  const [requestKey] = useState(() => crypto.randomUUID());
  const status = useQuery({
    queryKey: ["infra", "power-review", base, guest, review?.id],
    queryFn: () => apiGet<ReviewView>(`${base}/guests/${encodeURIComponent(guest)}/power-reviews/${review!.id}`),
    enabled: !!review?.id,
    refetchInterval: (q) => {
      const review = q.state.data?.review;
      const status = q.state.data?.agenthub_status ?? "";
      if (status === "passed" && review?.expires_at && Date.parse(review.expires_at) > Date.now()) return 3000;
      return ["passed", "denied", "rejected", "failed", "succeeded", "failed_execution", "unknown", "expired", "binding_mismatch", "stale"].includes(status) ? false : 3000;
    },
  });
  const request = async () => {
    setError("");
    setRequesting(true);
    try {
      const result = await apiSend<{ review: Review }>("POST", `${base}/guests/${encodeURIComponent(guest)}/power-review/${verb}${confirmSelf ? "?confirm=self" : ""}`, {}, { note: note.trim(), idempotencyKey: requestKey });
      setReview(result.review);
    } catch (e) { setError(e instanceof Error ? e.message : "Could not request AgentHub review."); }
    finally { setRequesting(false); }
  };
  const execute = async () => {
    if (!review) return;
    setError("");
    setExecuting(true);
    try {
      await apiSend("POST", `${base}/guests/${encodeURIComponent(guest)}/status/${verb}${confirmSelf ? "?confirm=self" : ""}`, {}, {
        note: note.trim(), headers: { "X-AgentHub-Review-ID": review.id },
      });
      onExecuted?.();
      onClose();
    } catch (e) { setError(e instanceof Error ? e.message : "The guest action did not complete."); }
    finally { setExecuting(false); }
  };
  const agentHubStatus = status.data?.agenthub_status ?? "pending_review";
  return (
    <ModalDialog isOpen title={`Review ${verb} ${guest}`} description="AgentHub is the approval authority. The provider call runs only after its current review passes." onOpenChange={(open) => { if (!open) onClose(); }}>
      <div className="space-y-4 px-6 py-5">
        {!review ? <>
          <TextField label="Change note" description="Explain why this guest action is needed." value={note} onChange={setNote} autoFocus />
          {confirmSelf && <label className="flex items-start gap-2 text-sm text-warning"><input type="checkbox" checked={selfConfirmed} onChange={(e) => setSelfConfirmed(e.currentTarget.checked)} />I understand this action can take down the Shippin control plane.</label>}
          <Button variant="primary" tooltip="Create a review request in AgentHub" isDisabled={requesting || !noteOK(note) || (confirmSelf && !selfConfirmed)} onPress={() => void request()}>{requesting ? "Requesting review…" : "Request AgentHub review"}</Button>
        </> : <>
          <dl className="grid gap-2 text-sm">
            <div><dt className="text-fg-subtle">AgentHub ticket</dt><dd className="break-all font-mono text-fg">{review.agenthub_ticket_id}</dd></div>
            <div><dt className="text-fg-subtle">Review state</dt><dd className="text-fg">{status.data?.agenthub_status ?? review.state}</dd></div>
          </dl>
          {review.expires_at && <p className="text-sm text-fg-muted">Shippin review window ends {new Date(review.expires_at).toLocaleString()}.</p>}
          {agentHubStatus === "passed" && <Button variant="primary" tooltip="Recheck AgentHub and execute this exact action once" isDisabled={executing || !review.expires_at || Date.parse(review.expires_at) <= Date.now()} onPress={() => void execute()}>{executing ? "Executing…" : "Execute reviewed action"}</Button>}
          {agentHubStatus === "expired" && <p role="alert" className="text-sm text-danger">This Shippin review window expired. Request a new AgentHub review before executing.</p>}
          {agentHubStatus === "unavailable" && <p role="status" className="text-sm text-warning">AgentHub could not be reached. Execution stays disabled while this panel retries.</p>}
          {agentHubStatus === "binding_mismatch" && <p role="alert" className="text-sm text-danger">The AgentHub ticket no longer matches this action. Request a new review.</p>}
          {agentHubStatus === "stale" && <p role="alert" className="text-sm text-danger">The provider connection changed after review. Request a new review.</p>}
          {agentHubStatus === "pending_review" && <p role="status" className="text-sm text-fg-muted">Waiting for an authorized reviewer in AgentHub. This panel checks again automatically.</p>}
          {["denied", "rejected", "failed"].includes(agentHubStatus) && <p role="alert" className="text-sm text-danger">AgentHub did not approve this action. Request a new review if the action is still needed.</p>}
          {status.isError && <p role="alert" className="text-sm text-warning">The current AgentHub ticket status could not be verified. Execution stays disabled while this panel retries.</p>}
        </>}
        {error && <p role="alert" className="text-sm text-danger">{error}</p>}
        <div className="flex justify-end border-t border-line pt-3"><Button variant="ghost" tooltip="Close review details" onPress={onClose}>Close</Button></div>
      </div>
    </ModalDialog>
  );
}
