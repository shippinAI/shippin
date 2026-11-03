import type { A2UIBlock } from "../chat/chat-types";

/** Member view of the official app cards. Commands are the vendors' own steps. */
export type AppsView = {
  signed_in: "unknown";
  desktop_ready: boolean;
  can_prepare: boolean;
  status_detail: string;
  kvm: boolean;
  blocks: A2UIBlock[];
  apps: { id: string; title: string; summary: string; command: string; update?: string; alt_id?: string; alt_label?: string; alt_command?: string; alt_update?: string; alt_update_label?: string; docs_url: string; docs_label: string; where: string }[];
};

/** Operator view: status only. No install command is sent to the staff screen. */
export type OperatorApps = {
  signed_in: "unknown";
  desktop_ready: boolean;
  status_detail: string;
  apps: { id: string; title: string; status: string }[];
};
