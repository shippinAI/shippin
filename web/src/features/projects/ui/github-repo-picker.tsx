import { Select } from "../../../lib/design/components/select/select";
import type { GitHubInstallationRepos, GitHubRepo } from "../../../lib/api/github-types";

export type PickedRepo = { installation: number; repo: GitHubRepo };

type Props = { installations: GitHubInstallationRepos[]; value?: PickedRepo; onChange: (p: PickedRepo) => void };

/** One select over every repository the tenant's installations can access. */
export function GitHubRepoPicker({ installations, value, onChange }: Props) {
  const all = installations.filter((i) => !i.suspended).flatMap((i) => i.repos.map((repo) => ({ installation: i.installation_id, repo })));
  const id = (p: PickedRepo) => `${p.installation}:${p.repo.id}`;
  return (
    <Select
      label="Repository"
      value={value ? id(value) : ""}
      onChange={(k) => {
        const p = all.find((x) => id(x) === k);
        if (p) onChange(p);
      }}
      options={all.map((p) => ({ id: id(p), label: `${p.repo.full_name}${p.repo.private ? " (private)" : ""}` }))}
    />
  );
}
