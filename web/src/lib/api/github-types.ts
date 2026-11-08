/** M3: repositories the tenant's Shippin GitHub App installations can access. */
export type GitHubRepo = { id: number; full_name: string; private: boolean; default_branch: string };

export type GitHubInstallationRepos = {
  installation_id: number;
  account_login: string;
  suspended: boolean;
  error?: string;
  repos: GitHubRepo[];
};

export type GitHubRepos = { installations: GitHubInstallationRepos[]; can_install: boolean };

export type ImportRequest = { installation_id: number; repo_id: number; branch?: string; workspace_id?: string };
