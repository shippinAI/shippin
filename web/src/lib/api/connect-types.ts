/** Connect accounts: per-workspace CLI sign-ins. No credential field is ever returned. */
export type ConnectionStatus = "connected" | "expiring" | "not_connected";

export type Connection = {
  id: string;
  label: string;
  vendor: string;
  token_label: string;
  token_hint: string;
  token_url: string;
  browser_note: string;
  status: ConnectionStatus;
  method?: "browser" | "token";
  expires_at?: string;
  stored_in_vaultix: boolean;
};

export type Connections = {
  checked_at: string;
  running: boolean;
  ide_terminal_url: string;
  connections: Connection[];
};

export type LoginState = {
  provider: string;
  state: "starting" | "waiting" | "connected" | "failed" | "expired";
  verification_url?: string;
  user_code?: string;
  code_input: boolean;
  started_at: string;
};
