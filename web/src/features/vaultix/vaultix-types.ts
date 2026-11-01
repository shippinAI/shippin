export type VaultixAccount = { org: string; workspace: string; workspace_slug: string; email: string; envs: string[]; keyring_connected: boolean; api_url: string };
export type VaultixSecret = { id: string; key: string; path: string; version: number; updated_at: string };
export type KeyringLogin = { api_url: string; client_id: string; client_secret: string; workspace: string; workspace_slug: string };
export type CloudImport = { url: string; client_id: string; client_secret: string; project: string; from_env: string; to_env: string; path: string };
