export type HubStatus = "open" | "claimed" | "in_progress" | "needs_approval" | "approved" | "done" | "rejected";

export type HubTask = {
  id: string;
  title: string;
  body: string;
  status: HubStatus;
  progress: string;
  lease_owner?: string;
  lease_until?: string;
  approval_note: string;
  decided_by?: string;
  decided_at?: string;
  created_by: string;
  created_at: string;
  updated_at: string;
};

export type HubEvent = { id: number; actor: string; kind: string; note: string; at: string };

export type HubMe = { sub: string; name: string; is_owner: boolean };

export const columns: { status: HubStatus; label: string; accent: string }[] = [
  { status: "open", label: "Open", accent: "bg-fg-subtle" },
  { status: "claimed", label: "Claimed", accent: "bg-info" },
  { status: "in_progress", label: "In progress", accent: "bg-primary" },
  { status: "needs_approval", label: "Needs approval", accent: "bg-warning" },
  { status: "approved", label: "Approved", accent: "bg-success" },
  { status: "done", label: "Done", accent: "bg-success" },
  { status: "rejected", label: "Rejected", accent: "bg-danger" },
];
