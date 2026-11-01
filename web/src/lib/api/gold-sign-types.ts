/** Facts gold-policy parsed from the ticket (synced from Gold). agent_summary is agent text. */
export type GoldFacts = {
  id: string;
  sha256: string;
  tier: string;
  tier_reasons: string[] | null;
  classes: string[] | null;
  host: string;
  oob_path: string;
  commands: string[] | null;
  rollback: string[] | null;
  verification: string[] | null;
  requested_at: number;
  agent_summary: string;
  sent?: "approve" | "deny" | "";
};
export type CredRef = { credential_id: string; kid: string };
export type GoldTickets = { items: GoldFacts[]; rp_id: string; keys: CredRef[]; sync_at: number; fresh: boolean };
export type BypassCert = {
  cid: string;
  holder_agent: string;
  scope: string[];
  nbf: number;
  exp: number;
  max_uses: number;
  uses: number;
  reason: string;
  revoke_sent: boolean;
};
export type BypassInfo = { status: { active: BypassCert[]; now: number; known: boolean; sync_at: number }; can_revoke: boolean; rp_id: string; keys: CredRef[] };
export type GoldKey = { kid: string; label: string; fingerprint: string; credential_id: string; created_at: number };
export type RegBegin = { challenge: string; rp_id: string; user_id: string; user_name: string; exclude: CredRef[] };
