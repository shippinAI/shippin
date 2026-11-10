import { ExternalLink } from "lucide-react";
import { useState } from "react";
import { Form } from "react-aria-components";
import { Button } from "../../../lib/design/components/button/button";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { TextField } from "../../../lib/design/components/field/text-field";
import { githubInstallURL, useGitHubRepos, useImportRepo } from "../../../lib/hooks/use-github";
import { GitHubRepoPicker, type PickedRepo } from "./github-repo-picker";

const slug = (s: string) => s.toLowerCase().replace(/[^a-z0-9-]+/g, "-").replace(/^[^a-z]+|-+$/g, "").slice(0, 40);

/** Import a repository through the Shippin GitHub App (private repos, push and PRs work). */
export function GitHubImportForm({ tenantId, active, workspaceId }: { tenantId: string; active: boolean; workspaceId: string }) {
  const q = useGitHubRepos(tenantId, active);
  const imp = useImportRepo(tenantId);
  const [picked, setPicked] = useState<PickedRepo>();
  const [name, setName] = useState("");
  const [branch, setBranch] = useState("");
  if (q.isError) return <EmptyState title="GitHub import isn't available" description="It isn't switched on for this tenant yet." />;
  const installs = q.data?.installations ?? [];
  const install = q.data?.can_install && (
    <Button variant="secondary" size="sm" tooltip="Install the Shippin GitHub App and choose which repositories it can access"
      onPress={() => window.location.assign(githubInstallURL(tenantId))}>
      <ExternalLink className="size-4" /> {installs.length ? "Add repositories on GitHub" : "Connect GitHub"}
    </Button>
  );
  if (q.data && installs.length === 0)
    return <div className="space-y-3 p-5 pt-0"><p className="text-sm text-fg-muted">Connect GitHub to import a repository. You choose exactly which repositories Shippin can see.</p>{install}</div>;
  return (
    <Form className="space-y-4 p-5 pt-0" onSubmit={(e) => {
      e.preventDefault();
      if (!picked) return;
      imp.mutate({ installation_id: picked.installation, repo_id: picked.repo.id, branch: branch || undefined, workspace_id: workspaceId }, { onSuccess: () => setPicked(undefined) });
    }}>
      <GitHubRepoPicker installations={installs} value={picked} onChange={(p) => { setPicked(p); setName(slug(p.repo.full_name.split("/")[1] ?? "")); setBranch(""); }} />
      {name && <p className="text-sm text-fg-muted">Becomes project <span className="font-mono text-fg">{name}</span>, named after the repository.</p>}
      <TextField label="Branch" placeholder={picked?.repo.default_branch ?? "main"} value={branch} onChange={setBranch} maxLength={200} description="Leave empty for the default branch." />
      <div className="flex flex-wrap items-center gap-2">
        <Button type="submit" variant="primary" size="sm" isDisabled={!picked || imp.isPending || !workspaceId} tooltip="Create the project and clone the repository into its devspace">
          Import
        </Button>
        {install}
      </div>
    </Form>
  );
}
